package pgbouncer

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"pgboundary/config"
	"pgboundary/internal/process"
)

// RemovedEntry describes a stale %include entry that Reconcile removed.
type RemovedEntry struct {
	Target string
	Reason string
}

// boundaryProcessAlive is overridden in tests to avoid depending on a real
// "boundary" process existing on the machine.
var boundaryProcessAlive = func(pid int) bool {
	return process.IsProcessType(pid, "boundary")
}

// Reconcile scans cfg.PgBouncer.ConfFile for %include entries whose
// connection file is missing or whose backing boundary process is no
// longer running, and removes them. If pgbouncer is currently running and
// anything was removed, it is reloaded so its in-memory config matches the
// file on disk.
func Reconcile(cfg *config.Config) ([]RemovedEntry, error) {
	content, err := os.ReadFile(cfg.PgBouncer.ConfFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read pgbouncer config: %w", err)
	}

	confDir := filepath.Dir(cfg.PgBouncer.ConfFile)

	lines := strings.Split(string(content), "\n")
	newLines := make([]string, 0, len(lines))
	var removed []RemovedEntry
	var stalePaths []string
	changed := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "%include") {
			includePath := strings.TrimSpace(strings.TrimPrefix(trimmed, "%include"))
			resolvedPath := includePath
			if !filepath.IsAbs(resolvedPath) {
				resolvedPath = filepath.Join(confDir, resolvedPath)
			}
			if entry, stale := evaluateInclude(resolvedPath); stale {
				changed = true
				stalePaths = append(stalePaths, resolvedPath)
				removed = append(removed, entry)
				continue
			}
		}
		newLines = append(newLines, line)
	}

	if !changed {
		return nil, nil
	}

	newContent := strings.Join(newLines, "\n")
	if err := os.WriteFile(cfg.PgBouncer.ConfFile, []byte(newContent), 0644); err != nil {
		return nil, fmt.Errorf("failed to write pgbouncer config: %w", err)
	}

	for _, path := range stalePaths {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			fmt.Printf("Warning: failed to remove stale connection file %s: %v\n", path, err)
		}
	}

	if running, _, err := CheckStatus(cfg.PgBouncer.PidFile); err == nil && running {
		if err := Reload(cfg); err != nil {
			return removed, fmt.Errorf("failed to reload pgbouncer after reconciling stale entries: %w", err)
		}
	}

	return removed, nil
}

// evaluateInclude decides whether the %include target at includePath is
// stale. It returns the entry to report and true if it should be dropped.
func evaluateInclude(includePath string) (RemovedEntry, bool) {
	connections, err := parseIncludedFile(includePath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return RemovedEntry{Target: includePath, Reason: "missing file"}, true
		}
		fmt.Printf("Warning: skipping unreadable include file %s: %v\n", includePath, err)
		return RemovedEntry{}, false
	}

	for _, conn := range connections {
		if conn.BoundaryPid > 0 && !boundaryProcessAlive(conn.BoundaryPid) {
			return RemovedEntry{Target: conn.Name, Reason: "dead boundary process"}, true
		}
	}
	return RemovedEntry{}, false
}
