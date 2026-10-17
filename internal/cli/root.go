package cli

import "github.com/spf13/cobra"

// version is set at build time: -ldflags "-X git-id/internal/cli.version=1.2.3".
var version = "dev"

// NewRootCmd builds the git-id command tree. Invoked as `git id` via git's
// subcommand lookup for the `git-id` binary. The command is named git-id so
// generated completion scripts target the binary, not git itself.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "git-id",
		Short: "Manage git identities (name, email, ssh key); run as `git id`",
		Long: `git-id keeps an address book of names, email addresses and ssh keys. An
identity combines one of each. Using an identity in a repository sets
user.name and user.email there and points core.sshCommand at git-id, which
picks the identity's ssh key on every push and fetch. Identities reference
the address book entries, so changing a name, email or key changes every
identity that uses it.

Typical use:

  git id init                         import the global name and email
  git id key add work ~/.ssh/id_work  add more datapoints
  git id add work --name me --email work-mail --key work
  git id use work                     in each repository
  git id sync                         after editing the address book
  git id tui                          the same interactively

It is installed as git-id, so git runs it as "git id".`,
		SilenceUsage:  true,
		SilenceErrors: false,
		Version:       version,
	}
	root.PersistentFlags().String("book", "", "address book file (default $"+bookEnv+" or $XDG_CONFIG_HOME/git-id/book.toml)")
	root.AddCommand(
		newNameCmd(),
		newEmailCmd(),
		newKeyCmd(),
		newIdentityAddCmd(),
		newIdentityEditCmd(),
		newIdentityRenameCmd(),
		newIdentityRmCmd(),
		newIdentityLsCmd(),
		newIdentityShowCmd(),
		newUseCmd(),
		newUnuseCmd(),
		newReposCmd(),
		newCurrentCmd(),
		newSyncCmd(),
		newSSHCmd(),
		newInitCmd(),
		newHookCmd(),
		newTUICmd(),
		newCleanupCmd(),
	)
	root.AddCommand(newCompletionCmd(root))
	attachCompletions(root)
	installHelp(root)
	return root
}
