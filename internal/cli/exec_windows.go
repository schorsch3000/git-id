//go:build windows

package cli

import (
	"errors"
	"os"
	"os/exec"
)

// runSSH runs ssh as a child process (Windows cannot replace the process) and
// exits with its status.
func runSSH(argv []string) error {
	bin, err := exec.LookPath(argv[0])
	if err != nil {
		return err
	}
	cmd := exec.Command(bin, argv[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			os.Exit(ee.ExitCode())
		}
		return err
	}
	return nil
}
