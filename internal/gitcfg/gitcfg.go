// Package gitcfg wraps the `git` binary for reading and writing git config.
package gitcfg

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// run executes git in dir (empty = current directory) and returns trimmed stdout.
func run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
		}
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, msg)
	}
	return strings.TrimRight(string(out), "\n"), nil
}

// exitCode returns git's exit status if err came from a failed git run.
func exitCode(err error) (int, bool) {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode(), true
	}
	return 0, false
}

// Toplevel returns the root of the work tree containing dir.
func Toplevel(dir string) (string, error) {
	return run(dir, "rev-parse", "--show-toplevel")
}

func get(dir, scope, key string) (string, bool, error) {
	v, err := run(dir, "config", scope, "--get", key)
	if err != nil {
		// exit status 1: key not set.
		if code, ok := exitCode(errors.Unwrap(err)); ok && code == 1 {
			return "", false, nil
		}
		return "", false, err
	}
	return v, true, nil
}

// GetLocal reads a key from the repository config of dir.
func GetLocal(dir, key string) (string, bool, error) { return get(dir, "--local", key) }

// GetGlobal reads a key from the global config.
func GetGlobal(key string) (string, bool, error) { return get("", "--global", key) }

// SetLocal writes a key to the repository config of dir.
func SetLocal(dir, key, value string) error {
	_, err := run(dir, "config", "--local", key, value)
	return err
}

// UnsetGlobal removes a key from the global config. A missing key is not an error.
func UnsetGlobal(key string) error {
	_, err := run("", "config", "--global", "--unset-all", key)
	if err != nil {
		// exit status 5: key did not exist.
		if code, ok := exitCode(errors.Unwrap(err)); ok && code == 5 {
			return nil
		}
	}
	return err
}

// Git runs an arbitrary git command in dir (empty = current directory) and
// returns its trimmed stdout.
func Git(dir string, args ...string) (string, error) { return run(dir, args...) }

// UnsetLocal removes a key from the repository config of dir. A missing key is not an error.
func UnsetLocal(dir, key string) error {
	_, err := run(dir, "config", "--local", "--unset-all", key)
	if err != nil {
		if code, ok := exitCode(errors.Unwrap(err)); ok && code == 5 {
			return nil
		}
	}
	return err
}
