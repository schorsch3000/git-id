package cli

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func decode[T any](t *testing.T, out string) T {
	t.Helper()
	var v T
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	return v
}

// realPublicKey writes a valid public key file and returns its path and fingerprint.
func realPublicKey(t *testing.T, dir string) (path, fp string) {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	sp, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	path = filepath.Join(dir, "real.pub")
	if err := os.WriteFile(path, ssh.MarshalAuthorizedKey(sp), 0o644); err != nil {
		t.Fatal(err)
	}
	return path, ssh.FingerprintSHA256(sp)
}

func TestJSONListings(t *testing.T) {
	book, key, pub := setup(t)
	mustRun(t, book, "name", "add", "me", "Dirk Heilig")
	mustRun(t, book, "email", "add", "work", "dirk@example.com")
	mustRun(t, book, "key", "add", "k1", key, "--public", pub)
	mustRun(t, book, "add", "work", "--name", "me", "--email", "work", "--key", "k1")

	if got := decode[[]valueRow](t, mustRun(t, book, "name", "ls", "--json")); len(got) != 1 || got[0] != (valueRow{"me", "Dirk Heilig"}) {
		t.Errorf("name ls = %+v", got)
	}
	if got := decode[[]valueRow](t, mustRun(t, book, "email", "ls", "--json")); len(got) != 1 || got[0] != (valueRow{"work", "dirk@example.com"}) {
		t.Errorf("email ls = %+v", got)
	}
	keys := decode[[]keyRow](t, mustRun(t, book, "key", "ls", "--json"))
	if len(keys) != 1 || keys[0].Label != "k1" || keys[0].Private != key || keys[0].Public != pub {
		t.Errorf("key ls = %+v", keys)
	}
	if got := decode[[]identityRow](t, mustRun(t, book, "ls", "--json")); len(got) != 1 || got[0] != (identityRow{"work", "me", "work", "k1"}) {
		t.Errorf("ls = %+v", got)
	}
	d := decode[identityDetail](t, mustRun(t, book, "show", "work", "--json"))
	if d.Identity != "work" || d.Name != "Dirk Heilig" || d.Email != "dirk@example.com" || d.Key != key {
		t.Errorf("show = %+v", d)
	}
}

func TestJSONEmptyListsAreArrays(t *testing.T) {
	book, _, _ := setup(t)
	for _, args := range [][]string{{"ls"}, {"name", "ls"}, {"email", "ls"}, {"key", "ls"}, {"repos", "ls"}} {
		out := strings.TrimSpace(mustRun(t, book, append(args, "--json")...))
		if out != "[]" {
			t.Errorf("git id %v --json = %q, want []", args, out)
		}
	}
	if _, err := run(t, book, "show", "ghost", "--json"); err == nil {
		t.Error("show of a missing identity succeeded")
	}
}

func TestReposJSON(t *testing.T) {
	isolate(t)
	book, _ := seed(t)
	repo := newRepo(t)
	t.Chdir(repo)
	mustRun(t, book, "use", "work")
	got := decode[[]repoInfo](t, mustRun(t, book, "repos", "ls", "--json"))
	if len(got) != 1 || got[0] != (repoInfo{repo, "work", "ok"}) {
		t.Errorf("repos ls = %+v", got)
	}
}

func TestFingerprints(t *testing.T) {
	book, key, bad := setup(t) // bad: a ".pub" file that is not a public key
	real, fp := realPublicKey(t, filepath.Dir(key))
	mustRun(t, book, "key", "add", "good", key, "--public", real)
	mustRun(t, book, "key", "add", "broken", key, "--public", bad)
	mustRun(t, book, "key", "add", "nopub", key)

	out := mustRun(t, book, "key", "ls")
	if l := lineWith(out, "good"); !strings.Contains(l, fp) {
		t.Errorf("good key line lacks fingerprint %s: %q", fp, l)
	}
	if l := lineWith(out, "broken"); !strings.Contains(l, "(unreadable)") {
		t.Errorf("broken key line: %q", l)
	}
	if l := lineWith(out, "nopub"); strings.Contains(l, "SHA256") || strings.Contains(l, "unreadable") {
		t.Errorf("key without public file shows a fingerprint: %q", l)
	}

	keys := decode[[]keyRow](t, mustRun(t, book, "key", "ls", "--json"))
	byLabel := map[string]keyRow{}
	for _, k := range keys {
		byLabel[k.Label] = k
	}
	if byLabel["good"].Fingerprint != fp || byLabel["nopub"].Fingerprint != "" {
		t.Errorf("json fingerprints = %+v", byLabel)
	}

	mustRun(t, book, "name", "add", "me", "A")
	mustRun(t, book, "email", "add", "e", "a@b.c")
	mustRun(t, book, "add", "x", "--name", "me", "--email", "e", "--key", "good")
	if show := mustRun(t, book, "show", "x"); !strings.Contains(show, "fingerprint: "+fp) {
		t.Errorf("show lacks the fingerprint:\n%s", show)
	}
	if d := decode[identityDetail](t, mustRun(t, book, "show", "x", "--json")); d.Fingerprint != fp {
		t.Errorf("show --json fingerprint = %q", d.Fingerprint)
	}
}

func TestActionCommandsPrintJSON(t *testing.T) {
	isolate(t)
	book, _ := seed(t)
	repo := newRepo(t)
	t.Chdir(repo)

	u := decode[useResult](t, mustRun(t, book, "use", "work", "--json"))
	if u != (useResult{"work", repo}) {
		t.Errorf("use = %+v", u)
	}
	c := decode[currentResult](t, mustRun(t, book, "current", "--json"))
	if c != (currentResult{"work", "Dirk Heilig", "dirk@example.com", repo}) {
		t.Errorf("current = %+v", c)
	}
	// quiet and json together make no sense
	if _, err := run(t, book, "current", "-q", "--json"); err == nil {
		t.Error("-q with --json accepted")
	}

	gone := newRepo(t)
	t.Chdir(gone)
	mustRun(t, book, "use", "work")
	t.Chdir(repo)
	if err := os.RemoveAll(gone); err != nil {
		t.Fatal(err)
	}
	s := decode[syncResult](t, mustRun(t, book, "sync", "--json"))
	if len(s.Repositories) != 2 {
		t.Fatalf("sync = %+v", s)
	}
	got := map[string]syncRepo{}
	for _, r := range s.Repositories {
		got[r.Path] = r
	}
	if got[repo] != (syncRepo{Path: repo, Identity: "work", Status: "synced"}) || got[gone].Status != "dropped" {
		t.Errorf("sync repositories = %+v", got)
	}

	r := decode[renameResult](t, mustRun(t, book, "rename", "work", "company", "--json"))
	if r.From != "work" || r.To != "company" || len(r.Updated) != 1 || r.Updated[0] != repo {
		t.Errorf("rename = %+v", r)
	}

	un := decode[unuseResult](t, mustRun(t, book, "unuse", "--json"))
	if un != (unuseResult{Repository: repo}) {
		t.Errorf("unuse = %+v", un)
	}
}

func TestSyncJSONReportsFailuresAndStillFails(t *testing.T) {
	isolate(t)
	book, _ := seed(t)
	repo := newRepo(t)
	t.Chdir(repo)
	mustRun(t, book, "use", "work")
	mustRun(t, book, "rm", "work")
	var out strings.Builder
	cmd := NewRootCmd()
	cmd.SetOut(&out)
	cmd.SetErr(new(strings.Builder))
	cmd.SetArgs([]string{"--book", book, "sync", "--json"})
	if err := cmd.Execute(); err == nil {
		t.Error("sync with a dangling identity succeeded")
	}
	s := decode[syncResult](t, out.String())
	if len(s.Repositories) != 1 || s.Repositories[0].Status != "failed" || s.Repositories[0].Error == "" {
		t.Errorf("sync = %+v", s)
	}
}

func TestRemovalAndInitJSON(t *testing.T) {
	book, key, _ := setup(t)
	mustRun(t, book, "name", "add", "me", "A")
	mustRun(t, book, "email", "add", "e", "a@b.c")
	mustRun(t, book, "key", "add", "k", key)
	mustRun(t, book, "add", "x", "--name", "me", "--email", "e", "--key", "k")
	if _, err := run(t, book, "email", "rm", "e", "--json"); err == nil {
		t.Error("removing a used email without --force succeeded")
	}
	r := decode[removedResult](t, mustRun(t, book, "email", "rm", "e", "--force", "--json"))
	if len(r.RemovedIdentities) != 1 || r.RemovedIdentities[0] != "x" {
		t.Errorf("rm = %+v", r)
	}
	mustRun(t, book, "name", "add", "spare", "S")
	if r := decode[removedResult](t, mustRun(t, book, "name", "rm", "spare", "--json")); r.RemovedIdentities == nil || len(r.RemovedIdentities) != 0 {
		t.Errorf("rm without dependents = %+v, want an empty list", r)
	}

	home := isolate(t)
	gitRun(t, home, "config", "--global", "user.name", "Dirk Heilig")
	gitRun(t, home, "config", "--global", "user.email", "dirk@example.org")
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	k2 := filepath.Join(home, ".ssh", "id_ed25519")
	if err := os.WriteFile(k2, []byte("k"), 0o600); err != nil {
		t.Fatal(err)
	}
	in := decode[initResult](t, mustRun(t, filepath.Join(home, "book.toml"), "init", "--json"))
	want := initResult{Label: "default", Name: "Dirk Heilig", Email: "dirk@example.org", Key: k2,
		IdentityCreated: true, RemovedGlobal: true, UseConfigOnly: true}
	if in != want {
		t.Errorf("init = %+v, want %+v", in, want)
	}
}
