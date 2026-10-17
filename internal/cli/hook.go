package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"git-id/internal/repo"
)

// hintMessage is what the shell hook prints when a repository has no identity.
const hintMessage = "git-id: no identity set for this repository. Run: git id use <identity>   (git id ls lists them)"

// overrideMessage is printed when the repository has an identity but
// $GIT_SSH_COMMAND, which git prefers over core.sshCommand, is set.
const overrideMessage = "git-id: " + repo.SSHOverrideWarning

// shellOverride is overrideMessage escaped for use inside double quotes.
var shellOverride = strings.ReplaceAll(overrideMessage, "$", `\$`)

// The hooks only call git-id when the directory contains .git. Exit status 1
// of "git-id current -q" means "no identity"; a missing git-id binary (127)
// stays silent. With an identity set (status 0) they also check the
// environment for $GIT_SSH_COMMAND.

var bashHook = `# git-id: remind when entering a git repository without an identity.
__git_id_last=
__git_id_hook() {
    [ "$PWD" = "$__git_id_last" ] && return 0
    __git_id_last=$PWD
    [ -e .git ] || return 0
    command -v git-id >/dev/null 2>&1 || return 0
    git-id current -q 2>/dev/null
    local rc=$?
    if [ $rc -eq 1 ]; then
        echo "` + hintMessage + `" >&2
    elif [ $rc -eq 0 ] && [ -n "${GIT_SSH_COMMAND-}" ]; then
        echo "` + shellOverride + `" >&2
    fi
    return 0
}
if [[ "$(declare -p PROMPT_COMMAND 2>/dev/null)" == "declare -a"* ]]; then
    case " ${PROMPT_COMMAND[*]} " in *" __git_id_hook "*) ;; *) PROMPT_COMMAND+=(__git_id_hook) ;; esac
else
    case ";${PROMPT_COMMAND};" in *";__git_id_hook;"*) ;; *) PROMPT_COMMAND="__git_id_hook${PROMPT_COMMAND:+;$PROMPT_COMMAND}" ;; esac
fi
`

var zshHook = `# git-id: remind when entering a git repository without an identity.
__git_id_hook() {
    [[ -e .git ]] || return 0
    (( $+commands[git-id] )) || return 0
    git-id current -q 2>/dev/null
    local rc=$?
    if [[ $rc -eq 1 ]]; then
        print -u2 -- "` + hintMessage + `"
    elif [[ $rc -eq 0 && -n "${GIT_SSH_COMMAND-}" ]]; then
        print -u2 -- "` + shellOverride + `"
    fi
    return 0
}
autoload -Uz add-zsh-hook
add-zsh-hook chpwd __git_id_hook
__git_id_hook
`

var fishHook = `# git-id: remind when entering a git repository without an identity.
function __git_id_hook --on-variable PWD
    test -e .git; or return 0
    command -q git-id; or return 0
    git-id current -q 2>/dev/null
    set -l rc $status
    if test $rc -eq 1
        echo "` + hintMessage + `" >&2
    else if test $rc -eq 0; and test -n "$GIT_SSH_COMMAND"
        echo "` + shellOverride + `" >&2
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
