package store

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"

	"github.com/pelletier/go-toml/v2"
)

// DefaultPath returns $XDG_CONFIG_HOME/git-id/book.toml, falling back to
// ~/.config/git-id/book.toml (also on macOS, like other command line tools)
// and to the user config directory (%AppData%) on Windows.
func DefaultPath() (string, error) {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		var err error
		if runtime.GOOS == "windows" {
			dir, err = os.UserConfigDir()
		} else {
			var home string
			if home, err = os.UserHomeDir(); err == nil {
				dir = filepath.Join(home, ".config")
			}
		}
		if err != nil {
			return "", err
		}
	}
	return filepath.Join(dir, "git-id", "book.toml"), nil
}

// Load reads the book at path. A missing file yields an empty book.
func Load(path string) (*Book, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return New(), nil
	}
	if err != nil {
		return nil, err
	}
	b := &Book{}
	if err := toml.Unmarshal(data, b); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	b.init()
	if err := b.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return b, nil
}

// Save validates and atomically writes the book (dir 0700, file 0600).
func Save(path string, b *Book) error {
	if err := b.Validate(); err != nil {
		return err
	}
	data, err := toml.Marshal(b)
	if err != nil {
		return err
	}
	return writeAtomic(path, data)
}

// writeAtomic writes data via a temp file and rename (dir 0700, file 0600).
func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// CreateTemp already uses 0600.
	return os.Rename(tmp.Name(), path)
}
