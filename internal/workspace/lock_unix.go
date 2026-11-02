//go:build !windows

package workspace

import (
	"errors"
	"os"
	"syscall"
)

// processAlive reports whether pid exists. EPERM means it exists but belongs to
// another user; either way the lock is not abandoned.
func processAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = p.Signal(syscall.Signal(0))
	return err == nil || errors.Is(err, syscall.EPERM)
}
