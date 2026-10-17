package cli

import (
	"encoding/json"
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"
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

// Results of the commands that act, printed by their --json flag.

type useResult struct {
	Identity   string `json:"identity"`
	Repository string `json:"repository"`
}

type currentResult struct {
	Identity   string `json:"identity"`
	Name       string `json:"name"`
	Email      string `json:"email"`
	Repository string `json:"repository"`
}

type unuseResult struct {
	Repository            string `json:"repository"`
	KeptForeignSSHCommand bool   `json:"kept_foreign_ssh_command"`
}

type syncRepo struct {
	Path     string `json:"path"`
	Identity string `json:"identity,omitempty"`
	Status   string `json:"status"` // synced, dropped, failed
	Error    string `json:"error,omitempty"`
}

type syncResult struct {
	Repositories []syncRepo `json:"repositories"`
}

type initResult struct {
	Label           string `json:"label"`
	Name            string `json:"name,omitempty"`
	Email           string `json:"email,omitempty"`
	Key             string `json:"key,omitempty"`
	IdentityCreated bool   `json:"identity_created"`
	RemovedGlobal   bool   `json:"removed_global_name_and_email"`
	UseConfigOnly   bool   `json:"user_use_config_only"`
}

type renameResult struct {
	From    string   `json:"from"`
	To      string   `json:"to"`
	Updated []string `json:"updated_repositories"`
}

type removedResult struct {
	RemovedIdentities []string `json:"removed_identities"`
}
