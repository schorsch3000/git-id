package cli

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"git-id/internal/cleanup"
	"git-id/internal/gitcfg"
	"git-id/internal/repo"
	"git-id/internal/store"
)

func newCleanupCmd() *cobra.Command {
	var currentBranch, dryRun, yes bool
	cmd := &cobra.Command{
		Use:   "cleanup",
		Short: "Rewrite history so other identities' names and emails become the current identity",
		Long: `Scan the history of this repository for author and committer names and email
addresses that are in the address book but do not belong to the identity this
repository uses (see "git id current"), and offer to replace them with the
current identity's. Values not in the address book are never touched.

For every finding you are asked: y(es) replaces it, n(o) keeps it, a(ll) replaces
it and all remaining findings of the same kind (batch by name, or by address),
s(kip) keeps it and all remaining ones of that kind, q(uit) aborts.

THIS REWRITES HISTORY. Commit hashes change, signatures of rewritten commits are
lost, and by default ALL branches, tags and remote-tracking refs are rewritten.
Use --current-branch to limit it to the checked-out branch. Before anything is
changed a bundle of all refs is written into the git directory and the command
to restore it is printed. Nothing is pushed. Needs git-filter-repo
(not bundled), a clean work tree, and a typed "yes" before it rewrites.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCleanup(cmd, currentBranch, dryRun, yes)
		},
	}
	cmd.Flags().BoolVar(&currentBranch, "current-branch", false, "rewrite only the checked-out branch instead of all refs")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would be replaced and change nothing")
	cmd.Flags().BoolVar(&yes, "yes", false, "accept every finding and skip the confirmation (for scripts)")
	return cmd
}

func runCleanup(cmd *cobra.Command, currentBranch, dryRun, yes bool) error {
	out := cmd.OutOrStdout()
	in := bufio.NewReader(cmd.InOrStdin())

	label, ok, err := repo.Identity("")
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("this repository has no identity; run `git id use <identity>` first")
	}
	var book *store.Book
	var cur store.Resolved
	err = view(cmd, func(b *store.Book) (err error) {
		book = b
		cur, err = b.Resolve(label)
		return err
	})
	if err != nil {
		return fmt.Errorf("current identity %q is unusable: %w", label, err)
	}
	top, err := gitcfg.Toplevel("")
	if err != nil {
		return err
	}

	ref, scope := "", "ALL branches, tags and remote-tracking refs"
	if currentBranch {
		if ref, err = gitcfg.Git(top, "symbolic-ref", "-q", "HEAD"); err != nil {
			return fmt.Errorf("--current-branch needs a checked-out branch (HEAD is detached)")
		}
		scope = "branch " + strings.TrimPrefix(ref, "refs/heads/") + " only"
	}

	if !dryRun {
		if err := cleanup.CheckFilterRepo(top); err != nil {
			return err
		}
		if dirty, err := cleanup.IsDirty(top); err != nil {
			return err
		} else if dirty {
			return fmt.Errorf("the work tree has uncommitted changes to tracked files; commit or stash them first")
		}
	}

	commits, err := cleanup.Scan(top, ref)
	if err != nil {
		return err
	}
	names, emails := cleanup.Find(commits, cleanup.KnownFrom(book, cur))
	fmt.Fprintf(out, "Identity of this repository: %s  (%s <%s>)\n", label, cur.Name, cur.Email)
	fmt.Fprintf(out, "Scanned %d commits (%s).\n", len(commits), scope)
	if len(names) == 0 && len(emails) == 0 {
		fmt.Fprintln(out, "Nothing to clean up: no other identity of the address book appears in this history.")
		return nil
	}

	fmt.Fprintln(out, "\nFound values of other identities:")
	for _, f := range append(append([]cleanup.Finding{}, names...), emails...) {
		fmt.Fprintf(out, "  %-5s %q  in %d commits\n", f.Kind, f.Value, f.Count)
	}
	target := cleanup.Target{Name: cur.Name, Email: cur.Email}

	if dryRun {
		fmt.Fprintf(out, "\nDry run: with %s rewritten, every value above would become %q (names) or %q (emails). Nothing was changed.\n",
			scope, target.Name, target.Email)
		return nil
	}

	banner(out, scope)
	var plan cleanup.Plan
	if yes {
		plan = cleanup.AcceptAll(target, names, emails)
	} else if plan, err = cleanup.Ask(in, out, target, names, emails); err != nil {
		return err
	}
	if plan.Empty() {
		fmt.Fprintln(out, "\nNothing selected, nothing changed.")
		return nil
	}

	affected := plan.Affected(commits)
	fmt.Fprintf(out, "\n%d of %d scanned commits carry a replaced value; they and all their descendants get new hashes.\n", len(affected), len(commits))
	if signed, err := cleanup.CountSigned(top, ref); err != nil {
		return err
	} else if signed > 0 {
		fmt.Fprintf(out, "WARNING: %d of the %d scanned commits are signed. git filter-repo drops the signature of every commit it rewrites, "+
			"signed commits of other authors included, and their hashes change too.\n", signed, len(commits))
	}
	if pushed, err := cleanup.PushedCount(top, affected); err != nil {
		return err
	} else if pushed > 0 {
		fmt.Fprintf(out, "WARNING: %d of them are already in remote-tracking refs, probably pushed. "+
			"Publishing the result needs a force push and affects everyone who has the old history.\n", pushed)
	}
	if !yes && !cleanup.Confirm(in, out, "Rewrite history now?") {
		fmt.Fprintln(out, "Aborted, nothing changed.")
		return cleanup.ErrAborted
	}

	backup, err := cleanup.Backup(top)
	if err != nil {
		return fmt.Errorf("could not write the backup, nothing was changed: %w", err)
	}
	fmt.Fprintf(out, "Backup of all refs: %s\n", backup)
	if err := cleanup.Rewrite(top, plan, ref); err != nil {
		return fmt.Errorf("%w\nThe backup is at %s; restore with: %s", err, backup, cleanup.RestoreHint(backup))
	}
	fmt.Fprintf(out, "Done: history rewritten (%s).\n", scope)
	fmt.Fprintf(out, "To undo: %s\n", cleanup.RestoreHint(backup))
	fmt.Fprintln(out, "Nothing was pushed. To publish: git push --force-with-lease --all && git push --force --tags")
	return nil
}

func banner(out interface{ Write([]byte) (int, error) }, scope string) {
	line := strings.Repeat("!", 72)
	fmt.Fprintf(out, "\n%s\n!! THIS REWRITES GIT HISTORY: commit hashes change, signatures are lost.\n!! Scope: %s\n%s\n", line, scope, line)
}
