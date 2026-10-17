package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// hookEnv prepares an address book with identity "work" and three directories:
// plain (no .git), bare (repo without identity) and withID (repo using "work").
type hookEnv struct {
	*env
	plain, bare, withID string
}

func newHookEnv(t *testing.T) *hookEnv {
	t.Helper()
	e := newEnv(t)
	key, _, _ := writeKeypair(t, e.home, "k")
	e.gid(t, e.home, "name", "add", "me", "Dirk")
	e.gid(t, e.home, "email", "add", "work", "w@x.de")
	e.gid(t, e.home, "key", "add", "k1", key)
	e.gid(t, e.home, "add", "work", "--name", "me", "--email", "work", "--key", "k1")

	h := &hookEnv{env: e, plain: filepath.Join(e.home, "plain")}
	if err := os.MkdirAll(h.plain, 0o755); err != nil {
		t.Fatal(err)
	}
	h.bare = e.newRepo(t, "noid", "ssh://x/y.git")
	h.withID = e.newRepo(t, "withid", "ssh://x/y.git")
	e.gid(t, h.withID, "use", "work")
	return h
}

// nagCounts splits shell output on "== <dir>" markers and counts reminders
// printed after each marker.
func nagCounts(out string) map[string]int {
	counts := map[string]int{}
	var cur string
	for _, l := range strings.Split(out, "\n") {
		if dir, ok := strings.CutPrefix(l, "== "); ok {
			cur = dir
			counts[cur] += 0
			continue
		}
		if cur != "" && strings.Contains(l, "no identity set") {
			counts[cur]++
		}
	}
	return counts
}

func requireShell(t *testing.T, name string) {
	t.Helper()
	if _, err := exec.LookPath(name); err != nil {
		t.Skipf("%s not available", name)
	}
}

// TestBashHook sources the generated hook in a real bash. Non-interactive bash
// has no prompt, so the hook function is called where PROMPT_COMMAND would
// call it.
func TestBashHook(t *testing.T) {
	requireShell(t, "bash")
	h := newHookEnv(t)
	script := `
eval "$(git-id hook bash)"
eval "$(git-id hook bash)"   # sourcing twice must not register twice
echo "PC=${PROMPT_COMMAND}" >&2
step() { echo "== $1" >&2; cd "$1" && __git_id_hook; }
step ` + h.plain + `
step ` + h.bare + `
step ` + h.bare + `
step ` + h.withID + `
step ` + h.bare + `
`
	out, err := h.run(h.home, "bash", "-c", script)
	if err != nil {
		t.Fatalf("bash: %v\n%s", err, out)
	}
	if n := strings.Count(out, "PC=__git_id_hook"); n != 1 || strings.Count(strings.SplitN(out, "\n", 2)[0], "__git_id_hook") != 1 {
		t.Errorf("hook not registered exactly once:\n%s", out)
	}
	n := nagCounts(out)
	if n[h.plain] != 0 || n[h.withID] != 0 {
		t.Errorf("nagged where it must not: %v\n%s", n, out)
	}
	// bare is entered twice in a row (nag once) and once more later (nag again).
	if n[h.bare] != 2 {
		t.Errorf("nags in repo without identity = %d, want 2\n%s", n[h.bare], out)
	}
}

// TestZshHook relies on zsh's real chpwd hook: plain `cd` triggers it.
func TestZshHook(t *testing.T) {
	requireShell(t, "zsh")
	h := newHookEnv(t)
	script := `
eval "$(git-id hook zsh)"
print -u2 "chpwd_functions=${chpwd_functions}"
step() { print -u2 "== $1"; cd "$1" }
step ` + h.plain + `
step ` + h.bare + `
step ` + h.withID + `
step ` + h.plain + `
step ` + h.bare + `
`
	out, err := h.run(h.home, "zsh", "-f", "-c", script)
	if err != nil {
		t.Fatalf("zsh: %v\n%s", err, out)
	}
	if !strings.Contains(out, "__git_id_hook") {
		t.Errorf("hook not registered in chpwd_functions:\n%s", out)
	}
	n := nagCounts(out)
	if n[h.plain] != 0 || n[h.withID] != 0 {
		t.Errorf("nagged where it must not: %v\n%s", n, out)
	}
	if n[h.bare] != 2 {
		t.Errorf("nags in repo without identity = %d, want 2 (once per entry)\n%s", n[h.bare], out)
	}
}

// TestFishHook relies on fish's real --on-variable PWD event: plain `cd`
// triggers it.
func TestFishHook(t *testing.T) {
	requireShell(t, "fish")
	h := newHookEnv(t)
	script := `
git-id hook fish | source
function step
    echo "== $argv[1]" >&2
    cd $argv[1]
end
step ` + h.plain + `
step ` + h.bare + `
step ` + h.withID + `
step ` + h.plain + `
step ` + h.bare + `
`
	out, err := h.run(h.home, "fish", "--no-config", "-c", script)
	if err != nil {
		t.Fatalf("fish: %v\n%s", err, out)
	}
	n := nagCounts(out)
	if n[h.plain] != 0 || n[h.withID] != 0 {
		t.Errorf("nagged where it must not: %v\n%s", n, out)
	}
	if n[h.bare] != 2 {
		t.Errorf("nags in repo without identity = %d, want 2 (once per entry)\n%s", n[h.bare], out)
	}
}

// TestHooksSilentWithoutBinary runs each hook with a PATH lacking git-id: the
// hook text comes from the real binary, but the shell cannot find it.
func TestHooksSilentWithoutBinary(t *testing.T) {
	h := newHookEnv(t)
	cases := []struct {
		shell string
		args  []string
		run   string // how to enter the repo and trigger the hook
	}{
		{"bash", []string{"-c"}, "cd " + h.bare + " && __git_id_hook"},
		{"zsh", []string{"-f", "-c"}, "cd " + h.bare},
		{"fish", []string{"--no-config", "-c"}, "cd " + h.bare},
	}
	for _, c := range cases {
		t.Run(c.shell, func(t *testing.T) {
			requireShell(t, c.shell)
			hook := h.gid(t, h.home, "hook", c.shell)
			bin, _ := exec.LookPath(c.shell)
			cmd := exec.Command(bin, append(c.args, hook+"\n"+c.run+"\n")...)
			cmd.Dir = h.home
			cmd.Env = append(os.Environ(), "PATH=/usr/bin:/bin", "HOME="+h.home)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("%s: %v\n%s", c.shell, err, out)
			}
			if strings.Contains(string(out), "no identity") {
				t.Errorf("nagged although git-id is not installed:\n%s", out)
			}
		})
	}
}
