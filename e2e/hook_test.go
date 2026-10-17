package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// hookEnv prepares an address book with identity "work" and these places:
// plain (no .git anywhere above), a repository without identity (bare) and one
// using "work" (withID), each with subdirectories.
type hookEnv struct {
	*env
	plain                   string
	bare, bareSub, bareDeep string
	withID, idSub, idDeep   string
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
	h.bare = e.newRepo(t, "noid", "ssh://x/y.git")
	h.withID = e.newRepo(t, "withid", "ssh://x/y.git")
	e.gid(t, h.withID, "use", "work")
	h.bareSub, h.bareDeep = filepath.Join(h.bare, "sub"), filepath.Join(h.bare, "sub", "deeper")
	h.idSub, h.idDeep = filepath.Join(h.withID, "sub"), filepath.Join(h.withID, "sub", "deeper")
	for _, d := range []string{h.plain, h.bareDeep, h.idDeep} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return h
}

// hookStep is one directory change; ssh exports $GIT_SSH_COMMAND first.
type hookStep struct {
	dir string
	ssh bool
}

// scenario walks through the places the hook must tell apart. Expected
// outcome: two reminders for the repository without identity (steps 2 and 6;
// 3 and 4 stay in the same repository, 6 enters it again through a
// subdirectory), none for the one with an identity, none outside repositories,
// and one $GIT_SSH_COMMAND warning (step 9, not repeated in step 10).
func (h *hookEnv) scenario() []hookStep {
	return []hookStep{
		{h.plain, false},    // 1: no repository
		{h.bare, false},     // 2: enter the repository without identity: reminder
		{h.bareDeep, false}, // 3: still inside: silent
		{h.bare, false},     // 4: still inside: silent
		{h.plain, false},    // 5: leave
		{h.bareSub, false},  // 6: enter again, through a subdirectory: reminder
		{h.idDeep, false},   // 7: repository with identity, entered deep inside: silent
		{h.plain, true},     // 8: leave, export GIT_SSH_COMMAND
		{h.idSub, true},     // 9: warning about GIT_SSH_COMMAND
		{h.idDeep, true},    // 10: same repository: no second warning
	}
}

// perDir splits shell output on "== <dir>" markers and counts the lines that
// contain needle after each marker.
func perDir(out, needle string) map[string]int {
	counts := map[string]int{}
	var cur string
	for _, l := range strings.Split(out, "\n") {
		if dir, ok := strings.CutPrefix(l, "== "); ok {
			cur = dir
			continue
		}
		if cur != "" && strings.Contains(l, needle) {
			counts[cur]++
		}
	}
	return counts
}

// under sums the counts of every directory inside root (root included).
func under(counts map[string]int, root string) int {
	n := 0
	for dir, c := range counts {
		if dir == root || strings.HasPrefix(dir, root+"/") {
			n += c
		}
	}
	return n
}

func (h *hookEnv) check(t *testing.T, shell, out string) {
	t.Helper()
	nags := perDir(out, "no identity set")
	if got := under(nags, h.bare); got != 2 {
		t.Errorf("%s: %d reminders in the repository without identity, want 2\n%s", shell, got, out)
	}
	for name, root := range map[string]string{"with identity": h.withID, "outside": h.plain} {
		if got := under(nags, root); got != 0 {
			t.Errorf("%s: %d reminders %s, want 0\n%s", shell, got, name, out)
		}
	}
	// The reminders sit where they should: entering at the root (step 2) and
	// through a subdirectory (step 6), nowhere in between.
	if nags[h.bare] != 1 || nags[h.bareSub] != 1 || nags[h.bareDeep] != 0 {
		t.Errorf("%s: reminders per directory = %v", shell, nags)
	}

	warns := perDir(out, "GIT_SSH_COMMAND is set")
	if warns[h.idSub] != 1 || under(warns, h.withID) != 1 || under(warns, h.bare) != 0 || under(warns, h.plain) != 0 {
		t.Errorf("%s: $GIT_SSH_COMMAND warnings per directory = %v, want exactly one in %s\n%s", shell, warns, h.idSub, out)
	}
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
	var sb strings.Builder
	sb.WriteString(`
eval "$(git-id hook bash)"
eval "$(git-id hook bash)"   # sourcing twice must not register twice
echo "PC=${PROMPT_COMMAND}" >&2
step() { echo "== $1" >&2; cd "$1" && __git_id_hook; }
`)
	for _, s := range h.scenario() {
		if s.ssh {
			sb.WriteString("export GIT_SSH_COMMAND=ssh\n")
		}
		sb.WriteString("step " + s.dir + "\n")
	}
	out, err := h.run(h.home, "bash", "-c", sb.String())
	if err != nil {
		t.Fatalf("bash: %v\n%s", err, out)
	}
	if n := strings.Count(out, "PC=__git_id_hook"); n != 1 || strings.Count(strings.SplitN(out, "\n", 2)[0], "__git_id_hook") != 1 {
		t.Errorf("hook not registered exactly once:\n%s", out)
	}
	h.check(t, "bash", out)
}

// TestZshHook relies on zsh's real chpwd hook: plain `cd` triggers it.
func TestZshHook(t *testing.T) {
	requireShell(t, "zsh")
	h := newHookEnv(t)
	var sb strings.Builder
	sb.WriteString(`
eval "$(git-id hook zsh)"
print -u2 "chpwd_functions=${chpwd_functions}"
step() { print -u2 "== $1"; cd "$1" }
`)
	for _, s := range h.scenario() {
		if s.ssh {
			sb.WriteString("export GIT_SSH_COMMAND=ssh\n")
		}
		sb.WriteString("step " + s.dir + "\n")
	}
	out, err := h.run(h.home, "zsh", "-f", "-c", sb.String())
	if err != nil {
		t.Fatalf("zsh: %v\n%s", err, out)
	}
	if !strings.Contains(out, "__git_id_hook") {
		t.Errorf("hook not registered in chpwd_functions:\n%s", out)
	}
	h.check(t, "zsh", out)
}

// TestFishHook relies on fish's real --on-variable PWD event: plain `cd`
// triggers it.
func TestFishHook(t *testing.T) {
	requireShell(t, "fish")
	h := newHookEnv(t)
	var sb strings.Builder
	sb.WriteString(`
git-id hook fish | source
function step
    echo "== $argv[1]" >&2
    cd $argv[1]
end
`)
	for _, s := range h.scenario() {
		if s.ssh {
			sb.WriteString("set -gx GIT_SSH_COMMAND ssh\n")
		}
		sb.WriteString("step " + s.dir + "\n")
	}
	out, err := h.run(h.home, "fish", "--no-config", "-c", sb.String())
	if err != nil {
		t.Fatalf("fish: %v\n%s", err, out)
	}
	h.check(t, "fish", out)
}

// TestHooksSilentWithoutBinary runs each hook with a PATH lacking git-id: the
// hook text comes from the real binary, but the shell cannot find it.
func TestHooksSilentWithoutBinary(t *testing.T) {
	h := newHookEnv(t)
	cases := []struct {
		shell string
		args  []string
		run   string // how to enter the repository and trigger the hook
	}{
		{"bash", []string{"-c"}, "cd " + h.bareDeep + " && __git_id_hook"},
		{"zsh", []string{"-f", "-c"}, "cd " + h.bareDeep},
		{"fish", []string{"--no-config", "-c"}, "cd " + h.bareDeep},
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
