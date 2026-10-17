package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"git-id/internal/gitcfg"
	idrepo "git-id/internal/repo"
	"git-id/internal/store"
)

// isolate points git at a throwaway HOME and global config.
func isolate(t *testing.T) (home string) {
	t.Helper()
	home = t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, ".gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv(bookEnv, "")
	return home
}

func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func newRepo(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "init", "-q")
	return dir
}

// seed creates identity "work" in a fresh book.
func seed(t *testing.T) (book, key string) {
	t.Helper()
	book, key, _ = setup(t)
	mustRun(t, book, "name", "add", "me", "Dirk Heilig")
	mustRun(t, book, "email", "add", "work", "dirk@example.com")
	mustRun(t, book, "key", "add", "k1", key)
	mustRun(t, book, "add", "work", "--name", "me", "--email", "work", "--key", "k1")
	return book, key
}

func TestUseAppliesIdentity(t *testing.T) {
	isolate(t)
	book, _ := seed(t)
	repo := newRepo(t)
	t.Chdir(repo)

	mustRun(t, book, "use", "work")
	for k, want := range map[string]string{
		"user.name":       "Dirk Heilig",
		"user.email":      "dirk@example.com",
		"id.identity":     "work",
		"core.sshCommand": idrepo.SSHCommand(book, "work"),
	} {
		if got := gitRun(t, repo, "config", "--local", k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
	repos, err := store.LoadRepos(store.RegistryPath(book))
	if err != nil || !reflect.DeepEqual(repos, []string{repo}) {
		t.Errorf("registry = %v, %v", repos, err)
	}
}

func TestUseOutsideRepoAndUnknownIdentity(t *testing.T) {
	isolate(t)
	book, _ := seed(t)
	t.Chdir(t.TempDir())
	if _, err := run(t, book, "use", "work"); err == nil {
		t.Error("use outside a repo succeeded")
	}
	t.Chdir(newRepo(t))
	if _, err := run(t, book, "use", "ghost"); err == nil {
		t.Error("use of unknown identity succeeded")
	}
}

func TestUseRefusesForeignSSHCommand(t *testing.T) {
	isolate(t)
	book, _ := seed(t)
	repo := newRepo(t)
	t.Chdir(repo)
	gitRun(t, repo, "config", "core.sshCommand", "ssh -v")
	if _, err := run(t, book, "use", "work"); err == nil {
		t.Fatal("foreign core.sshCommand overwritten without --force")
	}
	mustRun(t, book, "use", "work", "--force")
	if got := gitRun(t, repo, "config", "core.sshCommand"); got != idrepo.SSHCommand(book, "work") {
		t.Errorf("core.sshCommand = %q", got)
	}
}

func TestCurrent(t *testing.T) {
	isolate(t)
	book, _ := seed(t)
	repo := newRepo(t)
	t.Chdir(repo)

	if _, err := run(t, book, "current"); err == nil {
		t.Error("current succeeded without identity")
	}
	if out, err := run(t, book, "current", "-q"); err == nil || out != "" {
		t.Errorf("quiet current: out %q err %v", out, err)
	}
	mustRun(t, book, "use", "work")
	if out := mustRun(t, book, "current"); strings.TrimSpace(out) != "work" {
		t.Errorf("current = %q", out)
	}
	if out := mustRun(t, book, "current", "-q"); out != "" {
		t.Errorf("quiet current printed %q", out)
	}
	// Identity removed from the book: current must fail.
	mustRun(t, book, "rm", "work")
	if _, err := run(t, book, "current", "-q"); err == nil {
		t.Error("current succeeded with dangling identity")
	}
	t.Chdir(t.TempDir())
	if _, err := run(t, book, "current"); err == nil {
		t.Error("current succeeded outside a repo")
	}
}

func TestSyncPropagatesAndPrunes(t *testing.T) {
	isolate(t)
	book, _ := seed(t)
	a, b := newRepo(t), newRepo(t)
	for _, r := range []string{a, b} {
		t.Chdir(r)
		mustRun(t, book, "use", "work")
	}
	mustRun(t, book, "email", "edit", "work", "new@example.com")
	mustRun(t, book, "name", "edit", "me", "D. Heilig")
	if err := os.RemoveAll(b); err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())

	out := mustRun(t, book, "sync")
	if !strings.Contains(out, "synced "+a) || !strings.Contains(out, "dropped "+b) {
		t.Errorf("sync output:\n%s", out)
	}
	if got := gitRun(t, a, "config", "--local", "user.email"); got != "new@example.com" {
		t.Errorf("user.email = %q", got)
	}
	if got := gitRun(t, a, "config", "--local", "user.name"); got != "D. Heilig" {
		t.Errorf("user.name = %q", got)
	}
	repos, _ := store.LoadRepos(store.RegistryPath(book))
	if !reflect.DeepEqual(repos, []string{a}) {
		t.Errorf("registry after prune = %v", repos)
	}
}

func TestSyncReportsMissingIdentity(t *testing.T) {
	isolate(t)
	book, _ := seed(t)
	repo := newRepo(t)
	t.Chdir(repo)
	mustRun(t, book, "use", "work")
	mustRun(t, book, "rm", "work")
	if _, err := run(t, book, "sync"); err == nil {
		t.Error("sync succeeded with dangling identity")
	}
}

func TestSSHArgv(t *testing.T) {
	r := store.Resolved{Key: store.Key{Private: "/k/id"}}
	got := sshArgv(r, []string{"-p", "22", "host", "git-upload-pack 'x'"})
	want := []string{"ssh", "-i", "/k/id", "-o", "IdentitiesOnly=yes", "-p", "22", "host", "git-upload-pack 'x'"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("argv = %v", got)
	}
}

func TestInitImportsGlobal(t *testing.T) {
	home := isolate(t)
	gitRun(t, home, "config", "--global", "user.name", "Dirk Heilig")
	gitRun(t, home, "config", "--global", "user.email", "dirk@example.org")
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatal(err)
	}
	key := filepath.Join(sshDir, "id_ed25519")
	for _, f := range []string{key, key + ".pub"} {
		if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	book := filepath.Join(home, "book.toml")

	mustRun(t, book, "init")

	b, err := store.Load(book)
	if err != nil {
		t.Fatal(err)
	}
	r, err := b.Resolve("default")
	if err != nil {
		t.Fatal(err)
	}
	if r.Name != "Dirk Heilig" || r.Email != "dirk@example.org" || r.Key.Private != key || r.Key.Public != key+".pub" {
		t.Errorf("resolved = %+v", r)
	}
	for _, k := range []string{"user.name", "user.email"} {
		if _, ok, _ := gitcfg.GetGlobal(k); ok {
			t.Errorf("global %s still set", k)
		}
	}
	if _, err := run(t, book, "init"); err == nil {
		t.Error("second init succeeded")
	}
}

func TestInitWithoutKeyImportsDatapointsOnly(t *testing.T) {
	home := isolate(t)
	gitRun(t, home, "config", "--global", "user.name", "Dirk Heilig")
	gitRun(t, home, "config", "--global", "user.email", "dirk@example.org")
	book := filepath.Join(home, "book.toml")

	out := mustRun(t, book, "init")
	if !strings.Contains(out, "no identity created") {
		t.Errorf("output: %s", out)
	}
	b, _ := store.Load(book)
	if len(b.Names) != 1 || len(b.Emails) != 1 || len(b.Identities) != 0 {
		t.Errorf("book = %+v", b)
	}
}

func TestInitNothingToImport(t *testing.T) {
	home := isolate(t)
	if _, err := run(t, filepath.Join(home, "book.toml"), "init"); err == nil {
		t.Error("init succeeded without global config")
	}
}

func TestWarnsWhenGitSSHCommandOverrides(t *testing.T) {
	isolate(t)
	book, _ := seed(t)
	repo := newRepo(t)
	t.Chdir(repo)

	t.Setenv("GIT_SSH_COMMAND", "")
	if out := mustRun(t, book, "use", "work"); strings.Contains(out, "GIT_SSH_COMMAND") {
		t.Errorf("warned without the variable set:\n%s", out)
	}

	t.Setenv("GIT_SSH_COMMAND", "ssh -v")
	for _, args := range [][]string{{"use", "work"}, {"current"}} {
		out := mustRun(t, book, args...)
		if !strings.Contains(out, "GIT_SSH_COMMAND is set") {
			t.Errorf("git id %v did not warn:\n%s", args, out)
		}
	}
	// The hook relies on `current -q` staying silent.
	if out := mustRun(t, book, "current", "-q"); out != "" {
		t.Errorf("quiet current printed %q", out)
	}
}

func TestRenameDatapointsThroughCLI(t *testing.T) {
	isolate(t)
	book, _ := seed(t)
	mustRun(t, book, "email", "rename", "work", "company")
	mustRun(t, book, "name", "rename", "me", "dirk")
	mustRun(t, book, "key", "rename", "k1", "mainkey")
	out := mustRun(t, book, "show", "work")
	for _, want := range []string{"Dirk Heilig", "dirk@example.com"} {
		if !strings.Contains(out, want) {
			t.Errorf("identity broke after renames, missing %q:\n%s", want, out)
		}
	}
	if ls := mustRun(t, book, "ls"); !strings.Contains(ls, "dirk") || !strings.Contains(ls, "company") || !strings.Contains(ls, "mainkey") {
		t.Errorf("identity ls does not show the new labels:\n%s", ls)
	}
	if _, err := run(t, book, "email", "rename", "company", "bad label"); err == nil {
		t.Error("invalid label accepted")
	}
	mustRun(t, book, "email", "add", "x", "x@y.z")
	if _, err := run(t, book, "email", "rename", "x", "company"); err == nil {
		t.Error("collision accepted")
	}
}

func TestRenameIdentityUpdatesRepositories(t *testing.T) {
	isolate(t)
	book, _ := seed(t)
	repo := newRepo(t)
	t.Chdir(repo)
	mustRun(t, book, "use", "work")

	out := mustRun(t, book, "rename", "work", "company")
	if !strings.Contains(out, "updated "+repo) {
		t.Errorf("repository not reported:\n%s", out)
	}
	if got := gitRun(t, repo, "config", "--local", "id.identity"); got != "company" {
		t.Errorf("id.identity = %q", got)
	}
	if got := gitRun(t, repo, "config", "--local", "core.sshCommand"); got != idrepo.SSHCommand(book, "company") {
		t.Errorf("core.sshCommand = %q", got)
	}
	if got := strings.TrimSpace(mustRun(t, book, "current")); got != "company" {
		t.Errorf("current = %q", got)
	}
	// Sync keeps working with the new label.
	mustRun(t, book, "sync")

	// A foreign core.sshCommand is left alone.
	other := newRepo(t)
	t.Chdir(other)
	mustRun(t, book, "use", "company")
	gitRun(t, other, "config", "core.sshCommand", "ssh -v")
	mustRun(t, book, "rename", "company", "final")
	if got := gitRun(t, other, "config", "--local", "core.sshCommand"); got != "ssh -v" {
		t.Errorf("foreign core.sshCommand changed to %q", got)
	}
	if got := gitRun(t, other, "config", "--local", "id.identity"); got != "final" {
		t.Errorf("id.identity = %q", got)
	}

	if _, err := run(t, book, "rename", "ghost", "x"); err == nil {
		t.Error("renaming a missing identity succeeded")
	}
}

func TestUnuse(t *testing.T) {
	isolate(t)
	book, _ := seed(t)
	repo := newRepo(t)
	t.Chdir(repo)

	if _, err := run(t, book, "unuse"); err == nil {
		t.Error("unuse without identity succeeded")
	}
	mustRun(t, book, "use", "work")
	out := mustRun(t, book, "unuse")
	if !strings.Contains(out, "removed the identity from "+repo) {
		t.Errorf("output:\n%s", out)
	}
	for _, k := range []string{"user.name", "user.email", "id.identity", "core.sshCommand"} {
		if v, err := exec.Command("git", "-C", repo, "config", "--local", k).Output(); err == nil {
			t.Errorf("%s still set: %s", k, v)
		}
	}
	if _, err := run(t, book, "current", "-q"); err == nil {
		t.Error("current succeeds after unuse")
	}
	if regs, _ := store.LoadRepos(store.RegistryPath(book)); len(regs) != 0 {
		t.Errorf("repository still registered: %v", regs)
	}
	// Using it again works.
	mustRun(t, book, "use", "work")
}

func TestUnuseKeepsForeignSSHCommand(t *testing.T) {
	isolate(t)
	book, _ := seed(t)
	repo := newRepo(t)
	t.Chdir(repo)
	mustRun(t, book, "use", "work", "--force")
	gitRun(t, repo, "config", "core.sshCommand", "ssh -v")
	out := mustRun(t, book, "unuse")
	if !strings.Contains(out, "left in place") {
		t.Errorf("no note about the foreign command:\n%s", out)
	}
	if got := gitRun(t, repo, "config", "--local", "core.sshCommand"); got != "ssh -v" {
		t.Errorf("foreign core.sshCommand = %q", got)
	}
	t.Chdir(t.TempDir())
	if _, err := run(t, book, "unuse"); err == nil {
		t.Error("unuse outside a repository succeeded")
	}
}

func TestReposListAndForget(t *testing.T) {
	isolate(t)
	book, _ := seed(t)
	ok, gone, bare, dangling := newRepo(t), newRepo(t), newRepo(t), newRepo(t)
	for _, r := range []string{ok, gone, bare, dangling} {
		t.Chdir(r)
		mustRun(t, book, "use", "work")
	}
	gitRun(t, bare, "config", "--local", "--unset", "id.identity")
	gitRun(t, dangling, "config", "--local", "id.identity", "nope")
	if err := os.RemoveAll(gone); err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())

	out := mustRun(t, book, "repos", "ls")
	for repo, want := range map[string]string{ok: "work  ok", gone: "missing", bare: "no-identity", dangling: "nope  unknown-identity"} {
		line := lineWith(out, repo)
		if line == "" || !strings.Contains(line, want) {
			t.Errorf("repos ls line for %s = %q, want it to contain %q\n%s", repo, line, want, out)
		}
	}

	mustRun(t, book, "repos", "forget", gone)
	if out := mustRun(t, book, "repos", "ls"); strings.Contains(out, gone) {
		t.Errorf("forgotten repository still listed:\n%s", out)
	}
	if _, err := run(t, book, "repos", "forget", gone); err == nil {
		t.Error("forgetting an untracked repository succeeded")
	}
	if got, _ := complete(t, book, "repos", "forget", ""); len(got) != 3 {
		t.Errorf("completion offers %v, want the 3 tracked repositories", got)
	}
	// forgetting leaves the repository configuration alone.
	mustRun(t, book, "repos", "forget", ok)
	if got := gitRun(t, ok, "config", "--local", "id.identity"); got != "work" {
		t.Errorf("forget touched the repository: id.identity = %q", got)
	}
}

func TestInitStopsGitFromGuessingAnAuthor(t *testing.T) {
	home := isolate(t)
	gitRun(t, home, "config", "--global", "user.name", "Dirk Heilig")
	gitRun(t, home, "config", "--global", "user.email", "dirk@example.org")
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sshDir, "id_ed25519"), []byte("k"), 0o600); err != nil {
		t.Fatal(err)
	}
	book := filepath.Join(home, "book.toml")

	out := mustRun(t, book, "init")
	if !strings.Contains(out, "user.useConfigOnly = true") {
		t.Errorf("init does not report it:\n%s", out)
	}
	if got := gitRun(t, home, "config", "--global", "user.useConfigOnly"); got != "true" {
		t.Errorf("user.useConfigOnly = %q", got)
	}

	repo := newRepo(t)
	t.Chdir(repo)
	commit := exec.Command("git", "commit", "-q", "--allow-empty", "-m", "x")
	commit.Dir = repo
	if o, err := commit.CombinedOutput(); err == nil || !strings.Contains(string(o), "auto-detection is disabled") {
		t.Errorf("commit without identity: err %v\n%s", err, o)
	}
	mustRun(t, book, "use", "default")
	gitRun(t, repo, "commit", "-q", "--allow-empty", "-m", "x") // identity now set locally
	if got := gitRun(t, repo, "log", "-1", "--format=%an <%ae>"); got != "Dirk Heilig <dirk@example.org>" {
		t.Errorf("author = %q", got)
	}
}

func TestFailedInitLeavesGitAlone(t *testing.T) {
	home := isolate(t)
	if _, err := run(t, filepath.Join(home, "book.toml"), "init"); err == nil {
		t.Fatal("init without global config succeeded")
	}
	if out, err := exec.Command("git", "config", "--global", "user.useConfigOnly").Output(); err == nil {
		t.Errorf("user.useConfigOnly set by a failed init: %s", out)
	}
}

func TestSSHOverrideWarningIsYellowOnlyWhenColoursAreOn(t *testing.T) {
	isolate(t)
	book, _ := seed(t)
	repo := newRepo(t)
	t.Chdir(repo)
	t.Setenv("GIT_SSH_COMMAND", "ssh -v")
	warn := idrepo.SSHOverrideWarning

	t.Setenv("NO_COLOR", "")
	t.Setenv("CLICOLOR_FORCE", "")
	if out := mustRun(t, book, "use", "work"); strings.Contains(out, "\x1b[") || !strings.Contains(out, warn) {
		t.Errorf("plain output expected without a terminal:\n%q", out)
	}
	t.Setenv("CLICOLOR_FORCE", "1")
	out := mustRun(t, book, "use", "work")
	if !strings.Contains(out, "\x1b[") || !strings.Contains(out, warn) {
		t.Errorf("CLICOLOR_FORCE did not colour the warning:\n%q", out)
	}
	if strings.Contains(out, "\x1b[1;31m") || strings.Contains(out, "\x1b[1;91m") {
		t.Errorf("the warning is red:\n%q", out)
	}
	t.Setenv("NO_COLOR", "1")
	if out := mustRun(t, book, "use", "work"); strings.Contains(out, "\x1b[") {
		t.Errorf("NO_COLOR ignored:\n%q", out)
	}
}
