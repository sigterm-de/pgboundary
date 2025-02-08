package boundary

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"pgboundary/config"
	"pgboundary/internal/process"

	"github.com/hashicorp/boundary/api"
	"github.com/hashicorp/boundary/api/authmethods"
	"github.com/hashicorp/boundary/api/scopes"
)

type Connection struct {
	Username string
	Password string
	Host     string
	Port     string
	Pid      int
}

const defaultTimeout = 45 * time.Second

func getPrimaryAuthMethodId(client *api.Client, scopeId string, preferredMethod string) (string, error) {
	authMethodClient := authmethods.NewClient(client)

	// List auth methods in the scope
	result, err := authMethodClient.List(context.Background(), scopeId)
	if err != nil {
		return "", fmt.Errorf("failed to list auth methods: %w", err)
	}

	if process.Verbose {
		fmt.Printf("Found %d auth methods in scope %s:\n", len(result.Items), scopeId)
		for _, method := range result.Items {
			fmt.Printf("  - ID: %s, Type: %s, Name: %s\n", method.Id, method.Type, method.Name)
		}
	}

	// Look for the preferred auth method type
	for _, method := range result.Items {
		if method.Type == preferredMethod {
			if process.Verbose {
				fmt.Printf("Selected %s auth method: %s\n", preferredMethod, method.Id)
			}
			return method.Id, nil
		}
	}

	return "", fmt.Errorf("no %s auth method found in scope %s", preferredMethod, scopeId)
}

func StartConnection(target config.Target, authScope, targetScope, authMethod string) (*Connection, error) {
	// Initialize the client
	client, err := api.NewClient(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create boundary client: %w", err)
	}
	client.SetAddr(target.Host)

	// Get scope ID if not global
	var scopeId string
	if authScope != "global" {
		scopeClient := scopes.NewClient(client)

		ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
		defer cancel()

		listResult, err := scopeClient.List(ctx, "global")
		if err != nil {
			return nil, fmt.Errorf("failed to list scopes: %w", err)
		}

		for _, scope := range listResult.Items {
			if scope.Name == authScope {
				scopeId = scope.Id
				break
			}
		}
		if scopeId == "" {
			return nil, fmt.Errorf("scope %q not found", authScope)
		}
	} else {
		scopeId = "global"
	}

	// Get the primary auth method ID
	authMethodId, err := getPrimaryAuthMethodId(client, scopeId, authMethod)
	if err != nil {
		return nil, fmt.Errorf("failed to get auth method ID: %w", err)
	}
	// Authenticate
	authCmd := exec.Command("boundary", "authenticate", authMethod,
		"-scope-id", scopeId,
		"-auth-method-id", authMethodId,
		"-addr", target.Host,
		"-keyring-type", "none",
		"-format", "json")

	out, err := authCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("authentication failed: %w", err)
	}

	var authResp struct {
		Item struct {
			Attributes struct {
				Token string `json:"token"`
			} `json:"attributes"`
		} `json:"item"`
	}
	if err := json.Unmarshal(out, &authResp); err != nil {
		return nil, fmt.Errorf("failed to parse auth response: %w", err)
	}

	// Create a temporary file for the connection output
	tmpDir, err := os.MkdirTemp("", "boundary-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	outputFile := filepath.Join(tmpDir, "connection.json")

	// Start boundary connection in background
	connectCmd := exec.Command("boundary", "connect",
		"-target-name", target.Target,
		"-target-scope-name", targetScope,
		"-addr", target.Host,
		"-token", "env://BOUNDARY_TOKEN",
		"-format", "json")
	connectCmd.Env = append(os.Environ(), "BOUNDARY_TOKEN="+authResp.Item.Attributes.Token)

	// Open output file
	output, err := os.Create(outputFile)
	if err != nil {
		return nil, fmt.Errorf("failed to create output file: %w", err)
	}
	defer output.Close()

	connectCmd.Stdout = output
	connectCmd.Stderr = os.Stderr

	// Start the process
	if err := connectCmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start boundary connect: %w", err)
	}

	// Store the PID
	boundaryPid := connectCmd.Process.Pid

	// Wait a bit for the connection to be established
	time.Sleep(3 * time.Second)

	// Read and parse the output file
	content, err := os.ReadFile(outputFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read connection output: %w", err)
	}

	var connResp struct {
		Credentials []struct {
			Credential struct {
				Username string `json:"username"`
				Password string `json:"password"`
			} `json:"credential"`
		} `json:"credentials"`
		Address string `json:"address"`
		Port    int    `json:"port"`
	}

	if err := json.Unmarshal(content, &connResp); err != nil {
		return nil, fmt.Errorf("failed to parse connection response: %w", err)
	}

	if len(connResp.Credentials) == 0 {
		return nil, fmt.Errorf("no credentials found in response")
	}

	return &Connection{
		Username: connResp.Credentials[0].Credential.Username,
		Password: connResp.Credentials[0].Credential.Password,
		Host:     connResp.Address,
		Port:     strconv.Itoa(connResp.Port),
		Pid:      boundaryPid,
	}, nil
}

func Shutdown() error {
	// Get our own PID to exclude it
	ownPid := os.Getpid()

	procDir, err := os.Open("/proc")
	if err != nil {
		return fmt.Errorf("failed to open /proc: %w", err)
	}
	defer procDir.Close()

	entries, err := procDir.Readdir(-1)
	if err != nil {
		return fmt.Errorf("failed to read /proc directory: %w", err)
	}

	foundProcesses := false
	for _, entry := range entries {
		// Only look at directory entries that are numbers (PIDs)
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}

		// Skip our own process
		if pid == ownPid {
			continue
		}

		// Check if it's a boundary process
		if process.IsProcessType(pid, "boundary") {
			foundProcesses = true
			if err := process.KillProcess(pid); err != nil {
				return fmt.Errorf("failed to kill boundary process: %w", err)
			}
		}
	}

	if !foundProcesses {
		return fmt.Errorf("no boundary processes found")
	}

	return nil
}
