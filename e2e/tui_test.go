package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tuiDriver runs `git-id tui` in a pty (via zsh's zpty) and plays a script of
// steps. STEPS holds one step per line: key tokens separated by \x1e, then
// \x1f, then the text to wait for. Every token is written separately because
// bubbletea reads a multi-key write as one pasted string; a text token is fine
// as it is inserted into the focused input in one go. The first step has no
// keys and waits for the initial screen.
const tuiDriver = `
zmodload zsh/zpty
cd $REPO
zpty tui "git-id tui"
out=
wait_for() {
  local want=$1
  for i in {1..100}; do
    while zpty -r -t tui chunk 2>/dev/null; do out+=$chunk; done
    [[ $out == *$want* ]] && return 0
    sleep 0.1
  done
  return 1
}
for step in "${(@f)STEPS}"; do
  keys=${step%%$'\x1f'*}
  want=${step#*$'\x1f'}
  out=
  local -a toks
  toks=("${(@ps:\x1e:)keys}")
  for tok in "${toks[@]}"; do
    [[ -n $tok ]] && { zpty -w -n tui "$tok"; sleep 0.1 }
  done
  if ! wait_for "$want"; then
    print -r -- "TIMEOUT waiting for: $want"
    print -r -- "${out//$'\x1b'/<ESC>}"
    zpty -d tui
    exit 3
  fi
done
zpty -w -n tui q
sleep 0.3
zpty -d tui 2>/dev/null
exit 0
`

type tuiStep struct {
	keys []string
	want string
}

func runTUI(t *testing.T, e *env, repo string, steps []tuiStep) {
	t.Helper()
	var lines []string
	for _, s := range steps {
		lines = append(lines, strings.Join(s.keys, "\x1e")+"\x1f"+s.want)
	}
	e2 := *e
	e2.vars = append(append([]string{}, e.vars...), "TERM=xterm", "REPO="+repo, "STEPS="+strings.Join(lines, "\n"))
	if out, err := e2.run(e.home, "zsh", "-f", "-c", tuiDriver); err != nil {
		t.Fatalf("tui session failed: %v\n%s", err, out)
	}
}

// TestTUISession drives the real binary in a real pseudo-terminal: pick an
// identity for the repository, then add a name through the form.
func TestTUISession(t *testing.T) {
	requireShell(t, "zsh")
	e := completionEnv(t)
	repo := e.newRepo(t, "tuirepo", "ssh://x/y.git")
	if out, err := e.run(repo, "git", "id", "current", "-q"); err == nil {
		t.Fatalf("repo already has an identity: %s", out)
	}

	runTUI(t, e, repo, []tuiStep{
		{nil, "id-one"}, // initial screen lists the identity
		{[]string{"\r"}, "using identity id-one"},
		{[]string{"l", "a", "nm-two", "\t", "Nina", "\r"}, "added name nm-two"},
	})

	if got := e.must(t, repo, "git", "config", "--local", "user.email"); got != "w@x.de" {
		t.Errorf("user.email = %q", got)
	}
	if got := e.gid(t, repo, "current"); got != "id-one" {
		t.Errorf("current = %q", got)
	}
	if out := e.gid(t, e.home, "name", "ls"); !strings.Contains(out, "nm-two") || !strings.Contains(out, "Nina") {
		t.Errorf("name not added through the TUI:\n%s", out)
	}
}

func TestTUIRefusesWithoutTerminal(t *testing.T) {
	e := completionEnv(t)
	out, err := e.run(e.home, "git", "id", "tui")
	if err == nil || !strings.Contains(out, "interactive terminal") {
		t.Errorf("err %v, out %q", err, out)
	}
}

// TestTUIKeyPicker opens the key form in a real terminal, picks a key from
// ~/.ssh with ctrl+o and saves it.
func TestTUIKeyPicker(t *testing.T) {
	requireShell(t, "zsh")
	e := completionEnv(t)
	sshDir := filepath.Join(e.home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatal(err)
	}
	priv, pub, _ := writeKeypair(t, sshDir, "id_picked")
	repo := e.newRepo(t, "pickrepo", "ssh://x/y.git")

	runTUI(t, e, repo, []tuiStep{
		{nil, "id-one"},
		{[]string{"l", "l", "l", "a"}, "ctrl+o pick a key"}, // keys tab, add form
		{[]string{"\x0f"}, "id_picked"},                     // the picker lists the key
		{[]string{"\r"}, "picked"},                          // chosen: the form shows path and label
		{[]string{"\x13"}, "added key picked"},              // ctrl+s saves
	})

	out := e.gid(t, e.home, "key", "ls")
	line := ""
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "picked") {
			line = l
		}
	}
	if !strings.Contains(line, priv) || !strings.Contains(line, pub) || !strings.Contains(line, "SHA256:") {
		t.Errorf("key not added through the picker, key ls:\n%s", out)
	}
}
