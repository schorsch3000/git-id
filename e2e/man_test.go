package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"git-id/internal/cli"
)

// TestManPages checks the generated pages and that `git id --help` asks the
// man viewer for exactly the page that `make install` installs. A recording
// viewer stands in for man, so no man installation is needed.
func TestManPages(t *testing.T) {
	e := newEnv(t)
	pages := filepath.Join(e.home, "man")
	// The real environment: go needs its own caches, not the isolated HOME.
	gen := exec.Command("go", "run", "./tools/genman", pages)
	gen.Dir = ".."
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("genman: %v\n%s", err, out)
	}

	top, err := os.ReadFile(filepath.Join(pages, "git-id.1"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{".TH \"GIT-ID\" \"1\"", "address book of names", "git id use work", "SEE ALSO"} {
		if !strings.Contains(string(top), want) {
			t.Errorf("git-id.1 lacks %q", want)
		}
	}

	// Every visible command has a page; hidden ones (ssh) do not.
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		name := strings.ReplaceAll(c.CommandPath(), " ", "-") + ".1"
		_, err := os.Stat(filepath.Join(pages, name))
		switch {
		case c.Hidden && err == nil:
			t.Errorf("hidden command %q has a page", c.CommandPath())
		case !c.Hidden && c.Name() != "help" && err != nil:
			t.Errorf("no page for %q (%s)", c.CommandPath(), name)
		}
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(cli.NewRootCmd())

	// `git id --help` makes git run the viewer with the page name git-id.
	rec := filepath.Join(e.home, "asked")
	viewer := filepath.Join(e.home, "viewer")
	if err := os.WriteFile(viewer, []byte("#!/bin/sh\necho \"$@\" > "+rec+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	e.must(t, e.home, "git", "-c", "man.viewer=probe", "-c", "man.probe.cmd="+viewer, "id", "--help")
	asked, err := os.ReadFile(rec)
	if err != nil {
		t.Fatalf("viewer was not run: %v", err)
	}
	if page := strings.TrimSpace(string(asked)); page+".1" != "git-id.1" {
		t.Errorf("git asked for page %q, installed page is git-id.1", page)
	}
}
