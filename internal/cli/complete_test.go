package cli

import (
	"strings"
	"testing"
)

// complete runs cobra's hidden __complete and returns the candidate labels
// (descriptions stripped) and the directive line.
func complete(t *testing.T, book string, args ...string) (labels []string, directive string) {
	t.Helper()
	out := mustRun(t, book, append([]string{"__complete"}, args...)...)
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.HasPrefix(l, ":") {
			directive = l
		} else if l != "" && !strings.HasPrefix(l, "Completion ended") {
			labels = append(labels, strings.SplitN(l, "\t", 2)[0])
		}
	}
	return labels, directive
}

func has(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func TestDynamicCompletion(t *testing.T) {
	book, _ := seed(t)
	mustRun(t, book, "email", "add", "priv", "d@e.f")
	mustRun(t, book, "email", "add", "other", "g@h.i")

	cases := []struct {
		args []string
		want []string
		not  []string
	}{
		{[]string{"use", ""}, []string{"work"}, nil},
		{[]string{"show", ""}, []string{"work"}, nil},
		{[]string{"edit", ""}, []string{"work"}, nil},
		{[]string{"rm", ""}, []string{"work"}, nil},
		{[]string{"name", "edit", ""}, []string{"me"}, nil},
		{[]string{"name", "rm", ""}, []string{"me"}, nil},
		{[]string{"email", "edit", ""}, []string{"work", "priv", "other"}, nil},
		{[]string{"email", "rm", "p"}, []string{"priv"}, []string{"work", "other"}},
		{[]string{"key", "edit", ""}, []string{"k1"}, nil},
		{[]string{"add", "x", "--name", ""}, []string{"me"}, []string{"work"}},
		{[]string{"add", "x", "--email", ""}, []string{"work", "priv"}, []string{"me"}},
		{[]string{"add", "x", "--key", ""}, []string{"k1"}, nil},
		{[]string{"edit", "work", "--email", ""}, []string{"priv"}, nil},
	}
	for _, c := range cases {
		got, _ := complete(t, book, c.args...)
		for _, w := range c.want {
			if !has(got, w) {
				t.Errorf("%v: missing %q in %v", c.args, w, got)
			}
		}
		for _, n := range c.not {
			if has(got, n) {
				t.Errorf("%v: unexpected %q in %v", c.args, n, got)
			}
		}
	}
}

func TestCompletionOnlyFirstArgAndFreeForm(t *testing.T) {
	book, _ := seed(t)
	// second positional of `name edit <label> <value>` is a free value.
	if got, _ := complete(t, book, "name", "edit", "me", ""); len(got) != 0 {
		t.Errorf("value position completed: %v", got)
	}
	// new labels are free-form.
	if got, _ := complete(t, book, "name", "add", ""); len(got) != 0 {
		t.Errorf("new label completed: %v", got)
	}
	if got, _ := complete(t, book, "use", "work", ""); len(got) != 0 {
		t.Errorf("second arg of use completed: %v", got)
	}
}

func TestCompletionMissingBookIsEmpty(t *testing.T) {
	book, _, _ := setup(t)
	if got, _ := complete(t, book, "use", ""); len(got) != 0 {
		t.Errorf("got %v from empty book", got)
	}
}

func TestCompletionScripts(t *testing.T) {
	book, _, _ := setup(t)
	for sh, want := range map[string]string{
		"bash":       "__start_git-id",
		"zsh":        "#compdef git-id",
		"fish":       "git-id",
		"powershell": "git-id",
	} {
		out := mustRun(t, book, "completion", sh)
		if !strings.Contains(out, want) {
			t.Errorf("%s script lacks %q", sh, want)
		}
	}
	if out := mustRun(t, book, "completion", "bash"); !strings.Contains(out, "_git_id()") {
		t.Error("bash script lacks git glue")
	}
	if _, err := run(t, book, "completion", "tcsh"); err == nil {
		t.Error("unknown shell accepted")
	}
}
