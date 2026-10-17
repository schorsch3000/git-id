package cleanup

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
)

// ErrAborted is returned when the user quits or the input ends.
var ErrAborted = errors.New("aborted, nothing changed")

// Target is the identity values findings are replaced with.
type Target struct{ Name, Email string }

func (t Target) For(f Finding) string {
	if f.Kind == Name {
		return t.Name
	}
	return t.Email
}

// AcceptAll returns the plan that replaces every finding.
func AcceptAll(t Target, groups ...[]Finding) Plan {
	p := NewPlan()
	for _, g := range groups {
		for _, f := range g {
			p.Add(f, t.For(f))
		}
	}
	return p
}

// Ask goes through the findings, names first, and returns the plan the user
// chose. Per finding: y replaces it, n keeps it, a replaces it and every
// remaining finding of the same kind (batch by name or by address), s keeps
// it and every remaining one of the kind, q aborts.
func Ask(in *bufio.Reader, out io.Writer, t Target, names, emails []Finding) (Plan, error) {
	plan := NewPlan()
	for _, group := range [][]Finding{names, emails} {
		mode := byte(0) // 'a' accept rest, 's' skip rest
		for _, f := range group {
			switch mode {
			case 'a':
				plan.Add(f, t.For(f))
				continue
			case 's':
				continue
			}
			fmt.Fprintf(out, "\n[%s] %q  (%d commits)  ->  %q\n", f.Kind, f.Value, f.Count, t.For(f))
			for {
				fmt.Fprintf(out, "  replace? [y]es  [n]o  [a]ll remaining %ss  [s]kip remaining %ss  [q]uit > ", f.Kind, f.Kind)
				line, err := in.ReadString('\n')
				if err != nil && strings.TrimSpace(line) == "" {
					fmt.Fprintln(out)
					return Plan{}, ErrAborted
				}
				answer := strings.ToLower(strings.TrimSpace(line))
				if answer == "" {
					continue
				}
				switch answer[0] {
				case 'y':
					plan.Add(f, t.For(f))
				case 'n':
				case 'a':
					plan.Add(f, t.For(f))
					mode = 'a'
				case 's':
					mode = 's'
				case 'q':
					return Plan{}, ErrAborted
				default:
					fmt.Fprintf(out, "  please answer y, n, a, s or q\n")
					continue
				}
				break
			}
		}
	}
	return plan, nil
}

// Confirm asks for the final go-ahead; only the full word "yes" counts.
func Confirm(in *bufio.Reader, out io.Writer, prompt string) bool {
	fmt.Fprintf(out, "%s type \"yes\" to continue > ", prompt)
	line, _ := in.ReadString('\n')
	return strings.TrimSpace(line) == "yes"
}
