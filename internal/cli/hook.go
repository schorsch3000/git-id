package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// hintMessage is what the shell hook prints when a repository has no identity.
const hintMessage = "git-id: no identity set for this repository. Run: git id use <identity>   (git id ls lists them)"

// The hooks only call git-id when the directory contains .git, and only
// complain on exit status 1 (git-id's "no identity" status), so a missing
// git-id binary (127) stays silent.

const bashHook = `# git-id: remind when entering a git repository without an identity.
__git_id_last=
__git_id_hook() {
    [ "$PWD" = "$__git_id_last" ] && return 0
    __git_id_last=$PWD
    [ -e .git ] || return 0
    command -v git-id >/dev/null 2>&1 || return 0
    git-id current -q 2>/dev/null
    [ $? -eq 1 ] && echo "` + hintMessage + `" >&2
    return 0
}
if [[ "$(declare -p PROMPT_COMMAND 2>/dev/null)" == "declare -a"* ]]; then
    case " ${PROMPT_COMMAND[*]} " in *" __git_id_hook "*) ;; *) PROMPT_COMMAND+=(__git_id_hook) ;; esac
else
    case ";${PROMPT_COMMAND};" in *";__git_id_hook;"*) ;; *) PROMPT_COMMAND="__git_id_hook${PROMPT_COMMAND:+;$PROMPT_COMMAND}" ;; esac
fi
`

const zshHook = `# git-id: remind when entering a git repository without an identity.
__git_id_hook() {
    [[ -e .git ]] || return 0
    (( $+commands[git-id] )) || return 0
    git-id current -q 2>/dev/null
    [[ $? -eq 1 ]] && print -u2 -- "` + hintMessage + `"
    return 0
}
autoload -Uz add-zsh-hook
add-zsh-hook chpwd __git_id_hook
__git_id_hook
`

const fishHook = `# git-id: remind when entering a git repository without an identity.
function __git_id_hook --on-variable PWD
    test -e .git; or return 0
    command -q git-id; or return 0
    git-id current -q 2>/dev/null
    if test $status -eq 1
        echo "` + hintMessage + `" >&2
    end
end
__git_id_hook
`

func newHookCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "hook bash|zsh|fish",
		Short: "Print a shell hook that reminds you to set an identity in repositories without one",
		Long: `Print a shell snippet that checks, whenever you enter a directory containing
.git, whether the repository has an identity ("git-id current -q") and prints a
reminder if not. Add it to your shell startup file:

  bash:  eval "$(git-id hook bash)"      # ~/.bashrc
  zsh:   eval "$(git-id hook zsh)"       # ~/.zshrc
  fish:  git-id hook fish | source       # ~/.config/fish/config.fish

Only directories that directly contain .git are checked, not subdirectories.`,
		Args:      cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		ValidArgs: []string{"bash", "zsh", "fish"},
		RunE: func(cmd *cobra.Command, args []string) error {
			snippets := map[string]string{"bash": bashHook, "zsh": zshHook, "fish": fishHook}
			_, err := fmt.Fprint(cmd.OutOrStdout(), snippets[args[0]])
			return err
		},
	}
}
