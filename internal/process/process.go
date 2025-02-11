package process

import (
	"errors"
	"fmt"
	"strings"
	"syscall"

	"github.com/shirou/gopsutil/v4/process"
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
	proc, err := process.NewProcess(int32(pid))
	if err != nil {
		return false
	}

	name, err := proc.Name()
	if err != nil {
		return false
	}

	cmdline, err := proc.Cmdline()
	if err != nil {
		return false
	}

	matches := strings.EqualFold(name, processName) || strings.HasPrefix(cmdline, processName)
	if Verbose && matches {
		fmt.Printf("found %s process: %d\n", processName, pid)
	}
	return matches
}

// Processes returns a list of all running processes
func Processes() ([]*process.Process, error) {
	return process.Processes()
}
