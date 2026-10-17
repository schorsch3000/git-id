//go:build !windows

package cli

import (
	"os"
	"os/exec"
	"syscall"
)

// runSSH replaces the process with ssh, so signals, stdio and the exit status
// are ssh's own.
func runSSH(argv []string) error {
	bin, err := exec.LookPath(argv[0])
	if err != nil {
		return err
	}
	return syscall.Exec(bin, argv, os.Environ())
}
