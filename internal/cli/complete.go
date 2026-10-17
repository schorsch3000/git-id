package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"git-id/internal/store"
)

type completer = func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective)

// completeLabels completes the labels of one datapoint kind, with the value as
// description, for the first positional argument only.
func completeLabels(kind store.Kind) completer {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return labelsOf(cmd, kind, toComplete), cobra.ShellCompDirectiveNoFileComp
	}
}

// completeIdentities completes identity labels for the first positional argument.
func completeIdentities(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return identitiesOf(cmd, toComplete), cobra.ShellCompDirectiveNoFileComp
}

// completeFlagLabels completes a flag value with labels of one kind.
func completeFlagLabels(kind store.Kind) completer {
	return func(cmd *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return labelsOf(cmd, kind, toComplete), cobra.ShellCompDirectiveNoFileComp
	}
}

// noCompletion offers nothing (free-form arguments such as new labels).
func noCompletion(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return nil, cobra.ShellCompDirectiveNoFileComp
}

func labelsOf(cmd *cobra.Command, kind store.Kind, prefix string) []string {
	var out []string
	_ = view(cmd, func(b *store.Book) error {
		for _, l := range b.Labels(kind) {
			if !strings.HasPrefix(l, prefix) {
				continue
			}
			var desc string
			switch kind {
			case store.KindName:
				desc = b.Names[l]
			case store.KindEmail:
				desc = b.Emails[l]
			case store.KindKey:
				desc = b.Keys[l].Private
			}
			out = append(out, l+"\t"+desc)
		}
		return nil
	})
	return out
}

func identitiesOf(cmd *cobra.Command, prefix string) []string {
	var out []string
	_ = view(cmd, func(b *store.Book) error {
		for _, l := range b.IdentityLabels() {
			if !strings.HasPrefix(l, prefix) {
				continue
			}
			desc := ""
			if r, err := b.Resolve(l); err == nil {
				desc = fmt.Sprintf("%s <%s>", r.Name, r.Email)
			}
			out = append(out, l+"\t"+desc)
		}
		return nil
	})
	return out
}

// attachCompletions wires dynamic completion into the finished command tree.
// Command paths are looked up by name so a rename fails loudly at startup.
func attachCompletions(root *cobra.Command) {
	find := func(path ...string) *cobra.Command {
		c, _, err := root.Find(path)
		if err != nil || c.Name() != path[len(path)-1] {
			panic(fmt.Sprintf("completion: no command %v", path))
		}
		return c
	}
	for _, k := range []store.Kind{store.KindName, store.KindEmail, store.KindKey} {
		kind := string(k)
		find(kind, "edit").ValidArgsFunction = completeLabels(k)
		find(kind, "rm").ValidArgsFunction = completeLabels(k)
		find(kind, "add").ValidArgsFunction = noCompletion
	}
	// `key add <label> <path>` and `key edit --private/--public` complete files
	// for the path; the label is free-form.
	find("key", "add").ValidArgsFunction = func(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return nil, cobra.ShellCompDirectiveDefault
	}
	find("add").ValidArgsFunction = noCompletion
	for _, n := range []string{"edit", "rm", "show", "use"} {
		find(n).ValidArgsFunction = completeIdentities
	}
	for _, n := range []string{"add", "edit"} {
		c := find(n)
		must(c.RegisterFlagCompletionFunc("name", completeFlagLabels(store.KindName)))
		must(c.RegisterFlagCompletionFunc("email", completeFlagLabels(store.KindEmail)))
		must(c.RegisterFlagCompletionFunc("key", completeFlagLabels(store.KindKey)))
	}
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

// bashGitGlue lets git's own bash completion complete `git id <TAB>` by
// delegating to the cobra completion of the git-id binary. git calls _git_id
// with `words`/`cword` describing the whole `git id ...` line; the cobra
// function reads COMP_*, so they are rebuilt for `git-id ...`. Completion
// assumes the cursor is at the end of the line.
const bashGitGlue = `
# git completion calls _git_id for ` + "`git id <TAB>`" + `.
_git_id() {
    local COMP_WORDS=(git-id "${words[@]:2}")
    local COMP_CWORD=$((cword - 1))
    local COMP_LINE="${COMP_WORDS[*]}"
    local COMP_POINT=${#COMP_LINE}
    __start_git-id
}
`

// zshGitGlue lets git's zsh completion complete `git id <TAB>`. _git calls the
// function _git-id with words[1] == "id"; the cobra function (also named
// _git-id, bound to the git-id command) expects words[1] to be the binary.
// The cobra function is kept under another name and wrapped.
const zshGitGlue = `
# git completion calls _git-id for ` + "`git id <TAB>`" + ` with words[1]=id.
functions[__git-id_cobra]=$functions[_git-id]
_git-id() {
    [[ $words[1] == id ]] && words[1]=git-id
    __git-id_cobra "$@"
}
`

// fishGitGlue lets fish complete `git id <TAB>`. fish's own git completion is
// meant to hand custom `git-*` commands over to their completions but does
// not do so for git-id, so the tokens after `git id` are passed on explicitly.
// Global git options before the subcommand (`git -C dir id`) are not handled.
const fishGitGlue = `
# complete ` + "`git id ...`" + ` with the completions of git-id.
function __git_id_in_git_id
    set -l tokens (commandline -opc)
    test (count $tokens) -ge 2; and test $tokens[2] = id
end
function __git_id_git_complete
    set -l tokens (commandline -opc)
    set -e tokens[1..2]
    set -l line (string join ' ' -- (string escape -- $tokens))
    complete -C "git-id $line "(commandline -ct)
end
complete -c git -f -n __git_id_in_git_id -a '(__git_id_git_complete)'
`

func newCompletionCmd(root *cobra.Command) *cobra.Command {
	return &cobra.Command{
		Use:   "completion bash|zsh|fish|powershell",
		Short: "Generate a shell completion script for git-id",
		Long: `Generate a shell completion script. Labels of names, emails, keys and
identities are completed dynamically from the address book.

  bash:  source <(git-id completion bash)
  zsh:   git-id completion zsh > "${fpath[1]}/_git-id"
  fish:  git-id completion fish | source

The scripts complete the git-id binary and, for bash, zsh and fish, the ` + "`git id`" + ` form.`,
		Args:      cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		ValidArgs: []string{"bash", "zsh", "fish", "powershell"},
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			switch args[0] {
			case "bash":
				if err := root.GenBashCompletionV2(out, true); err != nil {
					return err
				}
				_, err := fmt.Fprint(out, bashGitGlue)
				return err
			case "zsh":
				if err := root.GenZshCompletion(out); err != nil {
					return err
				}
				_, err := fmt.Fprint(out, zshGitGlue)
				return err
			case "fish":
				if err := root.GenFishCompletion(out, true); err != nil {
					return err
				}
				_, err := fmt.Fprint(out, fishGitGlue)
				return err
			default:
				return root.GenPowerShellCompletionWithDesc(out)
			}
		},
	}
}
