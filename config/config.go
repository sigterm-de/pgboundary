package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/ini.v1"
)

type Config struct {
	PgBouncer PgBouncerConfig
	Scopes    ScopesConfig
	Auth      AuthConfig
	Targets   map[string]Target
}

type PgBouncerConfig struct {
	WorkDir  string
	ConfFile string
	PidFile  string
	AuthFile string
}

type ScopesConfig struct {
	Auth   string
	Target string
}

type Target struct {
	Host     string
	Target   string
	Database string
	Auth     string
	Scope    string
}

type AuthConfig struct {
	Method string
}

func LoadConfig(path string) (*Config, error) {
	cfg := &Config{
		Targets: make(map[string]Target),
	}

	file, err := ini.Load(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Get absolute path of config file for relative path resolution
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}
	configDir := filepath.Dir(absPath)

	// Load basic configuration
	cfg.PgBouncer.WorkDir = file.Section("pgbouncer").Key("workdir").String()
	cfg.PgBouncer.ConfFile = file.Section("pgbouncer").Key("conffile").String()
	cfg.Scopes.Auth = file.Section("scopes").Key("auth").String()
	cfg.Scopes.Target = file.Section("scopes").Key("target").String()

	// Load auth configuration
	cfg.Auth.Method = file.Section("auth").Key("method").String()
	if cfg.Auth.Method == "" {
		cfg.Auth.Method = "oidc" // default to oidc if not specified
	}

	// Resolve workdir path
	if filepath.IsAbs(cfg.PgBouncer.WorkDir) {
		cfg.PgBouncer.WorkDir = filepath.Clean(cfg.PgBouncer.WorkDir)
	} else {
		cfg.PgBouncer.WorkDir = filepath.Clean(filepath.Join(configDir, cfg.PgBouncer.WorkDir))
	}

	// Parse targets
	targetsSection := file.Section("targets")
	for _, key := range targetsSection.Keys() {
		target, err := parseTarget(key.String())
		if err != nil {
			return nil, fmt.Errorf("failed to parse target %s: %w", key.Name(), err)
		}
		cfg.Targets[key.Name()] = target
	}

	// Load and parse pgbouncer config
	pgBouncerConfPath := filepath.Join(cfg.PgBouncer.WorkDir, cfg.PgBouncer.ConfFile)
	if err := cfg.loadPgBouncerConfig(pgBouncerConfPath); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) loadPgBouncerConfig(path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read pgbouncer config: %w", err)
	}

	// Parse the file line by line to extract just the values we need
	lines := strings.Split(string(content), "\n")
	inPgBouncerSection := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip empty lines, comments and includes
		if trimmed == "" || strings.HasPrefix(trimmed, ";") || strings.HasPrefix(trimmed, "%include") {
			continue
		}

		// Check for section
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			section := strings.TrimPrefix(strings.TrimSuffix(trimmed, "]"), "[")
			inPgBouncerSection = section == "pgbouncer"
			continue
		}

		// Parse key-value pairs in pgbouncer section
		if inPgBouncerSection {
			parts := strings.SplitN(trimmed, "=", 2)
			if len(parts) != 2 {
				continue
			}
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])

			switch key {
			case "pidfile":
				c.PgBouncer.PidFile = value
			case "auth_file":
				c.PgBouncer.AuthFile = value
			}
		}
	}

	if c.PgBouncer.PidFile == "" {
		return fmt.Errorf("pidfile not found in pgbouncer config")
	}

	return nil
}

func parseTarget(value string) (Target, error) {
	target := Target{}

	// Split the string by spaces and parse key=value pairs
	for _, part := range strings.Fields(value) {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}

		switch kv[0] {
		case "host":
			target.Host = kv[1]
		case "target":
			target.Target = kv[1]
		case "database":
			target.Database = kv[1]
		case "auth":
			target.Auth = kv[1]
		case "scope":
			target.Scope = kv[1]
		}
	}

	// If database is not explicitly set, derive it from target name
	if target.Database == "" {
		target.Database = strings.TrimSuffix(target.Target, "-ro")
		target.Database = strings.ReplaceAll(target.Database, "-", "")
	}

	// Validate required fields
	if target.Host == "" || target.Target == "" {
		return Target{}, fmt.Errorf("target must have at least host and target fields")
	}

	// Validate that host starts with https://
	if !strings.HasPrefix(target.Host, "https://") {
		return Target{}, fmt.Errorf("host must start with https:// (got: %s)", target.Host)
	}

	return target, nil
}
