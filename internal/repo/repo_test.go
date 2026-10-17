package repo

import (
	"os/exec"
	"strings"
	"testing"
)

func TestSSHCommandPinsNonDefaultBook(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/xdg")
	if got := SSHCommand("/xdg/git-id/book.toml", "work"); got != "git id ssh work" {
		t.Errorf("default book: %q", got)
	}
	got := SSHCommand("/tmp/my book/it's.toml", "work")
	want := `GIT_ID_BOOK='/tmp/my book/it'\''s.toml' git id ssh work`
	if got != want {
		t.Errorf("custom book: %q, want %q", got, want)
	}
}

func TestIsOurSSHCommand(t *testing.T) {
	ours := []string{
		"git id ssh work",
		"git id ssh a.b_c-1",
		`GIT_ID_BOOK='/tmp/x.toml' git id ssh work`,
		`GIT_ID_BOOK='/tmp/it'\''s.toml' git id ssh work`,
		SSHCommand("/some/where/book.toml", "id1"),
	}
	for _, c := range ours {
		if !IsOurSSHCommand(c) {
			t.Errorf("%q should be ours", c)
		}
	}
	foreign := []string{
		"ssh -v", "git id ssh", "git id ssh work; rm -rf /", "git id ssh work extra",
		"GIT_ID_BOOK=/tmp/x git id ssh work", "FOO='x' git id ssh work", "",
	}
	for _, c := range foreign {
		if IsOurSSHCommand(c) {
			t.Errorf("%q should be foreign", c)
		}
	}
}

func TestSSHCommandRoundTripsThroughAShell(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh on this system")
	}
	// The pinned path must survive a real shell with awkward characters.
	book := "/tmp/we ird/$HOME/it's \"q\".toml"
	cmd := strings.Replace(SSHCommand(book, "work"), "git id ssh work", `sh -c 'printf %s "$GIT_ID_BOOK"'`, 1)
	out, err := shOutput(cmd)
	if err != nil {
		t.Fatal(err)
	}
	if out != book {
		t.Errorf("shell saw %q, want %q", out, book)
	}
}

func shOutput(cmd string) (string, error) {
	out, err := exec.Command("sh", "-c", cmd).Output()
	return string(out), err
}
