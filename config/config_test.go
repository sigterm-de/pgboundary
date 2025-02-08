package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	// Create temporary test files
	tmpDir := t.TempDir()

	configContent := `[pgbouncer]
workdir = ./work
conffile = pgbouncer.ini

[scopes]
auth = auth
target = target

[targets]
app1 = host=https://boundary.example.com target=app1-ro
app2 = host=https://boundary.example-two.com target=app2-ro database=custom_db auth=auth1 scope=scope1
app3 = host=https://boundary.example.com target=app1-rw
app4 = host=https://boundary.example.com target=app1
`

	pgbouncerContent := `[pgbouncer]
pidfile = pgbouncer.pid
auth_file = userlist.txt
`

	configPath := filepath.Join(tmpDir, "config.ini")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create workdir and pgbouncer config
	workDir := filepath.Join(tmpDir, "work")
	if err := os.MkdirAll(workDir, 0755); err != nil {
		t.Fatal(err)
	}

	pgbouncerPath := filepath.Join(workDir, "pgbouncer.ini")
	if err := os.WriteFile(pgbouncerPath, []byte(pgbouncerContent), 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		path    string
		want    *Config
		wantErr bool
	}{
		{
			name: "valid config",
			path: configPath,
			want: &Config{
				PgBouncer: struct {
					WorkDir  string
					ConfFile string
					PidFile  string
					AuthFile string
				}{
					WorkDir:  filepath.Join(tmpDir, "work"),
					ConfFile: "pgbouncer.ini",
					PidFile:  "pgbouncer.pid",
					AuthFile: "userlist.txt",
				},
				Scopes: struct {
					Auth   string
					Target string
				}{
					Auth:   "auth",
					Target: "target",
				},
				Targets: map[string]Target{
					"app1": {
						Host:     "https://boundary.example.com",
						Target:   "app1-ro",
						Database: "app1",
					},
					"app2": {
						Host:     "https://boundary.example-two.com",
						Target:   "app2-ro",
						Database: "custom_db",
						Auth:     "auth1",
						Scope:    "scope1",
					},
					"app3": {
						Host:     "https://boundary.example.com",
						Target:   "app1-rw",
						Database: "app1",
					},
					"app4": {
						Host:     "https://boundary.example.com",
						Target:   "app1",
						Database: "app1",
					},
				},
			},
			wantErr: false,
		},
		{
			name:    "non-existent file",
			path:    "nonexistent.ini",
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := LoadConfig(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			// Compare config fields
			if got.PgBouncer.WorkDir != tt.want.PgBouncer.WorkDir {
				t.Errorf("WorkDir = %v, want %v", got.PgBouncer.WorkDir, tt.want.PgBouncer.WorkDir)
			}
			if got.PgBouncer.ConfFile != tt.want.PgBouncer.ConfFile {
				t.Errorf("ConfFile = %v, want %v", got.PgBouncer.ConfFile, tt.want.PgBouncer.ConfFile)
			}
			if got.PgBouncer.PidFile != tt.want.PgBouncer.PidFile {
				t.Errorf("PidFile = %v, want %v", got.PgBouncer.PidFile, tt.want.PgBouncer.PidFile)
			}
			if got.PgBouncer.AuthFile != tt.want.PgBouncer.AuthFile {
				t.Errorf("AuthFile = %v, want %v", got.PgBouncer.AuthFile, tt.want.PgBouncer.AuthFile)
			}

			// Compare targets
			if len(got.Targets) != len(tt.want.Targets) {
				t.Errorf("got %d targets, want %d", len(got.Targets), len(tt.want.Targets))
			}
			for name, target := range tt.want.Targets {
				gotTarget, exists := got.Targets[name]
				if !exists {
					t.Errorf("target %s not found", name)
					continue
				}
				if gotTarget != target {
					t.Errorf("target %s = %+v, want %+v", name, gotTarget, target)
				}
			}
		})
	}
}

func TestParseTarget(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		value   string
		want    Target
		wantErr bool
	}{
		{
			name:  "minimal target",
			key:   "app1",
			value: "host=https://boundary.example.com target=app1-ro",
			want: Target{
				Host:     "https://boundary.example.com",
				Target:   "app1-ro",
				Database: "app1",
			},
			wantErr: false,
		},
		{
			name:  "full target",
			key:   "app2",
			value: "host=https://boundary.example-two.com target=app2-ro database=custom_db auth=auth1 scope=scope1",
			want: Target{
				Host:     "https://boundary.example-two.com",
				Target:   "app2-ro",
				Database: "custom_db",
				Auth:     "auth1",
				Scope:    "scope1",
			},
			wantErr: false,
		},
		{
			name:  "target with rw suffix",
			key:   "app3",
			value: "host=https://boundary.example.com target=app1-rw",
			want: Target{
				Host:     "https://boundary.example.com",
				Target:   "app1-rw",
				Database: "app1",
			},
			wantErr: false,
		},
		{
			name:  "target without ro/rw suffix",
			key:   "app4",
			value: "host=https://boundary.example.com target=app1",
			want: Target{
				Host:     "https://boundary.example.com",
				Target:   "app1",
				Database: "app1",
			},
			wantErr: false,
		},
		{
			name:    "missing host",
			key:     "invalid1",
			value:   "target=app1-ro",
			want:    Target{},
			wantErr: true,
		},
		{
			name:    "missing target",
			key:     "invalid2",
			value:   "host=https://boundary.example.com",
			want:    Target{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseTarget(tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseTarget() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if got != tt.want {
				t.Errorf("parseTarget() = %v, want %v", got, tt.want)
			}
		})
	}
}
