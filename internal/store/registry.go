package store

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// RegistryPath returns the repo registry file living next to the book.
func RegistryPath(bookPath string) string {
	return filepath.Join(filepath.Dir(bookPath), "repos.txt")
}

// LoadRepos reads the registered repository paths. A missing file yields none.
func LoadRepos(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, l := range strings.Split(string(data), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out, nil
}

// SaveRepos writes the repository paths sorted and deduplicated.
func SaveRepos(path string, repos []string) error {
	seen := map[string]bool{}
	var uniq []string
	for _, r := range repos {
		if !seen[r] {
			seen[r] = true
			uniq = append(uniq, r)
		}
	}
	sort.Strings(uniq)
	data := strings.Join(uniq, "\n")
	if data != "" {
		data += "\n"
	}
	return writeAtomic(path, []byte(data))
}

// AddRepo registers a repository path.
func AddRepo(path, repo string) error {
	repos, err := LoadRepos(path)
	if err != nil {
		return err
	}
	return SaveRepos(path, append(repos, repo))
}
