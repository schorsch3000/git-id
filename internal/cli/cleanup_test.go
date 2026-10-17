package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// runIn is run with scripted stdin.
func runIn(t *testing.T, book, input string, args ...string) (string, error) {
	t.Helper()
	cmd := NewRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader(input))
	cmd.SetArgs(append([]string{"--book", book}, args...))
	err := cmd.Execute()
	return out.String(), err
}

// commitAs commits a new file with the given author and committer.
func commitAs(t *testing.T, dir, name, email, msg string) {
	t.Helper()
	commitSplit(t, dir, name, email, name, email, msg)
}

func commitSplit(t *testing.T, dir, an, ae, cn, ce, msg string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, strings.ReplaceAll(msg, " ", "_")), []byte(msg), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "add", "-A")
	cmd := exec.Command("git", "commit", "-q", "-m", msg)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME="+an, "GIT_AUTHOR_EMAIL="+ae, "GIT_COMMITTER_NAME="+cn, "GIT_COMMITTER_EMAIL="+ce)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("commit: %v\n%s", err, out)
	}
}

// history returns "author|committer|subject" lines of every ref, oldest first.
func history(t *testing.T, dir string) string {
	t.Helper()
	return gitRun(t, dir, "log", "--all", "--reverse", "--format=%an <%ae>|%cn <%ce>|%s")
}

const (
	cur    = "Dirk Heilig <dirk@example.com>"
	oldAut = "Old Name <old@example.org>"
	other  = "Stranger <stranger@elsewhere.net>"
)

// cleanupFixture builds a repository using identity "work" (Dirk Heilig) whose
// history holds commits of the current identity, of other identities from the
// address book, and of someone unknown:
//
//	main: c1 Old Name | c2 Dirk | c3 Stranger | c4 author Old Name, committer Dirk
//	side: c5 Old Name <priv@example.org>, annotated tag v1 tagged by Old Name
func cleanupFixture(t *testing.T) (book, repo string) {
	t.Helper()
	if err := exec.Command("git", "filter-repo", "--version").Run(); err != nil {
		t.Skip("git-filter-repo not installed")
	}
	book, _ = seed(t)
	mustRun(t, book, "name", "add", "old", "Old Name")
	mustRun(t, book, "email", "add", "old", "old@example.org")
	mustRun(t, book, "email", "add", "priv", "priv@example.org")

	repo = newRepo(t)
	t.Chdir(repo)
	gitRun(t, repo, "checkout", "-q", "-b", "main")
	mustRun(t, book, "use", "work")

	commitAs(t, repo, "Old Name", "old@example.org", "c1")
	commitAs(t, repo, "Dirk Heilig", "dirk@example.com", "c2")
	commitAs(t, repo, "Stranger", "stranger@elsewhere.net", "c3")
	commitSplit(t, repo, "Old Name", "old@example.org", "Dirk Heilig", "dirk@example.com", "c4")
	gitRun(t, repo, "checkout", "-q", "-b", "side")
	commitAs(t, repo, "Old Name", "priv@example.org", "c5")
	tag := exec.Command("git", "tag", "-a", "-m", "release", "v1")
	tag.Dir = repo
	tag.Env = append(os.Environ(), "GIT_COMMITTER_NAME=Old Name", "GIT_COMMITTER_EMAIL=old@example.org")
	if out, err := tag.CombinedOutput(); err != nil {
		t.Fatalf("tag: %v\n%s", err, out)
	}
	gitRun(t, repo, "checkout", "-q", "main")
	return book, repo
}

func TestCleanupDryRunChangesNothing(t *testing.T) {
	book, repo := cleanupFixture(t)
	before := history(t, repo)
	out := mustRun(t, book, "cleanup", "--dry-run")
	for _, want := range []string{"Old Name", "old@example.org", "priv@example.org", "Dry run", "ALL branches"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	for _, unwanted := range []string{"Stranger", "stranger@elsewhere.net"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("unknown value %q reported:\n%s", unwanted, out)
		}
	}
	if history(t, repo) != before {
		t.Error("dry run changed history")
	}
}

func TestCleanupYesRewritesAllRefsAndKeepsBackup(t *testing.T) {
	book, repo := cleanupFixture(t)
	oldMain := gitRun(t, repo, "rev-parse", "main")
	oldSide := gitRun(t, repo, "rev-parse", "side")
	oldTag := gitRun(t, repo, "rev-parse", "v1")

	out := mustRun(t, book, "cleanup", "--yes")
	if !strings.Contains(out, "THIS REWRITES GIT HISTORY") || !strings.Contains(out, "ALL branches, tags and remote-tracking refs") {
		t.Errorf("no loud warning:\n%s", out)
	}

	h := history(t, repo)
	for _, gone := range []string{"Old Name", "old@example.org", "priv@example.org"} {
		if strings.Contains(h, gone) {
			t.Errorf("%q survived the cleanup:\n%s", gone, h)
		}
	}
	for _, kept := range []string{other, "|" + cur + "|c2", cur + "|" + cur + "|c4", cur + "|" + cur + "|c5"} {
		if !strings.Contains(h, kept) {
			t.Errorf("history lacks %q:\n%s", kept, h)
		}
	}
	if !strings.Contains(h, "|c3") || !strings.Contains(h, other+"|"+other+"|c3") {
		t.Errorf("unknown author touched:\n%s", h)
	}
	if tagger := gitRun(t, repo, "for-each-ref", "--format=%(taggername) <%(taggeremail)>", "refs/tags/v1"); tagger != "Dirk Heilig <<dirk@example.com>>" {
		t.Errorf("tagger = %q", tagger)
	}
	for ref, old := range map[string]string{"main": oldMain, "side": oldSide, "v1": oldTag} {
		if gitRun(t, repo, "rev-parse", ref) == old {
			t.Errorf("%s was not rewritten", ref)
		}
	}
	if got := gitRun(t, repo, "show", "main:c1"); got != "c1" {
		t.Errorf("file content changed: %q", got)
	}
	if got := gitRun(t, repo, "config", "--local", "id.identity"); got != "work" {
		t.Errorf("repo config lost: id.identity = %q", got)
	}
	if st := gitRun(t, repo, "status", "--porcelain"); st != "" {
		t.Errorf("work tree dirty after cleanup: %s", st)
	}

	// The backup bundle is valid and the printed command restores the old refs.
	var bundle string
	for _, l := range strings.Split(out, "\n") {
		if p, ok := strings.CutPrefix(l, "Backup of all refs: "); ok {
			bundle = p
		}
	}
	if bundle == "" {
		t.Fatalf("no backup path printed:\n%s", out)
	}
	gitRun(t, repo, "bundle", "verify", bundle)
	restore := exec.Command("sh", "-c", strings.TrimPrefix(lineWith(out, "To undo: "), "To undo: "))
	restore.Dir = repo
	if o, err := restore.CombinedOutput(); err != nil {
		t.Fatalf("restore command failed: %v\n%s", err, o)
	}
	for ref, old := range map[string]string{"main": oldMain, "side": oldSide, "v1": oldTag} {
		if got := gitRun(t, repo, "rev-parse", ref); got != old {
			t.Errorf("after restore %s = %s, want %s", ref, got, old)
		}
	}
}

func lineWith(out, prefix string) string {
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, prefix) {
			return l
		}
	}
	return ""
}

func TestCleanupCurrentBranchLeavesOthers(t *testing.T) {
	book, repo := cleanupFixture(t)
	oldSide := gitRun(t, repo, "rev-parse", "side")
	out := mustRun(t, book, "cleanup", "--yes", "--current-branch")
	if !strings.Contains(out, "branch main only") {
		t.Errorf("scope not stated:\n%s", out)
	}
	if got := gitRun(t, repo, "rev-parse", "side"); got != oldSide {
		t.Error("side was rewritten although only main was in scope")
	}
	if main := gitRun(t, repo, "log", "main", "--format=%an <%ae>"); strings.Contains(main, "Old Name") {
		t.Errorf("main still has the old identity:\n%s", main)
	}
	if side := gitRun(t, repo, "log", "side", "--format=%an <%ae>|%cn"); !strings.Contains(side, "priv@example.org") {
		t.Errorf("side lost its old values:\n%s", side)
	}
}

func TestCleanupInteractive(t *testing.T) {
	book, repo := cleanupFixture(t)
	// Findings are asked names first. Keep the name, then take all emails.
	out, err := runIn(t, book, "n\na\nyes\n", "cleanup")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	h := history(t, repo)
	if !strings.Contains(h, "Old Name <dirk@example.com>") {
		t.Errorf("name should be kept, email replaced:\n%s", h)
	}
	for _, gone := range []string{"old@example.org", "priv@example.org"} {
		if strings.Contains(h, gone) {
			t.Errorf("%q survived:\n%s", gone, h)
		}
	}
	if strings.Count(out, "replace?") != 2 {
		t.Errorf("batching should leave 2 questions (name, first email):\n%s", out)
	}
}

func TestCleanupAbortPaths(t *testing.T) {
	for name, input := range map[string]string{
		"quit at first question": "q\n",
		"decline confirmation":   "a\na\nno\n",
		"input ends":             "",
		"skip everything":        "s\ns\n",
	} {
		t.Run(name, func(t *testing.T) {
			book, repo := cleanupFixture(t)
			before := history(t, repo)
			runIn(t, book, input, "cleanup")
			if history(t, repo) != before {
				t.Error("history changed")
			}
			if matches, _ := filepath.Glob(filepath.Join(repo, ".git", "git-id-backup-*")); len(matches) != 0 {
				t.Errorf("backup written without rewrite: %v", matches)
			}
		})
	}
}

func TestCleanupWarnsAboutPushedCommits(t *testing.T) {
	book, repo := cleanupFixture(t)
	remote := filepath.Join(t.TempDir(), "remote.git")
	gitRun(t, repo, "init", "-q", "--bare", remote)
	gitRun(t, repo, "remote", "add", "origin", remote)
	gitRun(t, repo, "push", "-q", "origin", "main")

	out := mustRun(t, book, "cleanup", "--yes", "--current-branch")
	if !strings.Contains(out, "WARNING: 2 of them are already in remote-tracking refs") {
		t.Errorf("no pushed-commits warning:\n%s", out)
	}
	if !strings.Contains(out, "Nothing was pushed") {
		t.Errorf("missing publish hint:\n%s", out)
	}
	if got := gitRun(t, repo, "remote", "get-url", "origin"); got != remote {
		t.Errorf("origin remote = %q, want it preserved", got)
	}
	if got := gitRun(t, remote, "log", "main", "--format=%an"); !strings.Contains(got, "Old Name") {
		t.Error("the remote must not have been touched")
	}
}

func TestCleanupNothingToDo(t *testing.T) {
	book, _ := seed(t)
	repo := newRepo(t)
	t.Chdir(repo)
	mustRun(t, book, "use", "work")
	commitAs(t, repo, "Dirk Heilig", "dirk@example.com", "only mine")
	commitAs(t, repo, "Stranger", "x@y.z", "someone else")
	out := mustRun(t, book, "cleanup", "--yes")
	if !strings.Contains(out, "Nothing to clean up") {
		t.Errorf("output:\n%s", out)
	}
}

func TestCleanupRefusals(t *testing.T) {
	t.Run("no identity", func(t *testing.T) {
		book, _ := seed(t)
		t.Chdir(newRepo(t))
		if _, err := run(t, book, "cleanup", "--yes"); err == nil {
			t.Error("cleanup without identity succeeded")
		}
	})
	t.Run("not a repository", func(t *testing.T) {
		book, _ := seed(t)
		t.Chdir(t.TempDir())
		if _, err := run(t, book, "cleanup", "--yes"); err == nil {
			t.Error("cleanup outside a repository succeeded")
		}
	})
	t.Run("dirty work tree", func(t *testing.T) {
		book, repo := cleanupFixture(t)
		if err := os.WriteFile(filepath.Join(repo, "c1"), []byte("edited"), 0o644); err != nil {
			t.Fatal(err)
		}
		before := history(t, repo)
		out, err := run(t, book, "cleanup", "--yes")
		if err == nil || !strings.Contains(err.Error(), "uncommitted") {
			t.Errorf("err = %v\n%s", err, out)
		}
		if history(t, repo) != before {
			t.Error("history changed")
		}
	})
	t.Run("detached HEAD with --current-branch", func(t *testing.T) {
		book, repo := cleanupFixture(t)
		gitRun(t, repo, "checkout", "-q", "--detach")
		if _, err := run(t, book, "cleanup", "--yes", "--current-branch"); err == nil {
			t.Error("detached HEAD accepted")
		}
	})
}

// signedCount returns how many of the commits on every ref carry a signature.
func signedCount(t *testing.T, repo string) int {
	t.Helper()
	n := 0
	for _, h := range strings.Fields(gitRun(t, repo, "rev-list", "--all")) {
		if strings.Contains(gitRun(t, repo, "cat-file", "commit", h), "\ngpgsig ") {
			n++
		}
	}
	return n
}

// TestCleanupWithSignedCommits documents what git filter-repo does to signed
// history and checks that the user is told beforehand.
func TestCleanupWithSignedCommits(t *testing.T) {
	if _, err := exec.LookPath("ssh-keygen"); err != nil {
		t.Skip("ssh-keygen not available")
	}
	book, repo := cleanupFixture(t)
	t.Chdir(repo)
	dir := t.TempDir()
	signKey := filepath.Join(dir, "sign")
	if out, err := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", signKey).CombinedOutput(); err != nil {
		t.Fatalf("ssh-keygen: %v\n%s", err, out)
	}
	gitRun(t, repo, "config", "gpg.format", "ssh")
	gitRun(t, repo, "config", "user.signingkey", signKey+".pub")
	gitRun(t, repo, "config", "commit.gpgsign", "true")
	// Two signed commits on top of the fixture's unsigned history: one by the
	// current identity (unaffected by the cleanup) and one by another identity.
	commitAs(t, repo, "Dirk Heilig", "dirk@example.com", "signed mine")
	commitAs(t, repo, "Old Name", "old@example.org", "signed old")
	if got := signedCount(t, repo); got != 2 {
		t.Fatalf("setup: %d signed commits, want 2", got)
	}
	mineBefore := gitRun(t, repo, "rev-parse", "main~1")

	out := mustRun(t, book, "cleanup", "--yes", "--current-branch")
	if !strings.Contains(out, "WARNING: 2 of the 6 scanned commits are signed") {
		t.Errorf("no signature warning:\n%s", out)
	}
	if signedCount(t, repo) != 0 {
		t.Error("signatures survived the rewrite")
	}
	if gitRun(t, repo, "rev-parse", "main~1") == mineBefore {
		t.Error("the signed commit of the current identity kept its hash although its signature was dropped")
	}
	if h := gitRun(t, repo, "log", "main", "--format=%an <%ae>"); strings.Contains(h, "Old Name") {
		t.Errorf("old identity survived:\n%s", h)
	}
}

func TestCleanupDoesNotWarnWithoutSignatures(t *testing.T) {
	book, _ := cleanupFixture(t)
	if out := mustRun(t, book, "cleanup", "--yes"); strings.Contains(out, "are signed") {
		t.Errorf("signature warning without signed commits:\n%s", out)
	}
}

// trailerMessage is a commit message with trailers of the old identity, of a
// stranger, and a sentence that merely mentions one.
const trailerMessage = `trailers

The old Name appears in this body and so does a Signed-off-by: Old Name <old@example.org> sentence.

Signed-off-by: Old Name <OLD@example.org>
Co-authored-by: Old Name <old@example.org>
Signed-off-by: Stranger <stranger@elsewhere.net>`

func TestCleanupTrailers(t *testing.T) {
	for _, withFlag := range []bool{false, true} {
		name := "without --trailers"
		if withFlag {
			name = "with --trailers"
		}
		t.Run(name, func(t *testing.T) {
			book, repo := cleanupFixture(t)
			commitAs(t, repo, "Old Name", "old@example.org", trailerMessage)
			msgOf := func() string { return gitRun(t, repo, "log", "-1", "--format=%B", "main") }
			before := msgOf()

			args := []string{"cleanup", "--yes", "--current-branch"}
			if withFlag {
				args = append(args, "--trailers")
			}
			out := mustRun(t, book, args...)
			after := msgOf()
			if !withFlag {
				if after != before {
					t.Errorf("message changed without --trailers:\n%s", after)
				}
				if strings.Contains(out, "(--trailers)") {
					t.Errorf("announced trailer rewriting without the flag:\n%s", out)
				}
				return
			}
			if !strings.Contains(out, "(--trailers)") {
				t.Errorf("trailer rewriting not announced:\n%s", out)
			}
			for _, want := range []string{
				"Signed-off-by: Dirk Heilig <dirk@example.com>",
				"Co-authored-by: Dirk Heilig <dirk@example.com>",
				"Signed-off-by: Stranger <stranger@elsewhere.net>", // not in the address book
				"The old Name appears in this body and so does a Signed-off-by: Old Name <old@example.org> sentence.",
			} {
				if !strings.Contains(after, want) {
					t.Errorf("message lacks %q:\n%s", want, after)
				}
			}
			if strings.Contains(after, "OLD@example.org") || strings.Contains(after, "\nCo-authored-by: Old") {
				t.Errorf("an old trailer survived:\n%s", after)
			}
		})
	}
}

// hideFilterRepo puts only git on PATH, so git filter-repo cannot be found.
func hideFilterRepo(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("PATH games with a symlink")
	}
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("no git")
	}
	dir := t.TempDir()
	if err := os.Symlink(git, filepath.Join(dir, "git")); err != nil {
		t.Skip("cannot symlink:", err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("GIT_EXEC_PATH", "")
	if exec.Command("git", "filter-repo", "--version").Run() == nil {
		t.Skip("git filter-repo is found anyway (installed next to git itself)")
	}
}

// Only a rewrite needs git-filter-repo: a history with nothing to replace is
// reported as clean without it, and a history with findings refuses before it
// asks anything.
func TestCleanupNeedsFilterRepoOnlyToRewrite(t *testing.T) {
	book, repo := cleanupFixture(t)
	hideFilterRepo(t)
	before := history(t, repo)

	if _, err := run(t, book, "cleanup", "--yes"); err == nil || !strings.Contains(err.Error(), "git-filter-repo is required") {
		t.Errorf("expected the install hint, got %v", err)
	}
	if out, err := runIn(t, book, "", "cleanup"); err == nil || strings.Contains(out, "replace?") {
		t.Errorf("asked questions although the tool is missing: err %v\n%s", err, out)
	}
	if history(t, repo) != before {
		t.Error("history changed")
	}
	// A dry run only reports.
	if out := mustRun(t, book, "cleanup", "--dry-run"); !strings.Contains(out, "Dry run") {
		t.Errorf("dry run without the tool:\n%s", out)
	}
}
