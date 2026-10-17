package cli

import "github.com/spf13/cobra"

// NewRootCmd builds the git-id command tree. Invoked as `git id` via git's
// subcommand lookup for the `git-id` binary. The command is named git-id so
// generated completion scripts target the binary, not git itself.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "git-id",
		Short:         "Manage git identities (name, email, ssh key); run as `git id`",
		SilenceUsage:  true,
		SilenceErrors: false,
	}
	root.PersistentFlags().String("book", "", "address book file (default $"+bookEnv+" or $XDG_CONFIG_HOME/git-id/book.toml)")
	root.AddCommand(
		newNameCmd(),
		newEmailCmd(),
		newKeyCmd(),
		newIdentityAddCmd(),
		newIdentityEditCmd(),
		newIdentityRmCmd(),
		newIdentityLsCmd(),
		newIdentityShowCmd(),
		newUseCmd(),
		newCurrentCmd(),
		newSyncCmd(),
		newSSHCmd(),
		newInitCmd(),
		newHookCmd(),
	)
	root.AddCommand(newCompletionCmd(root))
	attachCompletions(root)
	return root
}
