package pgbouncer

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"pgboundary/config"
	"pgboundary/internal/boundary"
)

func TestUpdateConfig_WritesToStableConnectionsDir(t *testing.T) {
	dir := t.TempDir()
	confPath := filepath.Join(dir, "pgbouncer.ini")
	if err := os.WriteFile(confPath, []byte("[pgbouncer]\nlisten_port = 5432\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		PgBouncer: config.PgBouncerConfig{
			WorkDir:  dir,
			ConfFile: confPath,
		},
		Targets: map[string]config.Target{
			"mytarget": {Host: "https://boundary.example.com", Target: "mytarget-ro", Database: "mydb"},
		},
	}

	conn := &boundary.Connection{Username: "u", Password: "p", Host: "127.0.0.1", Port: "5432", Pid: 4242}

	if err := UpdateConfig(cfg, "mytarget", conn); err != nil {
		t.Fatalf("UpdateConfig() error = %v", err)
	}

	wantConnFile := filepath.Join(dir, ".pgboundary-connections", "mytarget.ini")
	if _, err := os.Stat(wantConnFile); err != nil {
		t.Fatalf("expected connection file at %s, stat err = %v", wantConnFile, err)
	}

	confContent, err := os.ReadFile(confPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(confContent), wantConnFile) {
		t.Errorf("expected conf file to include %s, got: %s", wantConnFile, confContent)
	}

	// A second call for the same target overwrites rather than accumulating a new file.
	if err := UpdateConfig(cfg, "mytarget", conn); err != nil {
		t.Fatalf("UpdateConfig() second call error = %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(dir, ".pgboundary-connections"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("got %d files in connections dir, want 1 (retry should overwrite, not accumulate)", len(entries))
	}

	confContentAfterRetry, err := os.ReadFile(confPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(confContentAfterRetry), "%include "+wantConnFile); got != 1 {
		t.Errorf("got %d %%include lines for %s after retry, want 1", got, wantConnFile)
	}
}

func TestRollbackConnection_RemovesEntryAndKillsProcess(t *testing.T) {
	dir := t.TempDir()

	proc := exec.Command("sleep", "30")
	if err := proc.Start(); err != nil {
		t.Fatalf("failed to start test process: %v", err)
	}
	defer func() { _ = proc.Process.Kill() }()

	connPath := writeConnFile(t, dir, "mytarget", proc.Process.Pid)
	confPath := writeConfFile(t, dir, connPath)
	cfg := newTestConfig(confPath)

	if err := RollbackConnection(cfg, "mytarget", proc.Process.Pid); err != nil {
		t.Fatalf("RollbackConnection() error = %v", err)
	}

	if _, err := os.Stat(connPath); !os.IsNotExist(err) {
		t.Errorf("expected connection file to be removed, stat err = %v", err)
	}

	confContent, err := os.ReadFile(confPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(confContent), connPath) {
		t.Errorf("expected %%include to be removed from conf file, got: %s", confContent)
	}

	if err := proc.Wait(); err == nil {
		t.Errorf("expected killed process to exit with error, got nil")
	}
}

func TestRollbackConnection_StillKillsProcessWhenRemoveFails(t *testing.T) {
	dir := t.TempDir()

	proc := exec.Command("sleep", "30")
	if err := proc.Start(); err != nil {
		t.Fatalf("failed to start test process: %v", err)
	}
	defer func() { _ = proc.Process.Kill() }()

	cfg := newTestConfig(filepath.Join(dir, "does-not-exist.ini"))

	if err := RollbackConnection(cfg, "mytarget", proc.Process.Pid); err == nil {
		t.Fatal("expected RollbackConnection to return an error when removeConnection fails")
	}

	if waitErr := proc.Wait(); waitErr == nil {
		t.Errorf("expected process to have been killed even though removeConnection failed, got nil Wait() error")
	}
}
