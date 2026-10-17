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

// The hooks look for .git in the current directory and its parents and remind
// once per entry into a repository. Exit status 1
// of "git-id current -q" means "no identity"; a missing git-id binary (127)
// stays silent. With an identity set (status 0) they also check the
// environment for $GIT_SSH_COMMAND.

var bashHook = `# git-id: remind when entering a git repository without an identity.
__git_id_last=
# __git_id_alert <sgr colour> <message>: coloured on a terminal (NO_COLOR switches it off), plain otherwise.
__git_id_alert() {
    if [ -t 2 ] && [ -z "${NO_COLOR-}" ]; then printf '\033[%sm%s\033[0m\n' "$1" "$2" >&2; else printf '%s\n' "$2" >&2; fi
}
__git_id_hook() {
    # Find the repository root by walking up looking for .git, without running git.
    local d=$PWD
    while [ -n "$d" ] && [ ! -e "$d/.git" ]; do d=${d%/*}; done
    if [ -z "$d" ]; then __git_id_last=; return 0; fi
    # One reminder per entry into a repository, not per directory inside it.
    [ "$d" = "$__git_id_last" ] && return 0
    __git_id_last=$d
    command -v git-id >/dev/null 2>&1 || return 0
    git-id current -q 2>/dev/null
    local rc=$?
    if [ $rc -eq 1 ]; then
        __git_id_alert "1;91" "` + hintMessage + `"
    elif [ $rc -eq 0 ] && [ -n "${GIT_SSH_COMMAND-}" ]; then
        __git_id_alert "1;93" "` + shellOverride + `"
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
typeset -g __git_id_last=
# __git_id_alert <sgr colour> <message>: coloured on a terminal (NO_COLOR switches it off), plain otherwise.
__git_id_alert() {
    if [[ -t 2 && -z "${NO_COLOR-}" ]]; then printf '\033[%sm%s\033[0m\n' "$1" "$2" >&2; else printf '%s\n' "$2" >&2; fi
}
__git_id_hook() {
    # Find the repository root by walking up looking for .git, without running git.
    local d=$PWD
    while [[ -n $d && ! -e $d/.git ]]; do d=${d%/*}; done
    if [[ -z $d ]]; then __git_id_last=; return 0; fi
    # One reminder per entry into a repository, not per directory inside it.
    [[ $d == "$__git_id_last" ]] && return 0
    __git_id_last=$d
    (( $+commands[git-id] )) || return 0
    git-id current -q 2>/dev/null
    local rc=$?
    if [[ $rc -eq 1 ]]; then
        __git_id_alert "1;91" "` + hintMessage + `"
    elif [[ $rc -eq 0 && -n "${GIT_SSH_COMMAND-}" ]]; then
        __git_id_alert "1;93" "` + shellOverride + `"
    fi
    return 0
}
autoload -Uz add-zsh-hook
add-zsh-hook chpwd __git_id_hook
__git_id_hook
`

var fishHook = `# git-id: remind when entering a git repository without an identity.
set -g __git_id_last ''
# __git_id_alert <sgr colour> <message>: coloured on a terminal (NO_COLOR switches it off), plain otherwise.
function __git_id_alert
    if isatty stderr; and test -z "$NO_COLOR"
        printf '\033[%sm%s\033[0m\n' $argv[1] $argv[2] >&2
    else
        printf '%s\n' $argv[2] >&2
    end
end
function __git_id_hook --on-variable PWD
    # Find the repository root by walking up looking for .git, without running git.
    set -l d $PWD
    while test -n "$d"; and not test -e "$d/.git"
        set d (string replace -r '/[^/]*$' '' -- $d)
    end
    if test -z "$d"
        set -g __git_id_last ''
        return 0
    end
    # One reminder per entry into a repository, not per directory inside it.
    test "$d" = "$__git_id_last"; and return 0
    set -g __git_id_last $d
    command -q git-id; or return 0
    git-id current -q 2>/dev/null
    set -l rc $status
    if test $rc -eq 1
        __git_id_alert "1;91" "` + hintMessage + `"
    else if test $rc -eq 0; and test -n "$GIT_SSH_COMMAND"
        __git_id_alert "1;93" "` + shellOverride + `"
    end
end
__git_id_hook
`

func newHookCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "hook bash|zsh|fish",
		Short: "Print a shell hook that reminds you to set an identity in repositories without one",
		Long: `Print a shell snippet that checks, whenever you enter a git repository, at its
root or in any subdirectory, whether the repository has an identity
("git-id current -q") and prints a reminder if not. Add it to your shell
startup file:

  bash:  eval "$(git-id hook bash)"      # ~/.bashrc
  zsh:   eval "$(git-id hook zsh)"       # ~/.zshrc
  fish:  git-id hook fish | source       # ~/.config/fish/config.fish

You are reminded once per entry into a repository: moving between its
subdirectories is silent, leaving the repository and coming back reminds again.
The repository is found by looking for .git in the current directory and its
parents, so the hook never starts git.`,
		Args:      cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		ValidArgs: []string{"bash", "zsh", "fish"},
		RunE: func(cmd *cobra.Command, args []string) error {
			snippets := map[string]string{"bash": bashHook, "zsh": zshHook, "fish": fishHook}
			_, err := fmt.Fprint(cmd.OutOrStdout(), snippets[args[0]])
			return err
		},
	}
}
