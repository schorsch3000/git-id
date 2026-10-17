package tui

import (
	"bufio"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/crypto/ssh"

	"git-id/internal/store"
)

// maxKeyDepth is how many directory levels below the ssh directory are searched
// (keys are often grouped as ~/.ssh/work/id_ed25519).
const maxKeyDepth = 2

// sshKey is a private key found in an ssh directory.
type sshKey struct {
	Name        string // path relative to the searched directory
	Path        string
	Public      string // path of Path.pub if it exists
	Fingerprint string // of the public key, if there is one
	Encrypted   bool   // protected by a passphrase
}

// discoverKeys lists the private keys in dir and up to maxKeyDepth levels of
// subdirectories, sorted by name. A file counts as a private key when its first
// line is a PEM/OpenSSH private key header; this skips known_hosts, config,
// *.pub and the like without guessing from file names. PuTTY .ppk files are
// not listed: OpenSSH cannot use them.
func discoverKeys(dir string) ([]sshKey, error) {
	if _, err := os.ReadDir(dir); err != nil {
		return nil, err
	}
	var keys []sshKey
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() {
				return fs.SkipDir // an unreadable subdirectory is just skipped
			}
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		if d.IsDir() {
			if rel != "." && strings.Count(rel, string(filepath.Separator)) >= maxKeyDepth {
				return fs.SkipDir
			}
			return nil
		}
		fi, err := os.Stat(path) // follows symlinks, as ssh does
		if err != nil || !fi.Mode().IsRegular() || !isPrivateKey(path) {
			return nil
		}
		k := sshKey{Name: filepath.ToSlash(rel), Path: path, Encrypted: isEncrypted(path)}
		if _, err := os.Stat(path + ".pub"); err == nil {
			k.Public = path + ".pub"
			k.Fingerprint = store.PublicFileFingerprint(k.Public)
		}
		keys = append(keys, k)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].Name < keys[j].Name })
	return keys, nil
}

func isPrivateKey(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	line, _ := bufio.NewReaderSize(f, 128).ReadString('\n')
	line = strings.TrimSpace(line)
	return strings.HasPrefix(line, "-----BEGIN ") && strings.HasSuffix(line, "PRIVATE KEY-----")
}

// isEncrypted reports whether the key needs a passphrase. Keys that do not
// parse for another reason count as not encrypted.
func isEncrypted(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil || len(data) > 64*1024 {
		return false
	}
	_, err = ssh.ParseRawPrivateKey(data)
	var missing *ssh.PassphraseMissingError
	return errors.As(err, &missing)
}

var badLabelChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// suggestLabel derives a label from a key file name ("id_ed25519" becomes
// "ed25519", "work/id_rsa" becomes "rsa"); it returns "" when nothing valid
// comes out.
func suggestLabel(name string) string {
	l := strings.TrimPrefix(filepath.Base(name), "id_")
	l = strings.Trim(badLabelChars.ReplaceAllString(l, "-"), "-.")
	if store.ValidateLabel(l) != nil {
		return ""
	}
	return l
}

// picker is the "choose a key from the ssh directory" list.
type picker struct {
	dir    string
	keys   []sshKey
	cursor int
	err    string
}
