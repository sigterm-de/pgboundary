package process

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"
)

// Verbose enables debug output
var Verbose bool

// KillProcess attempts to kill a process with the given PID
func KillProcess(pid int) error {
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		if !errors.Is(err, syscall.ESRCH) { // Ignore "no such process" errors
			return fmt.Errorf("failed to kill process %d: %w", pid, err)
		}
	}
	return nil
}

// IsProcessType checks if a process with given PID is of the expected type
func IsProcessType(pid int, processName string) bool {
	cmdlinePath := fmt.Sprintf("/proc/%d/cmdline", pid)
	cmdline, err := os.ReadFile(cmdlinePath)
	if err != nil {
		return false
	}
	if Verbose && strings.HasPrefix(string(cmdline), processName) {
		fmt.Printf("found %s process: %d\n", processName, pid)
	}
	return strings.HasPrefix(string(cmdline), processName)
}
