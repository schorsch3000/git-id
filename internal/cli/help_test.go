package cli

import (
	"regexp"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// helpOf returns the --help output of the command at path.
func helpOf(t *testing.T, root *cobra.Command, path ...string) string {
	t.Helper()
	var out strings.Builder
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(append(path, "--help"))
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

// allPaths lists the path of every command, hidden ones excluded.
func allPaths(c *cobra.Command, prefix []string) [][]string {
	paths := [][]string{prefix}
	for _, sub := range c.Commands() {
		if sub.Hidden {
			continue
		}
		paths = append(paths, allPaths(sub, append(append([]string{}, prefix...), sub.Name()))...)
	}
	return paths
}

// Without colours the custom help must read exactly like cobra's own.
func TestHelpWithoutColourMatchesCobraDefault(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	custom := NewRootCmd()
	plain := NewRootCmd()
	plain.SetHelpFunc(nil)
	plain.SetUsageFunc(nil)
	for _, p := range allPaths(custom, nil) {
		got, want := helpOf(t, custom, p...), helpOf(t, plain, p...)
		if got != want {
			t.Errorf("help of %q differs from cobra's default\n--- got\n%s\n--- want\n%s", p, got, want)
		}
	}
}

func TestHelpIsColouredOnlyWhenWanted(t *testing.T) {
	// A buffer is not a terminal: no colours by default, as when piping into less.
	t.Setenv("CLICOLOR_FORCE", "")
	t.Setenv("NO_COLOR", "")
	plain := helpOf(t, NewRootCmd())
	if strings.Contains(plain, "\x1b[") {
		t.Errorf("escape sequences without a terminal:\n%q", plain)
	}

	t.Setenv("CLICOLOR_FORCE", "1")
	coloured := helpOf(t, NewRootCmd())
	if !strings.Contains(coloured, "\x1b[") {
		t.Fatal("CLICOLOR_FORCE did not turn colours on")
	}
	if ansiRe.ReplaceAllString(coloured, "") != plain {
		t.Errorf("colour changed the text:\n%s", ansiRe.ReplaceAllString(coloured, ""))
	}

	// NO_COLOR wins over CLICOLOR_FORCE.
	t.Setenv("NO_COLOR", "1")
	if out := helpOf(t, NewRootCmd()); strings.Contains(out, "\x1b[") {
		t.Errorf("NO_COLOR ignored:\n%q", out)
	}
}

func TestColouredHelpHighlightsWhatYouType(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("CLICOLOR_FORCE", "1")
	s := newHelpStyles(&strings.Builder{})
	typed, head := s.typed.Render, s.heading.Render

	root := helpOf(t, NewRootCmd())
	for _, want := range []string{
		head("Usage:"), head("Available Commands:"), head("Flags:"),
		typed("git-id"), typed("use"), typed("cleanup"), typed("--book"), typed("-h, --help"),
		typed("git id init"), // an example line of the long description
	} {
		if !strings.Contains(root, want) {
			t.Errorf("root help lacks %q:\n%q", want, root)
		}
	}

	use := helpOf(t, NewRootCmd(), "use")
	for _, want := range []string{typed("git-id use"), typed("--force"), head("Global Flags:")} {
		if !strings.Contains(use, want) {
			t.Errorf("use help lacks %q:\n%q", want, use)
		}
	}

	cleanup := helpOf(t, NewRootCmd(), "cleanup")
	if !strings.Contains(cleanup, s.warn.Render("THIS REWRITES HISTORY.")) {
		t.Errorf("the history warning is not highlighted:\n%q", cleanup)
	}
	for _, flag := range []string{"--dry-run", "--yes", "--current-branch"} {
		if !strings.Contains(cleanup, typed(flag)) {
			t.Errorf("flag %s not highlighted", flag)
		}
	}
}
