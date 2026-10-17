package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh"

	"git-id/internal/store"
)

// addJSONFlag gives a listing command a --json flag.
func addJSONFlag(c *cobra.Command) *cobra.Command {
	c.Flags().Bool("json", false, "print machine-readable JSON")
	return c
}

func wantJSON(cmd *cobra.Command) bool {
	j, _ := cmd.Flags().GetBool("json")
	return j
}

// printJSON writes v indented. Callers pass non-nil slices so empty lists
// print as [] and not null.
func printJSON(cmd *cobra.Command, v any) error {
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// printTable writes rows as aligned columns.
func printTable(cmd *cobra.Command, rows [][]string) error {
	tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
	for _, r := range rows {
		for i, c := range r {
			if i > 0 {
				fmt.Fprint(tw, "\t")
			}
			fmt.Fprint(tw, c)
		}
		fmt.Fprintln(tw)
	}
	return tw.Flush()
}

type valueRow struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type keyRow struct {
	Label       string `json:"label"`
	Private     string `json:"private"`
	Public      string `json:"public,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

type identityRow struct {
	Label string `json:"label"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Key   string `json:"key"`
}

type identityDetail struct {
	Identity    string `json:"identity"`
	Name        string `json:"name"`
	Email       string `json:"email"`
	Key         string `json:"key"`
	Public      string `json:"public,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

// fingerprint returns the SHA256 fingerprint of the public key file of k, "" if
// there is none and "(unreadable)" if the file cannot be parsed as a public key.
func fingerprint(k store.Key) string {
	if k.Public == "" {
		return ""
	}
	data, err := os.ReadFile(k.Public)
	if err != nil {
		return "(unreadable)"
	}
	pub, _, _, _, err := ssh.ParseAuthorizedKey(data)
	if err != nil {
		return "(unreadable)"
	}
	return ssh.FingerprintSHA256(pub)
}
