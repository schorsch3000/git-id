package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"git-id/internal/repo"
	"git-id/internal/store"
)

func newIdentityAddCmd() *cobra.Command {
	var id store.Identity
	cmd := &cobra.Command{
		Use:   "add <identity> --name <label> --email <label> --key <label>",
		Short: "Add an identity from existing datapoints",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return modify(cmd, func(b *store.Book) error { return b.AddIdentity(args[0], id) })
		},
	}
	cmd.Flags().StringVar(&id.Name, "name", "", "name label")
	cmd.Flags().StringVar(&id.Email, "email", "", "email label")
	cmd.Flags().StringVar(&id.Key, "key", "", "key label")
	for _, f := range []string{"name", "email", "key"} {
		_ = cmd.MarkFlagRequired(f)
	}
	return cmd
}

func newIdentityEditCmd() *cobra.Command {
	var name, email, key string
	cmd := &cobra.Command{
		Use:   "edit <identity> [--name <label>] [--email <label>] [--key <label>]",
		Short: "Point an identity at different datapoints",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			f := cmd.Flags()
			if !f.Changed("name") && !f.Changed("email") && !f.Changed("key") {
				return fmt.Errorf("nothing to change: give --name, --email and/or --key")
			}
			return modify(cmd, func(b *store.Book) error {
				id, ok := b.Identities[args[0]]
				if !ok {
					return fmt.Errorf("identity %q: %w", args[0], store.ErrNotFound)
				}
				if f.Changed("name") {
					id.Name = name
				}
				if f.Changed("email") {
					id.Email = email
				}
				if f.Changed("key") {
					id.Key = key
				}
				return b.SetIdentity(args[0], id)
			})
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "name label")
	cmd.Flags().StringVar(&email, "email", "", "email label")
	cmd.Flags().StringVar(&key, "key", "", "key label")
	return cmd
}

func newIdentityRmCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rm <identity>",
		Short: "Remove an identity (datapoints stay)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return modify(cmd, func(b *store.Book) error { return b.RemoveIdentity(args[0]) })
		},
	}
}

func newIdentityLsCmd() *cobra.Command {
	return addJSONFlag(&cobra.Command{
		Use:   "ls",
		Short: "List identities",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return view(cmd, func(b *store.Book) error {
				rows := []identityRow{}
				for _, l := range b.IdentityLabels() {
					id := b.Identities[l]
					rows = append(rows, identityRow{l, id.Name, id.Email, id.Key})
				}
				if wantJSON(cmd) {
					return printJSON(cmd, rows)
				}
				var t [][]string
				for _, r := range rows {
					t = append(t, []string{r.Label, r.Name, r.Email, r.Key})
				}
				return printTable(cmd, t)
			})
		},
	})
}

func newIdentityShowCmd() *cobra.Command {
	return addJSONFlag(&cobra.Command{
		Use:   "show <identity>",
		Short: "Show an identity with resolved values",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return view(cmd, func(b *store.Book) error {
				r, err := b.Resolve(args[0])
				if err != nil {
					return err
				}
				d := identityDetail{r.Label, r.Name, r.Email, r.Key.Private, r.Key.Public, fingerprint(r.Key)}
				if wantJSON(cmd) {
					return printJSON(cmd, d)
				}
				out := cmd.OutOrStdout()
				fmt.Fprintf(out, "identity: %s\nname:     %s\nemail:    %s\nkey:      %s\n", d.Identity, d.Name, d.Email, d.Key)
				if d.Public != "" {
					fmt.Fprintf(out, "pubkey:   %s\n", d.Public)
					fmt.Fprintf(out, "fingerprint: %s\n", d.Fingerprint)
				}
				return nil
			})
		},
	})
}

func newIdentityRenameCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rename <identity> <new-label>",
		Short: "Rename an identity; repositories using it are updated",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := bookPath(cmd)
			if err != nil {
				return err
			}
			if err := modify(cmd, func(b *store.Book) error { return b.RenameIdentity(args[0], args[1]) }); err != nil {
				return err
			}
			updated, err := repo.Relabel(path, args[0], args[1])
			for _, dir := range updated {
				fmt.Fprintf(cmd.OutOrStdout(), "updated %s\n", dir)
			}
			return err
		},
	}
}
