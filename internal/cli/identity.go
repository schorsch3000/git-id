package cli

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

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
	return &cobra.Command{
		Use:   "ls",
		Short: "List identities",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return view(cmd, func(b *store.Book) error {
				tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
				for _, l := range b.IdentityLabels() {
					id := b.Identities[l]
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", l, id.Name, id.Email, id.Key)
				}
				return tw.Flush()
			})
		},
	}
}

func newIdentityShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <identity>",
		Short: "Show an identity with resolved values",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return view(cmd, func(b *store.Book) error {
				r, err := b.Resolve(args[0])
				if err != nil {
					return err
				}
				out := cmd.OutOrStdout()
				fmt.Fprintf(out, "identity: %s\nname:     %s\nemail:    %s\nkey:      %s\n", r.Label, r.Name, r.Email, r.Key.Private)
				if r.Key.Public != "" {
					fmt.Fprintf(out, "pubkey:   %s\n", r.Key.Public)
				}
				return nil
			})
		},
	}
}
