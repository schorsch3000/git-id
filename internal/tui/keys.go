package tui

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"git-id/internal/store"
)

// sshKey is a private key found in an ssh directory.
type sshKey struct {
	Name        string // file name
	Path        string
	Public      string // path of Name.pub if it exists
	Fingerprint string // of the public key, if there is one
}

// discoverKeys lists the private keys directly inside dir, sorted by name. A
// file counts as a private key when its first line is a PEM/OpenSSH private
// key header; this skips known_hosts, config, *.pub and the like without
// guessing from file names.
func discoverKeys(dir string) ([]sshKey, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var keys []sshKey
	for _, e := range entries {
		path := filepath.Join(dir, e.Name())
		fi, err := os.Stat(path) // follows symlinks, as ssh does
		if err != nil || !fi.Mode().IsRegular() || !isPrivateKey(path) {
			continue
		}
		k := sshKey{Name: e.Name(), Path: path}
		if _, err := os.Stat(path + ".pub"); err == nil {
			k.Public = path + ".pub"
			k.Fingerprint = store.PublicFileFingerprint(k.Public)
		}
		keys = append(keys, k)
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

var badLabelChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// suggestLabel derives a label from a key file name ("id_ed25519" becomes
// "ed25519"); it returns "" when nothing valid comes out.
func suggestLabel(name string) string {
	l := strings.TrimPrefix(name, "id_")
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
