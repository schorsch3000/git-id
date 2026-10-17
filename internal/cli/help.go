package cli

import (
	"fmt"
	"io"
	"regexp"
	"strings"
	"text/template"
	"unicode"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

// helpStyles colour the help output. They come from a renderer bound to the
// writer, so colours appear only when that writer is a terminal that supports
// them; NO_COLOR switches them off and CLICOLOR_FORCE switches them on.
type helpStyles struct{ heading, typed, dim, warn lipgloss.Style }

func newHelpStyles(w io.Writer) helpStyles {
	r := lipgloss.NewRenderer(w)
	return helpStyles{
		heading: r.NewStyle().Bold(true),
		typed:   r.NewStyle().Bold(true).Foreground(lipgloss.Color("14")), // what you type: commands, flags
		dim:     r.NewStyle().Faint(true),
		warn:    r.NewStyle().Bold(true).Foreground(lipgloss.Color("9")),
	}
}

// flagRe finds the flag names at the start of a line of pflag's usage output.
var flagRe = regexp.MustCompile(`^(\s+)((?:-\w, )?--[A-Za-z0-9-]+|-\w)`)

func (s helpStyles) funcs() template.FuncMap {
	return template.FuncMap{
		"h":    func(t string) string { return s.heading.Render(t) },
		"dim":  func(t string) string { return s.dim.Render(t) },
		"path": func(t string) string { return s.typed.Render(t) },
		"useline": func(c *cobra.Command) string {
			line := c.UseLine()
			if p := c.CommandPath(); strings.HasPrefix(line, p) {
				return s.typed.Render(p) + line[len(p):]
			}
			return line
		},
		"cmdpad": func(name string, pad int) string {
			return s.typed.Render(name) + strings.Repeat(" ", max(pad-len(name), 0))
		},
		"flags": func(usages string) string {
			lines := strings.Split(strings.TrimRightFunc(usages, unicode.IsSpace), "\n")
			for i, l := range lines {
				if m := flagRe.FindStringSubmatchIndex(l); m != nil {
					lines[i] = l[:m[4]] + s.typed.Render(l[m[4]:m[5]]) + l[m[5]:]
				}
			}
			return strings.Join(lines, "\n")
		},
		"more": func(path string) string {
			return s.dim.Render(fmt.Sprintf("Use \"%s [command] --help\" for more information about a command.", path))
		},
	}
}

// usageTemplate is cobra's default usage template without command groups,
// with the parts you type or read as headings run through the style functions.
const usageTemplate = `{{h "Usage:"}}{{if .Runnable}}
  {{useline .}}{{end}}{{if .HasAvailableSubCommands}}
  {{path .CommandPath}} [command]{{end}}{{if gt (len .Aliases) 0}}

{{h "Aliases:"}}
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

{{h "Examples:"}}
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}

{{h "Available Commands:"}}{{range .Commands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{cmdpad .Name .NamePadding}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

{{h "Flags:"}}
{{flags .LocalFlags.FlagUsages}}{{end}}{{if .HasAvailableInheritedFlags}}

{{h "Global Flags:"}}
{{flags .InheritedFlags.FlagUsages}}{{end}}{{if .HasAvailableSubCommands}}

{{more .CommandPath}}{{end}}
`

func renderUsage(c *cobra.Command, w io.Writer) error {
	t, err := template.New("usage").Funcs(newHelpStyles(w).funcs()).Parse(usageTemplate)
	if err != nil {
		return err
	}
	return t.Execute(w, c)
}

// installHelp makes --help and usage errors coloured.
func installHelp(root *cobra.Command) {
	root.SetUsageFunc(func(c *cobra.Command) error { return renderUsage(c, c.OutOrStderr()) })
	root.SetHelpFunc(func(c *cobra.Command, _ []string) {
		out := c.OutOrStdout()
		if text := c.Long; text != "" || c.Short != "" {
			if text == "" {
				text = c.Short
			}
			fmt.Fprintf(out, "%s\n\n", newHelpStyles(out).long(strings.TrimRightFunc(text, unicode.IsSpace)))
		}
		if c.Runnable() || c.HasSubCommands() {
			_ = renderUsage(c, out)
		}
	})
}

var (
	// exampleRe matches an example line of a long description: indented
	// "git id ..." command, optionally followed by two or more spaces and a comment.
	exampleRe = regexp.MustCompile(`^(\s+git id .*?\S)(\s{2,}\S.*)?$`)
	warnRe    = regexp.MustCompile(`THIS REWRITES HISTORY\.?`)
)

// long colours a command's long description: example commands like the usage
// they show, and the history-rewriting warning.
func (s helpStyles) long(text string) string {
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		if m := exampleRe.FindStringSubmatchIndex(l); m != nil {
			cmd := strings.TrimLeft(l[m[2]:m[3]], " ")
			lines[i] = l[:m[3]-len(cmd)] + s.typed.Render(cmd) + l[m[3]:]
			continue
		}
		lines[i] = warnRe.ReplaceAllStringFunc(l, func(w string) string { return s.warn.Render(w) })
	}
	return strings.Join(lines, "\n")
}
