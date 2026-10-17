package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"git-id/internal/gitcfg"
	"git-id/internal/repo"
	"git-id/internal/store"
)

func newUseCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "use <identity>",
		Short: "Use an identity in the current repository",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := bookPath(cmd)
			if err != nil {
				return err
			}
			b, err := store.Load(path)
			if err != nil {
				return err
			}
			top, err := repo.Use(path, b, args[0], "", force)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "using identity %s in %s\n", args[0], top)
			warnSSHOverride(cmd)
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "replace an existing foreign core.sshCommand")
	return cmd
}

func newCurrentCmd() *cobra.Command {
	var quiet bool
	cmd := &cobra.Command{
		Use:   "current",
		Short: "Show the identity of the current repository (exit 1 if none)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceErrors = quiet
			label, ok, err := repo.Identity("")
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("no identity set; run `git id use <identity>`")
			}
			return view(cmd, func(b *store.Book) error {
				if _, err := b.Resolve(label); err != nil {
					return fmt.Errorf("repository uses identity %q which is unusable: %w", label, err)
				}
				if !quiet {
					fmt.Fprintln(cmd.OutOrStdout(), label)
					warnSSHOverride(cmd)
				}
				return nil
			})
		},
	}
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "print nothing, only set the exit status")
	return cmd
}

func newSyncCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "Re-apply identities to all repositories that use one",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := bookPath(cmd)
			if err != nil {
				return err
			}
			b, err := store.Load(path)
			if err != nil {
				return err
			}
			regPath := store.RegistryPath(path)
			repos, err := store.LoadRepos(regPath)
			if err != nil {
				return err
			}
			out, errOut := cmd.OutOrStdout(), cmd.ErrOrStderr()
			var keep []string
			failed := 0
			for _, dir := range repos {
				label, ok, err := repoIdentity(dir)
				if err != nil || !ok {
					fmt.Fprintf(out, "dropped %s (gone or no identity)\n", dir)
					continue
				}
				keep = append(keep, dir)
				r, err := b.Resolve(label)
				if err == nil {
					err = repo.Apply(dir, path, r, false)
				}
				if err != nil {
					failed++
					fmt.Fprintf(errOut, "%s: %v\n", dir, err)
					continue
				}
				fmt.Fprintf(out, "synced %s (%s)\n", dir, label)
			}
			if err := store.SaveRepos(regPath, keep); err != nil {
				return err
			}
			if failed > 0 {
				return fmt.Errorf("%d repositories failed to sync", failed)
			}
			return nil
		},
	}
}

// repoIdentity returns the identity label set in the repository at dir, if it still exists.
func repoIdentity(dir string) (string, bool, error) {
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return "", false, fmt.Errorf("missing")
	}
	label, ok, err := gitcfg.GetLocal(dir, repo.IdentityKey)
	return label, ok && label != "", err
}

// sshArgv builds the ssh command line for an identity's key.
func sshArgv(r store.Resolved, rest []string) []string {
	return append([]string{"ssh", "-i", r.Key.Private, "-o", "IdentitiesOnly=yes"}, rest...)
}

// newSSHCmd is the wrapper behind core.sshCommand. It resolves the key from
// the address book on every call, so key edits propagate without a sync.
func newSSHCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "ssh <identity> [ssh args...]",
		Short:              "Run ssh with the key of an identity (used via core.sshCommand)",
		Hidden:             true,
		DisableFlagParsing: true,
		Args:               cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var r store.Resolved
			err := view(cmd, func(b *store.Book) (err error) {
				r, err = b.Resolve(args[0])
				return err
			})
			if err != nil {
				return err
			}
			return runSSH(sshArgv(r, args[1:]))
		},
	}
}

// detectKey finds a default ssh key in ~/.ssh.
func detectKey() (priv, pub string) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", ""
	}
	for _, n := range []string{"id_ed25519", "id_ecdsa", "id_rsa"} {
		p := filepath.Join(home, ".ssh", n)
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			priv = p
			if _, err := os.Stat(p + ".pub"); err == nil {
				pub = p + ".pub"
			}
			return priv, pub
		}
	}
	return "", ""
}

func newInitCmd() *cobra.Command {
	var label, keyArg string
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Import the global git name/email into the address book, remove them from global config and set user.useConfigOnly",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := store.ValidateLabel(label); err != nil {
				return err
			}
			name, hasName, err := gitcfg.GetGlobal("user.name")
			if err != nil {
				return err
			}
			email, hasEmail, err := gitcfg.GetGlobal("user.email")
			if err != nil {
				return err
			}
			if !hasName && !hasEmail {
				return fmt.Errorf("no global user.name or user.email set; nothing to import")
			}

			var key store.Key
			if keyArg != "" {
				if key, err = buildKey(keyArg, keyArg+".pub", fileExists(keyArg+".pub")); err != nil {
					return err
				}
			} else if priv, pub := detectKey(); priv != "" {
				key = store.Key{Private: priv, Public: pub}
			}

			out := cmd.OutOrStdout()
			err = modify(cmd, func(b *store.Book) error {
				if hasName {
					if err := b.AddName(label, name); err != nil {
						return err
					}
					fmt.Fprintf(out, "imported name %s = %s\n", label, name)
				}
				if hasEmail {
					if err := b.AddEmail(label, email); err != nil {
						return err
					}
					fmt.Fprintf(out, "imported email %s = %s\n", label, email)
				}
				if key.Private != "" {
					if err := b.AddKey(label, key); err != nil {
						return err
					}
					fmt.Fprintf(out, "imported key %s = %s\n", label, key.Private)
				}
				if hasName && hasEmail && key.Private != "" {
					if err := b.AddIdentity(label, store.Identity{Name: label, Email: label, Key: label}); err != nil {
						return err
					}
					fmt.Fprintf(out, "created identity %s\n", label)
				} else {
					fmt.Fprintln(out, "no identity created: need a global name, email and an ssh key (use --key)")
				}
				return nil
			})
			if err != nil {
				return fmt.Errorf("%w (use --label to import under another label)", err)
			}
			for _, k := range []string{"user.name", "user.email"} {
				if err := gitcfg.UnsetGlobal(k); err != nil {
					return fmt.Errorf("book saved, but removing global %s failed: %w", k, err)
				}
			}
			fmt.Fprintln(out, "removed global user.name and user.email")
			// Without this git would guess an author from the user and host
			// name in every repository that has no identity.
			if err := gitcfg.SetGlobal("user.useConfigOnly", "true"); err != nil {
				return fmt.Errorf("book saved, but setting user.useConfigOnly failed: %w", err)
			}
			fmt.Fprintln(out, "set user.useConfigOnly = true: git no longer guesses an author, repositories without an identity cannot commit")
			return nil
		},
	}
	cmd.Flags().StringVar(&label, "label", "default", "label for the imported datapoints and identity")
	cmd.Flags().StringVar(&keyArg, "key", "", "private key to import (default: first of ~/.ssh/id_ed25519, id_ecdsa, id_rsa)")
	return cmd
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// warnSSHOverride tells the user when $GIT_SSH_COMMAND defeats the identity's key.
func warnSSHOverride(cmd *cobra.Command) {
	if repo.SSHOverridden() {
		fmt.Fprintln(cmd.ErrOrStderr(), repo.SSHOverrideWarning)
	}
}
