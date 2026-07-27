package pgbouncer

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"pgboundary/config"
)

func newTestConfig(confFile string) *config.Config {
	return &config.Config{
		PgBouncer: config.PgBouncerConfig{
			WorkDir:  filepath.Dir(confFile),
			ConfFile: confFile,
			PidFile:  filepath.Join(filepath.Dir(confFile), "pgbouncer.pid"),
		},
	}
}

func writeConnFile(t *testing.T, dir, target string, pid int) string {
	t.Helper()
	path := filepath.Join(dir, target+".ini")
	content := "; boundary_pid=" + strconv.Itoa(pid) + "\n[databases]\n" +
		target + " = host=127.0.0.1 port=5432 dbname=db user=u password=p"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeConfFile(t *testing.T, dir string, includes ...string) string {
	t.Helper()
	var sb strings.Builder
	sb.WriteString("[pgbouncer]\nlisten_port = 5432\n\n")
	for _, inc := range includes {
		sb.WriteString("%include " + inc + "\n")
	}
	path := filepath.Join(dir, "pgbouncer.ini")
	if err := os.WriteFile(path, []byte(sb.String()), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReconcile_RemovesMissingFile(t *testing.T) {
	dir := t.TempDir()
	missingPath := filepath.Join(dir, "gone.ini") // never created
	confPath := writeConfFile(t, dir, missingPath)
	cfg := newTestConfig(confPath)

	removed, err := Reconcile(cfg)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if len(removed) != 1 || removed[0].Reason != "missing file" {
		t.Fatalf("removed = %+v, want one entry with reason 'missing file'", removed)
	}

	content, err := os.ReadFile(confPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), missingPath) {
		t.Errorf("conf file still contains stale include: %s", content)
	}
}

func TestReconcile_RemovesDeadBoundaryProcess(t *testing.T) {
	dir := t.TempDir()
	connPath := writeConnFile(t, dir, "mytarget", 999999)
	confPath := writeConfFile(t, dir, connPath)
	cfg := newTestConfig(confPath)

	original := boundaryProcessAlive
	boundaryProcessAlive = func(pid int) bool { return false }
	defer func() { boundaryProcessAlive = original }()

	removed, err := Reconcile(cfg)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if len(removed) != 1 || removed[0].Target != "mytarget" || removed[0].Reason != "dead boundary process" {
		t.Fatalf("removed = %+v, want mytarget/dead boundary process", removed)
	}
	if _, err := os.Stat(connPath); !os.IsNotExist(err) {
		t.Errorf("expected connection file to be deleted, stat err = %v", err)
	}
}

func TestReconcile_KeepsLiveConnection(t *testing.T) {
	dir := t.TempDir()
	connPath := writeConnFile(t, dir, "mytarget", 123)
	confPath := writeConfFile(t, dir, connPath)
	cfg := newTestConfig(confPath)

	original := boundaryProcessAlive
	boundaryProcessAlive = func(pid int) bool { return true }
	defer func() { boundaryProcessAlive = original }()

	removed, err := Reconcile(cfg)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if len(removed) != 0 {
		t.Fatalf("removed = %+v, want none", removed)
	}
	if _, err := os.Stat(connPath); err != nil {
		t.Errorf("expected connection file to survive, stat err = %v", err)
	}

	content, err := os.ReadFile(confPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), connPath) {
		t.Errorf("expected conf file to still include %s, got: %s", connPath, content)
	}
}

func TestReconcile_SkipsUnreadableInclude(t *testing.T) {
	dir := t.TempDir()
	badPath := filepath.Join(dir, "not-a-file")
	if err := os.Mkdir(badPath, 0755); err != nil {
		t.Fatal(err)
	}
	confPath := writeConfFile(t, dir, badPath)
	cfg := newTestConfig(confPath)

	removed, err := Reconcile(cfg)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if len(removed) != 0 {
		t.Fatalf("removed = %+v, want none (unreadable include should be left alone, not treated as missing)", removed)
	}
}

func TestReconcile_ResolvesRelativeIncludePath(t *testing.T) {
	dir := t.TempDir()
	connPath := writeConnFile(t, dir, "reltarget", 123)
	relIncludePath := filepath.Base(connPath)
	confPath := writeConfFile(t, dir, relIncludePath)
	cfg := newTestConfig(confPath)

	original := boundaryProcessAlive
	boundaryProcessAlive = func(pid int) bool { return true }
	defer func() { boundaryProcessAlive = original }()

	removed, err := Reconcile(cfg)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if len(removed) != 0 {
		t.Fatalf("removed = %+v, want none (relative include pointing at a live, existing file must not be treated as missing)", removed)
	}

	content, err := os.ReadFile(confPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), relIncludePath) {
		t.Errorf("expected conf file to still include the relative path, got: %s", content)
	}
}

func TestReconcile_MixedLiveAndStale(t *testing.T) {
	dir := t.TempDir()
	liveConnPath := writeConnFile(t, dir, "livetarget", 111)
	deadConnPath := writeConnFile(t, dir, "deadtarget", 999)
	confPath := writeConfFile(t, dir, liveConnPath, deadConnPath)
	cfg := newTestConfig(confPath)

	original := boundaryProcessAlive
	boundaryProcessAlive = func(pid int) bool { return pid == 111 }
	defer func() { boundaryProcessAlive = original }()

	removed, err := Reconcile(cfg)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if len(removed) != 1 || removed[0].Target != "deadtarget" {
		t.Fatalf("removed = %+v, want exactly deadtarget", removed)
	}

	if _, err := os.Stat(liveConnPath); err != nil {
		t.Errorf("expected live connection file to survive, stat err = %v", err)
	}
	if _, err := os.Stat(deadConnPath); !os.IsNotExist(err) {
		t.Errorf("expected dead connection file to be removed, stat err = %v", err)
	}

	content, err := os.ReadFile(confPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), liveConnPath) {
		t.Errorf("expected conf file to still include the live connection, got: %s", content)
	}
	if strings.Contains(string(content), deadConnPath) {
		t.Errorf("expected conf file to no longer include the dead connection, got: %s", content)
	}
}

func TestReconcile_NoChangesReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	confPath := writeConfFile(t, dir)
	cfg := newTestConfig(confPath)

	removed, err := Reconcile(cfg)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if len(removed) != 0 {
		t.Fatalf("removed = %+v, want none", removed)
	}
}
