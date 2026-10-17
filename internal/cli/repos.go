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

func newUnuseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unuse",
		Short: "Remove the identity from the current repository again",
		Long: `Remove user.name, user.email, id.identity and the core.sshCommand written by
git-id from the current repository and stop tracking it for "git id sync".
A core.sshCommand that git-id did not write is left alone.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := bookPath(cmd)
			if err != nil {
				return err
			}
			top, kept, err := repo.Unuse(path, "")
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "removed the identity from %s\n", top)
			if kept {
				fmt.Fprintln(cmd.ErrOrStderr(), "note: core.sshCommand was not written by git-id and was left in place")
			}
			return nil
		},
	}
}

// repoInfo describes one repository registered for sync.
type repoInfo struct {
	Path     string `json:"path"`
	Identity string `json:"identity,omitempty"`
	Status   string `json:"status"` // ok, missing, no-identity, unknown-identity
}

func repoStatus(b *store.Book, dir string) repoInfo {
	info := repoInfo{Path: dir}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		info.Status = "missing"
		return info
	}
	label, ok, err := gitcfg.GetLocal(dir, repo.IdentityKey)
	switch {
	case err != nil:
		info.Status = "missing"
	case !ok || label == "":
		info.Status = "no-identity"
	default:
		info.Identity = label
		info.Status = "ok"
		if _, err := b.Resolve(label); err != nil {
			info.Status = "unknown-identity"
		}
	}
	return info
}

func newReposCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "repos",
		Short: "Show or forget the repositories tracked for sync",
	}
	root.AddCommand(
		addJSONFlag(&cobra.Command{
			Use:   "ls",
			Short: "List repositories that git id use registered, with their identity and state",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				path, err := bookPath(cmd)
				if err != nil {
					return err
				}
				return view(cmd, func(b *store.Book) error {
					dirs, err := store.LoadRepos(store.RegistryPath(path))
					if err != nil {
						return err
					}
					rows := []repoInfo{}
					for _, d := range dirs {
						rows = append(rows, repoStatus(b, d))
					}
					if wantJSON(cmd) {
						return printJSON(cmd, rows)
					}
					var t [][]string
					for _, r := range rows {
						t = append(t, []string{r.Path, r.Identity, r.Status})
					}
					return printTable(cmd, t)
				})
			},
		}),
		&cobra.Command{
			Use:   "forget <path>",
			Short: "Stop tracking a repository for sync (its configuration is not touched)",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				path, err := bookPath(cmd)
				if err != nil {
					return err
				}
				abs, err := filepath.Abs(args[0])
				if err != nil {
					return err
				}
				reg := store.RegistryPath(path)
				dirs, err := store.LoadRepos(reg)
				if err != nil {
					return err
				}
				found := false
				for _, d := range dirs {
					found = found || d == abs || d == args[0]
				}
				if !found {
					return fmt.Errorf("%s is not tracked", args[0])
				}
				if err := store.RemoveRepo(reg, abs); err != nil {
					return err
				}
				return store.RemoveRepo(reg, args[0])
			},
		},
	)
	return root
}
