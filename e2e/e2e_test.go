// Package e2e pushes real commits over real ssh to an in-process ssh server
// and checks that git-id selects the right key and author per repository.
package e2e

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// env is an isolated shell environment: own HOME, git config, address book,
// and a bin dir holding the freshly built git-id plus an ssh shim.
type env struct {
	home string
	vars []string
}

func newEnv(t *testing.T) *env {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the end-to-end tests drive POSIX shells and an sh ssh shim")
	}
	for _, tool := range []string{"git", "ssh", "go", "git-receive-pack", "git-upload-pack"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not available: %v", tool, err)
		}
	}
	realSSH, _ := exec.LookPath("ssh")
	root := t.TempDir()
	home := filepath.Join(root, "home")
	bin := filepath.Join(root, "bin")
	for _, d := range []string{home, bin} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	build := exec.Command("go", "build", "-o", filepath.Join(bin, "git-id"), "./cmd/git-id")
	build.Dir = ".."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build git-id: %v\n%s", err, out)
	}

	// ssh reads its config from the passwd home, not $HOME, so loopback host
	// key checking is disabled by a shim in front of the real ssh. The key
	// options still come from git-id's wrapper.
	shim := "#!/bin/sh\nexec " + realSSH + " -F /dev/null -o BatchMode=yes" +
		" -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR \"$@\"\n"
	if err := os.WriteFile(filepath.Join(bin, "ssh"), []byte(shim), 0o755); err != nil {
		t.Fatal(err)
	}

	// Drop everything that could override core.sshCommand or leak real keys
	// (GIT_SSH_COMMAND beats core.sshCommand, SSH_AUTH_SOCK offers agent keys).
	var vars []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		// CI, NO_COLOR, CLICOLOR_FORCE and COLORTERM decide whether colour
		// libraries treat the pty as a terminal (CI makes them assume it is not);
		// the tests set what they need and must not depend on the runner.
		switch k {
		case "CI", "NO_COLOR", "CLICOLOR_FORCE", "COLORTERM", "TERM":
			continue
		}
		if strings.HasPrefix(k, "GIT_") || strings.HasPrefix(k, "SSH_") || k == "HOME" || k == "PATH" {
			continue
		}
		vars = append(vars, kv)
	}
	vars = append(vars,
		"HOME="+home,
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"GIT_CONFIG_GLOBAL="+filepath.Join(home, ".gitconfig"),
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_TERMINAL_PROMPT=0",
		"GIT_ID_BOOK="+filepath.Join(home, "book.toml"),
	)
	return &env{home: home, vars: vars}
}

func (e *env) run(dir, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = e.vars
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (e *env) must(t *testing.T, dir, name string, args ...string) string {
	t.Helper()
	out, err := e.run(dir, name, args...)
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
	return strings.TrimSpace(out)
}

// gid runs `git id ...`, which also exercises git's subcommand lookup.
func (e *env) gid(t *testing.T, dir string, args ...string) string {
	t.Helper()
	return e.must(t, dir, "git", append([]string{"id"}, args...)...)
}

// commit creates a commit with the repo's configured author.
func (e *env) commit(t *testing.T, repo, msg string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repo, "f.txt"), []byte(msg+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	e.must(t, repo, "git", "add", "f.txt")
	e.must(t, repo, "git", "commit", "-q", "-m", msg)
}

func (e *env) newRepo(t *testing.T, name, remote string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	e.must(t, dir, "git", "init", "-q", "-b", "main")
	e.must(t, dir, "git", "remote", "add", "origin", remote)
	return dir
}

func TestPushUsesIdentityKey(t *testing.T) {
	e := newEnv(t)
	keys := filepath.Join(e.home, "keys")
	if err := os.MkdirAll(keys, 0o700); err != nil {
		t.Fatal(err)
	}
	workPriv, workPub, workKey := writeKeypair(t, keys, "work")
	privPriv, privPub, privKey := writeKeypair(t, keys, "priv")

	root := filepath.Join(e.home, "server")
	srv := newSSHServer(t, root)
	srv.authorize("work", workKey, "/work.git")
	srv.authorize("priv", privKey, "/priv.git")
	for _, r := range []string{"work.git", "priv.git"} {
		e.must(t, e.home, "git", "init", "-q", "--bare", filepath.Join(root, r))
	}

	// Address book with two identities.
	e.gid(t, e.home, "name", "add", "work", "Dirk Heilig")
	e.gid(t, e.home, "name", "add", "priv", "Dirk H.")
	e.gid(t, e.home, "email", "add", "work", "dirk.heilig@example.com")
	e.gid(t, e.home, "email", "add", "priv", "dirk@example.org")
	e.gid(t, e.home, "key", "add", "work", workPriv, "--public", workPub)
	e.gid(t, e.home, "key", "add", "priv", privPriv, "--public", privPub)
	e.gid(t, e.home, "add", "work", "--name", "work", "--email", "work", "--key", "work")
	e.gid(t, e.home, "add", "priv", "--name", "priv", "--email", "priv", "--key", "priv")

	author := func(repo string) string {
		return e.must(t, filepath.Join(root, repo), "git", "log", "-1", "--format=%an <%ae>", "main")
	}
	wantLast := func(key, repo string, allowed bool) {
		t.Helper()
		got := srv.last(t)
		if got.Key != key || got.Repo != repo || got.Allowed != allowed {
			t.Fatalf("server saw %+v, want key=%s repo=%s allowed=%v", got, key, repo, allowed)
		}
	}

	work := e.newRepo(t, "work", srv.url("/work.git"))
	priv := e.newRepo(t, "priv", srv.url("/priv.git"))
	e.gid(t, work, "use", "work")
	e.gid(t, priv, "use", "priv")

	t.Run("each repo pushes with its identity", func(t *testing.T) {
		e.commit(t, work, "work 1")
		e.must(t, work, "git", "push", "-q", "origin", "main")
		wantLast("work", "/work.git", true)
		if got := author("work.git"); got != "Dirk Heilig <dirk.heilig@example.com>" {
			t.Errorf("work author = %q", got)
		}

		e.commit(t, priv, "priv 1")
		e.must(t, priv, "git", "push", "-q", "origin", "main")
		wantLast("priv", "/priv.git", true)
		if got := author("priv.git"); got != "Dirk H. <dirk@example.org>" {
			t.Errorf("priv author = %q", got)
		}
	})

	t.Run("wrong identity is rejected, switching fixes it", func(t *testing.T) {
		e.gid(t, work, "use", "priv")
		e.commit(t, work, "work 2")
		if out, err := e.run(work, "git", "push", "-q", "origin", "main"); err == nil {
			t.Fatalf("push with the wrong key succeeded:\n%s", out)
		}
		wantLast("priv", "/work.git", false)

		e.gid(t, work, "use", "work")
		e.must(t, work, "git", "push", "-q", "origin", "main")
		wantLast("work", "/work.git", true)
	})

	t.Run("key edit propagates without sync", func(t *testing.T) {
		rotPriv, rotPub, rotKey := writeKeypair(t, keys, "work-rotated")
		srv.authorize("work-rotated", rotKey, "/work.git")
		e.gid(t, e.home, "key", "edit", "work", "--private", rotPriv, "--public", rotPub)

		e.commit(t, work, "work 3")
		e.must(t, work, "git", "push", "-q", "origin", "main")
		wantLast("work-rotated", "/work.git", true)
	})

	t.Run("pinned book works without GIT_ID_BOOK in the environment", func(t *testing.T) {
		// git runs core.sshCommand in a shell that never saw --book or the
		// variable; the command itself must carry the book location.
		var vars []string
		for _, kv := range e.vars {
			if !strings.HasPrefix(kv, "GIT_ID_BOOK=") {
				vars = append(vars, kv)
			}
		}
		bare := *e
		bare.vars = vars
		cmd := e.must(t, work, "git", "config", "--local", "core.sshCommand")
		if !strings.HasPrefix(cmd, "GIT_ID_BOOK='") {
			t.Fatalf("core.sshCommand does not pin the book: %q", cmd)
		}
		e.commit(t, work, "work 4")
		bare.must(t, work, "git", "push", "-q", "origin", "main")
		wantLast("work-rotated", "/work.git", true)
	})

	t.Run("repo without identity cannot push", func(t *testing.T) {
		bare := e.newRepo(t, "bare", srv.url("/work.git"))
		e.must(t, bare, "git", "-c", "user.name=x", "-c", "user.email=x@y.z", "commit", "-q", "--allow-empty", "-m", "x")
		before := srv.count()
		if out, err := e.run(bare, "git", "push", "-q", "origin", "main"); err == nil {
			t.Fatalf("push without identity succeeded:\n%s", out)
		}
		if srv.count() != before {
			t.Error("server ran a git command for an unauthenticated client")
		}
		if out, err := e.run(bare, "git", "id", "current", "-q"); err == nil {
			t.Errorf("current -q succeeded without identity: %s", out)
		}
	})
}
