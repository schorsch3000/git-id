package store

import (
	"fmt"
	"golang.org/x/crypto/ssh"
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

// Fingerprint returns the SHA256 fingerprint of the public key file of k, ""
// if there is none and "(unreadable)" if the file cannot be parsed as a public key.
func Fingerprint(k Key) string {
	if k.Public == "" {
		return ""
	}
	return PublicFileFingerprint(k.Public)
}

// PublicFileFingerprint is Fingerprint for a public key file path.
func PublicFileFingerprint(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return "(unreadable)"
	}
	pub, _, _, _, err := ssh.ParseAuthorizedKey(data)
	if err != nil {
		return "(unreadable)"
	}
	return ssh.FingerprintSHA256(pub)
}
