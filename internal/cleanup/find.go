// Package cleanup finds commits whose author or committer names and email
// addresses belong to other identities of the address book and rewrites them
// to the identity of the current repository.
package cleanup

import (
	"fmt"
	"sort"
	"strings"

	"git-id/internal/gitcfg"
	"git-id/internal/store"
)

// Commit holds the identity fields of one commit.
type Commit struct {
	Hash                          string
	AuthorName, AuthorEmail       string
	CommitterName, CommitterEmail string
}

// Kind tells names and email addresses apart.
type Kind int

const (
	Name Kind = iota
	Email
)

func (k Kind) String() string {
	if k == Name {
		return "name"
	}
	return "email"
}

// Finding is a name or email address from the address book that appears in
// history although it is not the current identity's.
type Finding struct {
	Kind  Kind
	Value string
	Count int // commits carrying the value as author or committer
}

// Scan lists the commits reachable from ref, or from all refs if ref is empty.
func Scan(dir, ref string) ([]Commit, error) {
	args := []string{"log", "--format=%H%x1f%an%x1f%ae%x1f%cn%x1f%ce"}
	if ref == "" {
		args = append(args, "--all")
	} else {
		args = append(args, ref)
	}
	out, err := gitcfg.Git(dir, args...)
	if err != nil {
		return nil, err
	}
	var commits []Commit
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		f := strings.Split(line, "\x1f")
		if len(f) != 5 {
			return nil, fmt.Errorf("unexpected git log line %q", line)
		}
		commits = append(commits, Commit{f[0], f[1], f[2], f[3], f[4]})
	}
	return commits, nil
}

// Known is what Find compares history against: every name and email in the
// address book, and the identity that must stay as it is.
type Known struct {
	Names, Emails     map[string]bool // email keys are lower-case
	CurName, CurEmail string
}

// KnownFrom builds Known from the book and the repository's current identity.
func KnownFrom(b *store.Book, cur store.Resolved) Known {
	k := Known{Names: map[string]bool{}, Emails: map[string]bool{}, CurName: cur.Name, CurEmail: cur.Email}
	for _, n := range b.Names {
		k.Names[n] = true
	}
	for _, e := range b.Emails {
		k.Emails[strings.ToLower(e)] = true
	}
	return k
}

func (k Known) foreignName(n string) bool { return k.Names[n] && n != k.CurName }

func (k Known) foreignEmail(e string) bool {
	l := strings.ToLower(e)
	return k.Emails[l] && l != strings.ToLower(k.CurEmail)
}

// Find returns the foreign names and emails found in commits, most frequent
// first. Values not in the address book are never reported.
func Find(commits []Commit, k Known) (names, emails []Finding) {
	nc, ec := map[string]int{}, map[string]int{}
	for _, c := range commits {
		for _, n := range distinct(c.AuthorName, c.CommitterName) {
			if k.foreignName(n) {
				nc[n]++
			}
		}
		for _, e := range distinct(c.AuthorEmail, c.CommitterEmail) {
			if k.foreignEmail(e) {
				ec[e]++
			}
		}
	}
	return findings(Name, nc), findings(Email, ec)
}

func distinct(a, b string) []string {
	if a == b {
		return []string{a}
	}
	return []string{a, b}
}

func findings(kind Kind, counts map[string]int) []Finding {
	out := make([]Finding, 0, len(counts))
	for v, n := range counts {
		out = append(out, Finding{kind, v, n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Value < out[j].Value
	})
	return out
}

// Plan maps old values to the value they are replaced with.
type Plan struct {
	Names, Emails map[string]string
}

// NewPlan returns an empty plan.
func NewPlan() Plan { return Plan{Names: map[string]string{}, Emails: map[string]string{}} }

// Empty reports whether the plan changes nothing.
func (p Plan) Empty() bool { return len(p.Names) == 0 && len(p.Emails) == 0 }

// Add records that the finding is replaced with to.
func (p Plan) Add(f Finding, to string) {
	if f.Kind == Name {
		p.Names[f.Value] = to
	} else {
		p.Emails[f.Value] = to
	}
}

// Affected returns the hashes of the commits the plan changes.
func (p Plan) Affected(commits []Commit) []string {
	var out []string
	for _, c := range commits {
		_, an := p.Names[c.AuthorName]
		_, cn := p.Names[c.CommitterName]
		_, ae := p.Emails[c.AuthorEmail]
		_, ce := p.Emails[c.CommitterEmail]
		if an || cn || ae || ce {
			out = append(out, c.Hash)
		}
	}
	return out
}

// CountSigned returns how many commits reachable from ref (all refs if empty)
// carry a signature. git filter-repo drops the signature of every commit it
// rewrites, signed commits by other authors included, which also changes
// their hashes. The raw log is read instead of %G?, which would run a
// signature verification per commit.
func CountSigned(dir, ref string) (int, error) {
	args := []string{"log", "--format=raw"}
	if ref == "" {
		args = append(args, "--all")
	} else {
		args = append(args, ref)
	}
	out, err := gitcfg.Git(dir, args...)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "gpgsig ") || strings.HasPrefix(line, "gpgsig-sha256 ") {
			n++
		}
	}
	return n, nil
}
