package e2e

import (
	"strings"
	"testing"
	"time"
)

// startupDriver runs a command in a pseudo-terminal and prints everything it
// writes until WANT shows up (or 4 s pass). A terminal that never answers a
// query makes a program stall, which shows up here as WANT missing.
const startupDriver = `
zmodload zsh/zpty
zpty p "$CMD"
out=
for i in {1..40}; do
  while zpty -r -t p chunk 2>/dev/null; do out+=$chunk; done
  [[ $out == *$WANT* ]] && break
  sleep 0.1
done
print -rn -- "$out"
zpty -d p 2>/dev/null
`

func inTerminal(t *testing.T, e *env, extraEnv []string, cmd, want string) (string, time.Duration) {
	t.Helper()
	e2 := *e
	e2.vars = append(append([]string{}, e.vars...), append(extraEnv, "TERM=xterm-256color", "CMD="+cmd, "WANT="+want)...)
	start := time.Now()
	out, err := e2.run(e.home, "zsh", "-f", "-c", startupDriver)
	if err != nil {
		t.Fatalf("%s: %v\n%s", cmd, err, out)
	}
	return out, time.Since(start)
}

// TestNoTerminalQueryAtStartup guards internal/termquiet: without it,
// bubbletea's init writes an OSC 11 background-colour query and a cursor
// position request on every start, and the command waits for an answer that a
// plain pty never sends.
func TestNoTerminalQueryAtStartup(t *testing.T) {
	requireShell(t, "zsh")
	e := completionEnv(t)
	for _, c := range []struct{ cmd, want string }{
		{"git-id --version", "git-id version"},
		{"git-id current -q; echo done_$((6*7))", "done_42"}, // what the shell hook runs
		{"git-id name ls", "nm-one"},
	} {
		out, _ := inTerminal(t, e, nil, c.cmd, c.want)
		if !strings.Contains(out, c.want) {
			t.Errorf("%q printed nothing useful within 4 s (stalled on a terminal query?):\n%q", c.cmd, out)
		}
		if strings.Contains(out, "\x1b]11;") || strings.Contains(out, "\x1b[6n") {
			t.Errorf("%q queried the terminal:\n%q", c.cmd, out)
		}
	}
}

// TestHelpColoursOnATerminal checks the real decision made for a tty: colours
// on a terminal, off with NO_COLOR.
func TestHelpColoursOnATerminal(t *testing.T) {
	requireShell(t, "zsh")
	e := completionEnv(t)

	out, _ := inTerminal(t, e, nil, "git-id use --help", "Global Flags")
	for _, want := range []string{"\x1b[1mUsage:\x1b[0m", "--force\x1b[0m"} {
		if !strings.Contains(out, want) {
			t.Errorf("help on a terminal lacks %q:\n%q", want, out)
		}
	}

	out, _ = inTerminal(t, e, []string{"NO_COLOR=1"}, "git-id use --help", "Global Flags")
	if strings.Contains(out, "\x1b[") {
		t.Errorf("NO_COLOR ignored on a terminal:\n%q", out)
	}
	if !strings.Contains(out, "Usage:") {
		t.Errorf("no help text:\n%q", out)
	}
}
