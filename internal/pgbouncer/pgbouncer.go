package pgbouncer

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"pgboundary/config"
	"pgboundary/internal/boundary"
	"pgboundary/internal/process"

	"gopkg.in/ini.v1"
)

func UpdateConfig(cfg *config.Config, targetName string, conn *boundary.Connection) error {
	if cfg == nil || conn == nil {
		return fmt.Errorf("invalid configuration or connection")
	}

	target, ok := cfg.Targets[targetName]
	if !ok {
		return fmt.Errorf("target %q not found in configuration", targetName)
	}

	connectionsDir := filepath.Join(cfg.PgBouncer.WorkDir, ".pgboundary-connections")
	if err := os.MkdirAll(connectionsDir, 0700); err != nil {
		return fmt.Errorf("failed to create connections directory: %w", err)
	}

	connFile := filepath.Join(connectionsDir, targetName+".ini")

	// Extract config string creation for better readability
	configContent := formatDatabaseConfig(targetName, conn, target.Database)

	if err := os.WriteFile(connFile, []byte(configContent), 0600); err != nil {
		return fmt.Errorf("failed to write connection config: %w", err)
	}

	if err := removeIncludeLine(cfg.PgBouncer.ConfFile, connFile); err != nil {
		return fmt.Errorf("failed to remove existing include for %s: %w", targetName, err)
	}

	f, err := os.OpenFile(cfg.PgBouncer.ConfFile, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open pgbouncer config: %w", err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			fmt.Printf("failed to close pgbouncer config: %v\n", err)
		}
	}()

	if _, err := fmt.Fprintf(f, "\n%%include %s\n", connFile); err != nil {
		return fmt.Errorf("failed to update pgbouncer config: %w", err)
	}

	return nil
}

// removeIncludeLine removes any %include line in confFile that points at
// includePath, so a repeated UpdateConfig call for the same target doesn't
// accumulate duplicate entries.
func removeIncludeLine(confFile, includePath string) error {
	content, err := os.ReadFile(confFile)
	if err != nil {
		return fmt.Errorf("failed to read pgbouncer config: %w", err)
	}

	lines := strings.Split(string(content), "\n")
	newLines := make([]string, 0, len(lines))
	changed := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "%include") {
			existing := strings.TrimSpace(strings.TrimPrefix(trimmed, "%include"))
			if existing == includePath {
				changed = true
				continue
			}
		}
		newLines = append(newLines, line)
	}

	if !changed {
		return nil
	}

	return os.WriteFile(confFile, []byte(strings.Join(newLines, "\n")), 0644)
}

// RollbackConnection undoes a partially-applied UpdateConfig call: it
// removes the %include entry and connection file for targetName, and kills
// the boundary connect process backing it. Used when UpdateConfig succeeded
// but the following reload/start of pgbouncer failed, so a failed connect
// attempt leaves no trace.
func RollbackConnection(cfg *config.Config, targetName string, boundaryPid int) error {
	removeErr := removeConnection(cfg, targetName)

	var killErr error
	if boundaryPid > 0 {
		killErr = process.KillProcess(boundaryPid)
	}

	switch {
	case removeErr != nil && killErr != nil:
		return fmt.Errorf("failed to remove connection %s: %v; failed to kill boundary process %d: %w", targetName, removeErr, boundaryPid, killErr)
	case removeErr != nil:
		return fmt.Errorf("failed to remove connection %s: %w", targetName, removeErr)
	case killErr != nil:
		return fmt.Errorf("failed to kill boundary process %d: %w", boundaryPid, killErr)
	default:
		return nil
	}
}

func formatDatabaseConfig(targetName string, conn *boundary.Connection, dbName string) string {
	return fmt.Sprintf(
		"; boundary_pid=%d\n[databases]\n%s = host=%s port=%s dbname=%s user=%s password=%s",
		conn.Pid, targetName, conn.Host, conn.Port, dbName, conn.Username, conn.Password,
	)
}

func Reload(cfg *config.Config) error {
	pidBytes, err := os.ReadFile(cfg.PgBouncer.PidFile)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("failed to read pid file: %w", err)
		}
		// Start pgbouncer if not running
		return startPgBouncer(cfg)
	}

	pidStr := strings.TrimSpace(string(pidBytes))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return fmt.Errorf("invalid PID in file: %w", err)
	}

	if process.Verbose {
		fmt.Printf("sending HUP signal to pgbouncer with PID %d\n", pid)
	}
	if err := syscall.Kill(pid, syscall.SIGHUP); err != nil {
		if errors.Is(err, syscall.ESRCH) {
			// Process not found, start pgbouncer
			return startPgBouncer(cfg)
		}
		return fmt.Errorf("failed to send HUP signal: %w", err)
	}

	return nil
}

func Shutdown(cfg *config.Config) error {
	pidBytes, err := os.ReadFile(cfg.PgBouncer.PidFile)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("no PID file found at %s", cfg.PgBouncer.PidFile)
		}
		return fmt.Errorf("failed to read PID file: %w", err)
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(pidBytes)))
	if err != nil {
		return fmt.Errorf("invalid PID in file: %w", err)
	}

	if process.IsProcessType(pid, "pgbouncer") {
		if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
			if errors.Is(err, syscall.ESRCH) {
				return fmt.Errorf("no process found with PID %d", pid)
			}
			return fmt.Errorf("failed to send SIGTERM to process: %w", err)
		}
	}
	return nil
}

func CleanConfig(cfg *config.Config) error {
	content, err := os.ReadFile(cfg.PgBouncer.ConfFile)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	lines := strings.Split(string(content), "\n")
	newLines := make([]string, 0, len(lines))

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, ";") || !strings.HasPrefix(trimmed, "%include") {
			newLines = append(newLines, line)
		}
	}

	// Ensure file ends with a newline
	newContent := strings.TrimSpace(strings.Join(newLines, "\n")) + "\n"
	if err := os.WriteFile(cfg.PgBouncer.ConfFile, []byte(newContent), 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// Helper function to start pgbouncer
func startPgBouncer(cfg *config.Config) error {
	cmd := exec.Command("pgbouncer", "--daemon", cfg.PgBouncer.ConfFile)
	cmd.Dir = cfg.PgBouncer.WorkDir
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to start pgbouncer: %w", err)
	}
	return nil
}

func CheckStatus(pidFile string) (bool, int, error) {
	content, err := os.ReadFile(pidFile)
	if err != nil {
		return false, -1, fmt.Errorf("no pid file found at %s: %w", pidFile, err)
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(content)))
	if err != nil {
		return false, -1, fmt.Errorf("invalid pid file content: %w", err)
	}

	// Use process.IsProcessType to check if it's a running pgbouncer process
	if !process.IsProcessType(pid, "pgbouncer") {
		return false, -1, fmt.Errorf("process %d is not pgbouncer or is not running", pid)
	}

	return true, pid, nil
}

type ConnectionDetail struct {
	Name        string
	BoundaryPid int
}

func GetConnectionDetails(configFile string) ([]ConnectionDetail, error) {
	var connections []ConnectionDetail

	// First read the file to process includes
	content, err := os.ReadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("error reading config file %s: %w", configFile, err)
	}

	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "%include") {
			includePath := strings.TrimSpace(strings.TrimPrefix(trimmed, "%include"))
			included, err := parseIncludedFile(includePath)
			if err != nil {
				fmt.Printf("Warning: error processing include file %s: %v\n", includePath, err)
				continue
			}
			connections = append(connections, included...)
		}
	}

	return connections, nil
}

func parseIncludedFile(filePath string) ([]ConnectionDetail, error) {
	var connections []ConnectionDetail

	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("error reading include file %s: %w", filePath, err)
	}

	lines := strings.Split(string(content), "\n")
	var boundaryPid int

	// Parse the boundary PID from comments if present
	for _, line := range lines {
		if strings.HasPrefix(line, "; boundary_pid=") {
			pidStr := strings.TrimPrefix(line, "; boundary_pid=")
			if pid, err := strconv.Atoi(strings.TrimSpace(pidStr)); err == nil {
				boundaryPid = pid
				break
			}
		}
	}

	// Parse the file as INI
	file, err := ini.LoadSources(ini.LoadOptions{
		IgnoreInlineComment: true,
	}, filePath)
	if err != nil {
		return nil, fmt.Errorf("error parsing config file %s: %w", filePath, err)
	}

	// Get databases section
	dbSection := file.Section("databases")
	if dbSection != nil {
		for _, key := range dbSection.Keys() {
			connections = append(connections, ConnectionDetail{
				Name:        key.Name(),
				BoundaryPid: boundaryPid,
			})
		}
	}

	return connections, nil
}

func ShutdownConnection(cfg *config.Config, connectionName string) error {
	// Get connection details to find the boundary PID
	connections, err := GetConnectionDetails(cfg.PgBouncer.ConfFile)
	if err != nil {
		return fmt.Errorf("failed to get connection details: %w", err)
	}

	var targetConn *ConnectionDetail
	for _, conn := range connections {
		if conn.Name == connectionName {
			targetConn = &conn
			break
		}
	}

	if targetConn == nil {
		return fmt.Errorf("connection %q not found", connectionName)
	}

	// Kill the boundary process if it exists
	if targetConn.BoundaryPid > 0 && process.IsProcessType(targetConn.BoundaryPid, "boundary") {
		if err := process.KillProcess(targetConn.BoundaryPid); err != nil {
			return fmt.Errorf("failed to kill boundary process: %w", err)
		}
	}

	// Remove the connection from pgbouncer config
	if err := removeConnection(cfg, connectionName); err != nil {
		return fmt.Errorf("failed to remove connection from config: %w", err)
	}

	// Check if there are any remaining boundary connections
	remainingConnections, err := GetConnectionDetails(cfg.PgBouncer.ConfFile)
	if err != nil {
		return fmt.Errorf("failed to check remaining connections: %w", err)
	}

	hasActiveBoundary := false
	for _, conn := range remainingConnections {
		if conn.BoundaryPid > 0 {
			hasActiveBoundary = true
			break
		}
	}

	// If no more boundary connections, shutdown pgbouncer
	if !hasActiveBoundary {
		if process.Verbose {
			fmt.Println("no more boundary connections, shutting down pgbouncer")
		}
		if err := Shutdown(cfg); err != nil {
			return fmt.Errorf("failed to shutdown pgbouncer: %w", err)
		}
		if err := CleanConfig(cfg); err != nil {
			return fmt.Errorf("failed to clean pgbouncer config: %w", err)
		}
		return nil
	}

	// Otherwise just reload pgbouncer
	if err := Reload(cfg); err != nil {
		return fmt.Errorf("failed to reload pgbouncer: %w", err)
	}

	return nil
}

func removeConnection(cfg *config.Config, connectionName string) error {
	content, err := os.ReadFile(cfg.PgBouncer.ConfFile)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	lines := strings.Split(string(content), "\n")
	var newLines []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Check included files for the connection
		if strings.HasPrefix(trimmed, "%include") {
			includePath := strings.TrimSpace(strings.TrimPrefix(trimmed, "%include"))
			if hasConnection(includePath, connectionName) {
				// Remove the included file
				func() {
					if err := os.Remove(includePath); err != nil {
						fmt.Printf("failed to remove include file %s: %v\n", includePath, err)
					}
				}()
				continue
			}
		}

		newLines = append(newLines, line)
	}

	// Write the updated config back
	newContent := strings.Join(newLines, "\n")
	if err := os.WriteFile(cfg.PgBouncer.ConfFile, []byte(newContent), 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

func hasConnection(filePath string, connectionName string) bool {
	_, err := os.ReadFile(filePath)
	if err != nil {
		return false
	}

	// Parse as INI
	file, err := ini.LoadSources(ini.LoadOptions{
		IgnoreInlineComment: true,
	}, filePath)
	if err != nil {
		return false
	}

	// Check databases section
	dbSection := file.Section("databases")
	if dbSection == nil {
		return false
	}

	return dbSection.HasKey(connectionName)
}

func IsTargetConnected(cfg *config.Config, target string) (bool, error) {
	connections, err := GetConnectionDetails(cfg.PgBouncer.ConfFile)
	if err != nil {
		return false, fmt.Errorf("failed to get connection details: %w", err)
	}

	for _, conn := range connections {
		if conn.Name == target {
			return true, nil
		}
	}

	return false, nil
}
