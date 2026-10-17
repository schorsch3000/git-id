package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ResolveKeyPath expands a leading ~, makes the path absolute and checks that
// the file exists and is not a directory.
func ResolveKeyPath(p string) (string, error) {
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		p = filepath.Join(home, strings.TrimPrefix(p, "~"))
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	if fi, err := os.Stat(abs); err != nil {
		return "", fmt.Errorf("key file: %w", err)
	} else if fi.IsDir() {
		return "", fmt.Errorf("key file %s is a directory", abs)
	}
	return abs, nil
}
