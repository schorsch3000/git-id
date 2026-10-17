package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestBashGitCompletion drives the generated bash script the way git's own
// completion does: it calls _git_id with `words`/`cword` for a `git id ...`
// line and reads COMPREPLY.
func TestBashGitCompletion(t *testing.T) {
	const bashCompletion = "/usr/share/bash-completion/bash_completion"
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
	if _, err := os.Stat(bashCompletion); err != nil {
		t.Skip("bash-completion not installed")
	}
	e := newEnv(t)
	keyPriv, _, _ := writeKeypair(t, e.home, "k")
	e.gid(t, e.home, "name", "add", "me", "Dirk")
	e.gid(t, e.home, "email", "add", "work", "w@x.de")
	e.gid(t, e.home, "email", "add", "priv", "p@x.de")
	e.gid(t, e.home, "key", "add", "k1", keyPriv)
	e.gid(t, e.home, "add", "work", "--name", "me", "--email", "work", "--key", "k1")

	// complete runs _git_id for the given words and returns COMPREPLY.
	complete := func(words ...string) []string {
		t.Helper()
		quoted := make([]string, len(words))
		for i, w := range words {
			quoted[i] = "'" + w + "'"
		}
		script := `
source ` + bashCompletion + `
source <(git-id completion bash)
words=(` + strings.Join(quoted, " ") + `)
cword=$((${#words[@]} - 1))
COMP_WORDS=("${words[@]}"); COMP_CWORD=$cword
COMP_LINE="${words[*]}"; COMP_POINT=${#COMP_LINE}
_git_id
printf '%s\n' "${COMPREPLY[@]}"
`
		out, err := e.run(e.home, "bash", "-c", script)
		if err != nil {
			t.Fatalf("bash: %v\n%s", err, out)
		}
		var got []string
		for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
			if l != "" {
				got = append(got, strings.Fields(l)[0])
			}
		}
		return got
	}
	contains := func(list []string, s string) bool {
		for _, x := range list {
			if x == s {
				return true
			}
		}
		return false
	}

	if got := complete("git", "id", "use", ""); !contains(got, "work") {
		t.Errorf("git id use <TAB> = %v, want work", got)
	}
	if got := complete("git", "id", "add", "x", "--email", ""); !contains(got, "work") || !contains(got, "priv") || contains(got, "me") {
		t.Errorf("git id add x --email <TAB> = %v", got)
	}
	if got := complete("git", "id", "em"); !contains(got, "email") {
		t.Errorf("git id em<TAB> = %v, want email", got)
	}
	if got := complete("git", "id", "email", "rm", "p"); !contains(got, "priv") || contains(got, "work") {
		t.Errorf("git id email rm p<TAB> = %v", got)
	}
}

// completionEnv creates an address book with distinctive labels so candidates
// can be told apart from anything else the shell prints.
func completionEnv(t *testing.T) *env {
	t.Helper()
	e := newEnv(t)
	keyPriv, _, _ := writeKeypair(t, e.home, "k")
	e.gid(t, e.home, "name", "add", "nm-one", "Dirk")
	e.gid(t, e.home, "email", "add", "em-one", "w@x.de")
	e.gid(t, e.home, "email", "add", "em-two", "p@x.de")
	e.gid(t, e.home, "key", "add", "ky-one", keyPriv)
	e.gid(t, e.home, "add", "id-one", "--name", "nm-one", "--email", "em-one", "--key", "ky-one")
	return e
}

type compCase struct {
	line string   // what the user typed before pressing TAB
	want []string // must be offered
	not  []string // must not be offered
}

var compCases = []compCase{
	{line: "git id use ", want: []string{"id-one"}, not: []string{"em-one", "nm-one", "ky-one"}},
	{line: "git id add x --email ", want: []string{"em-one", "em-two"}, not: []string{"nm-one", "ky-one", "id-one"}},
	{line: "git id email rm ", want: []string{"em-one", "em-two"}, not: []string{"nm-one", "ky-one", "id-one"}},
}

// ptyDriver types a line followed by TAB into a real interactive shell running
// in a pty and prints everything the shell wrote. zsh's zpty module provides
// the pty, so fish is driven through a zsh helper too.
const ptyDriver = `
zmodload zsh/zpty
zpty sh "$SHELL_CMD"
sleep $STARTUP
out=; while zpty -r -t sh chunk 2>/dev/null; do out+=$chunk; done
zpty -w sh "$SETUP; echo SETUP_$((1+1))"$'\r'
for i in {1..100}; do
  while zpty -r -t sh chunk 2>/dev/null; do out+=$chunk; done
  [[ $out == *SETUP_2* ]] && break
  sleep 0.1
done
out=
zpty -w -n sh "$LINE$TABS"
for i in {1..60}; do
  while zpty -r -t sh chunk 2>/dev/null; do out+=$chunk; done
  [[ $out == *$WANT* ]] && break
  sleep 0.1
done
print -r -- "$out"
zpty -d sh
`

var ansiRe = regexp.MustCompile(`\x1b(\[[0-9;?<>=]*[ -/]*[@-~]|\][^\x07]*\x07|\([A-Z0-9]|[=>])`)

// pressTab runs c.line + TAB in a real shell and returns the cleaned output.
func pressTab(t *testing.T, e *env, shellCmd, setup, tabs, startup string, c compCase) string {
	t.Helper()
	e2 := *e
	e2.vars = append(append([]string{}, e.vars...),
		"TERM=xterm", "SHELL_CMD="+shellCmd, "SETUP="+setup, "LINE="+c.line,
		"TABS="+tabs, "STARTUP="+startup, "WANT="+c.want[len(c.want)-1])
	out, err := e2.run(e.home, "zsh", "-f", "-c", ptyDriver)
	if err != nil {
		t.Fatalf("%s %q: %v\n%s", shellCmd, c.line, err, out)
	}
	return ansiRe.ReplaceAllString(out, "")
}

// TestFishGitCompletion drives a real interactive fish.
func TestFishGitCompletion(t *testing.T) {
	requireShell(t, "fish")
	requireShell(t, "zsh")
	e := completionEnv(t)
	for _, c := range compCases {
		// A second TAB opens the pager when the first only inserted a common prefix.
		out := pressTab(t, e, "fish --no-config -i", "git-id completion fish | source", "\t\t", "2", c)
		checkCandidates(t, "fish", c, out)
	}
}

// TestZshGitCompletion drives a real interactive zsh with compinit and git's
// own _git completion loaded.
func TestZshGitCompletion(t *testing.T) {
	requireShell(t, "zsh")
	e := completionEnv(t)
	setup := "PS1='> '; unsetopt list_ambiguous; zstyle ':completion:*' menu no; " +
		"bindkey '^I' complete-word; autoload -Uz compinit; compinit -u -d " +
		filepath.Join(e.home, "zcompdump") + "; source <(git-id completion zsh)"
	for _, c := range compCases {
		out := pressTab(t, e, "zsh -f -i", setup, "\t", "0", c)
		checkCandidates(t, "zsh", c, out)
	}
}

func checkCandidates(t *testing.T, shell string, c compCase, out string) {
	t.Helper()
	for _, w := range c.want {
		if !strings.Contains(out, w) {
			t.Errorf("%s %q: %q not offered\n%s", shell, c.line, w, out)
		}
	}
	for _, n := range c.not {
		if strings.Contains(out, n) {
			t.Errorf("%s %q: %q offered but must not be\n%s", shell, c.line, n, out)
		}
	}
}
