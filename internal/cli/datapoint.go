package cli

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"git-id/internal/store"
)

// valueKind describes a plain string datapoint (name or email).
type valueKind struct {
	kind   store.Kind
	values func(*store.Book) map[string]string
	add    func(*store.Book, string, string) error
	set    func(*store.Book, string, string) error
}

func newValueCmd(vk valueKind) *cobra.Command {
	k := string(vk.kind)
	root := &cobra.Command{
		Use:   k,
		Short: fmt.Sprintf("Manage %s datapoints", k),
	}
	root.AddCommand(
		&cobra.Command{
			Use:   "add <label> <value>",
			Short: fmt.Sprintf("Add a %s", k),
			Args:  cobra.ExactArgs(2),
			RunE: func(cmd *cobra.Command, args []string) error {
				return modify(cmd, func(b *store.Book) error { return vk.add(b, args[0], args[1]) })
			},
		},
		&cobra.Command{
			Use:   "edit <label> <value>",
			Short: fmt.Sprintf("Change a %s; all identities using it follow", k),
			Args:  cobra.ExactArgs(2),
			RunE: func(cmd *cobra.Command, args []string) error {
				return modify(cmd, func(b *store.Book) error { return vk.set(b, args[0], args[1]) })
			},
		},
		newRenameCmd(vk.kind),
		newRmCmd(vk.kind),
		&cobra.Command{
			Use:   "ls",
			Short: fmt.Sprintf("List %ss", k),
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				return view(cmd, func(b *store.Book) error {
					tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
					vals := vk.values(b)
					for _, l := range b.Labels(vk.kind) {
						fmt.Fprintf(tw, "%s\t%s\n", l, vals[l])
					}
					return tw.Flush()
				})
			},
		},
	)
	return root
}

func newRmCmd(kind store.Kind) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "rm <label>",
		Short: fmt.Sprintf("Remove a %s", kind),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return modify(cmd, func(b *store.Book) error {
				gone, err := b.Remove(kind, args[0], force)
				for _, id := range gone {
					fmt.Fprintf(cmd.OutOrStdout(), "removed identity %s\n", id)
				}
				return err
			})
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "also remove identities that use it")
	return cmd
}

func newNameCmd() *cobra.Command {
	return newValueCmd(valueKind{
		kind:   store.KindName,
		values: func(b *store.Book) map[string]string { return b.Names },
		add:    (*store.Book).AddName,
		set:    (*store.Book).SetName,
	})
}

func newEmailCmd() *cobra.Command {
	return newValueCmd(valueKind{
		kind:   store.KindEmail,
		values: func(b *store.Book) map[string]string { return b.Emails },
		add:    (*store.Book).AddEmail,
		set:    (*store.Book).SetEmail,
	})
}

func newKeyCmd() *cobra.Command {
	root := &cobra.Command{Use: "key", Short: "Manage ssh key datapoints"}

	var addPub string
	add := &cobra.Command{
		Use:   "add <label> <private-key-path>",
		Short: "Add an ssh key",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			k, err := buildKey(args[1], addPub, cmd.Flags().Changed("public"))
			if err != nil {
				return err
			}
			return modify(cmd, func(b *store.Book) error { return b.AddKey(args[0], k) })
		},
	}
	add.Flags().StringVar(&addPub, "public", "", "public key path")

	var editPriv, editPub string
	edit := &cobra.Command{
		Use:   "edit <label>",
		Short: "Change an ssh key; all identities using it follow",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			f := cmd.Flags()
			if !f.Changed("private") && !f.Changed("public") {
				return fmt.Errorf("nothing to change: give --private and/or --public")
			}
			return modify(cmd, func(b *store.Book) error {
				k, ok := b.Keys[args[0]]
				if !ok {
					return fmt.Errorf("key %q: %w", args[0], store.ErrNotFound)
				}
				if f.Changed("private") {
					p, err := store.ResolveKeyPath(editPriv)
					if err != nil {
						return err
					}
					k.Private = p
				}
				if f.Changed("public") {
					if editPub == "" {
						k.Public = ""
					} else {
						p, err := store.ResolveKeyPath(editPub)
						if err != nil {
							return err
						}
						k.Public = p
					}
				}
				return b.SetKey(args[0], k)
			})
		},
	}
	edit.Flags().StringVar(&editPriv, "private", "", "private key path")
	edit.Flags().StringVar(&editPub, "public", "", "public key path (empty clears)")

	ls := &cobra.Command{
		Use:   "ls",
		Short: "List ssh keys",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return view(cmd, func(b *store.Book) error {
				tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
				for _, l := range b.Labels(store.KindKey) {
					k := b.Keys[l]
					fmt.Fprintf(tw, "%s\t%s\t%s\n", l, k.Private, k.Public)
				}
				return tw.Flush()
			})
		},
	}

	root.AddCommand(add, edit, newRenameCmd(store.KindKey), newRmCmd(store.KindKey), ls)
	return root
}

func buildKey(private, public string, hasPublic bool) (store.Key, error) {
	priv, err := store.ResolveKeyPath(private)
	if err != nil {
		return store.Key{}, err
	}
	k := store.Key{Private: priv}
	if hasPublic && public != "" {
		if k.Public, err = store.ResolveKeyPath(public); err != nil {
			return store.Key{}, err
		}
	}
	return k, nil
}

func newRenameCmd(kind store.Kind) *cobra.Command {
	return &cobra.Command{
		Use:   "rename <label> <new-label>",
		Short: fmt.Sprintf("Rename a %s; identities using it follow", kind),
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return modify(cmd, func(b *store.Book) error { return b.RenameDatapoint(kind, args[0], args[1]) })
		},
	}
}
