package cli

import (
	"os"

	"github.com/spf13/cobra"

	"git-id/internal/store"
)

const bookEnv = "GIT_ID_BOOK"

// bookPath resolves the address book location: --book flag, then $GIT_ID_BOOK,
// then the XDG default.
func bookPath(cmd *cobra.Command) (string, error) {
	if p, _ := cmd.Flags().GetString("book"); p != "" {
		return p, nil
	}
	if p := os.Getenv(bookEnv); p != "" {
		return p, nil
	}
	return store.DefaultPath()
}

// modify loads the book, applies fn and saves it if fn succeeds.
func modify(cmd *cobra.Command, fn func(*store.Book) error) error {
	path, err := bookPath(cmd)
	if err != nil {
		return err
	}
	b, err := store.Load(path)
	if err != nil {
		return err
	}
	if err := fn(b); err != nil {
		return err
	}
	return store.Save(path, b)
}

// view loads the book read-only.
func view(cmd *cobra.Command, fn func(*store.Book) error) error {
	path, err := bookPath(cmd)
	if err != nil {
		return err
	}
	b, err := store.Load(path)
	if err != nil {
		return err
	}
	return fn(b)
}
