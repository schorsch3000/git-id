package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	idrepo "git-id/internal/repo"
	"git-id/internal/store"
)

// press feeds keys to the model. Names such as "enter" or "tab" are special
// keys; anything else is typed as text.
func press(t *testing.T, m *Model, keys ...string) {
	t.Helper()
	special := map[string]tea.KeyType{
		"enter": tea.KeyEnter, "tab": tea.KeyTab, "shift+tab": tea.KeyShiftTab, "esc": tea.KeyEsc,
		"up": tea.KeyUp, "down": tea.KeyDown, "left": tea.KeyLeft, "right": tea.KeyRight,
		"ctrl+s": tea.KeyCtrlS, "ctrl+c": tea.KeyCtrlC,
	}
	for _, k := range keys {
		if kt, ok := special[k]; ok {
			m.Update(tea.KeyMsg{Type: kt})
			continue
		}
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)})
	}
}

// isolate gives git a throwaway HOME so the real config is never touched.
func isolate(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, ".gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
}

func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func newRepo(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "init", "-q")
	return dir
}

// fixture creates a book with identity "work" and a model whose repository
// directory is dir.
func fixture(t *testing.T, dir string) (*Model, string) {
	t.Helper()
	isolate(t)
	d := t.TempDir()
	keyFile := filepath.Join(d, "id_work")
	if err := os.WriteFile(keyFile, []byte("k"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(d, "book.toml")
	b := store.New()
	for _, err := range []error{
		b.AddName("me", "Dirk Heilig"),
		b.AddEmail("work", "dirk@example.com"),
		b.AddEmail("priv", "dirk@example.org"),
		b.AddKey("k1", store.Key{Private: keyFile}),
		b.AddIdentity("work", store.Identity{Name: "me", Email: "work", Key: "k1"}),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Save(path, b); err != nil {
		t.Fatal(err)
	}
	m, err := New(path, dir)
	if err != nil {
		t.Fatal(err)
	}
	return m, keyFile
}

func reload(t *testing.T, m *Model) *store.Book {
	t.Helper()
	b, err := store.Load(m.path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestViewShowsTabsItemsAndRepoState(t *testing.T) {
	repo := newRepo(t)
	m, _ := fixture(t, repo)
	v := m.View()
	for _, want := range []string{"Identities", "Names", "Emails", "Keys", "work", "Dirk Heilig <dirk@example.com>", "has no identity"} {
		if !strings.Contains(v, want) {
			t.Errorf("view lacks %q:\n%s", want, v)
		}
	}

	m2, _ := fixture(t, t.TempDir())
	if v := m2.View(); !strings.Contains(v, "not in a git repository") {
		t.Errorf("view outside repo:\n%s", v)
	}
}

func TestNavigationClampsAndWraps(t *testing.T) {
	m, _ := fixture(t, newRepo(t))
	press(t, m, "left")
	if m.tab != tabKeys {
		t.Errorf("left from first tab = %v, want wrap to keys", m.tab)
	}
	press(t, m, "right")
	if m.tab != tabIdentities {
		t.Errorf("right from last tab = %v, want wrap", m.tab)
	}
	press(t, m, "l") // names
	press(t, m, "down", "down", "down")
	if m.cursor[tabNames] != 0 {
		t.Errorf("cursor = %d, want clamped to 0 (one name)", m.cursor[tabNames])
	}
	press(t, m, "l", "down", "down", "down", "down") // emails: work, priv
	if m.cursor[tabEmails] != 1 {
		t.Errorf("cursor = %d, want 1", m.cursor[tabEmails])
	}
	press(t, m, "up", "up", "up")
	if m.cursor[tabEmails] != 0 {
		t.Errorf("cursor = %d, want 0", m.cursor[tabEmails])
	}
}

func TestQuit(t *testing.T) {
	for _, k := range []string{"q", "ctrl+c"} {
		m, _ := fixture(t, newRepo(t))
		var cmd tea.Cmd
		if k == "q" {
			_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
		} else {
			_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
		}
		if cmd == nil {
			t.Fatalf("%s: no command", k)
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Errorf("%s did not quit", k)
		}
	}
}

func TestAddNameEmailKeyAndIdentity(t *testing.T) {
	m, keyFile := fixture(t, newRepo(t))

	press(t, m, "l", "a", "nm2", "tab", "Other Name", "enter")     // names
	press(t, m, "l", "a", "em3", "tab", "x@y.z", "ctrl+s")         // emails: ctrl+s from first field
	press(t, m, "l", "a", "ky2", "tab", keyFile, "enter", "enter") // keys: private, then (empty) public

	b := reload(t, m)
	if b.Names["nm2"] != "Other Name" || b.Emails["em3"] != "x@y.z" || b.Keys["ky2"].Private != keyFile {
		t.Fatalf("book after adds: %+v", b)
	}
	if m.mode != modeList {
		t.Fatalf("mode = %v, want list", m.mode)
	}

	// Identity: labels sorted, so emails are [em3 priv work]; pick em3 (first) and key ky2 (second).
	press(t, m, "l", "a", "id2") // back on identities, label typed
	press(t, m, "tab", "right")  // name: me -> nm2 (names: me, nm2)
	press(t, m, "tab")           // email stays em3
	press(t, m, "tab", "right")  // key: k1 -> ky2
	press(t, m, "enter")
	r, err := reload(t, m).Resolve("id2")
	if err != nil {
		t.Fatal(err)
	}
	if r.Name != "Other Name" || r.Email != "x@y.z" || r.Key.Private != keyFile {
		t.Errorf("identity id2 = %+v", r)
	}
	if !strings.Contains(m.View(), "added identity id2") {
		t.Errorf("no success status:\n%s", m.View())
	}
}

func TestAddIdentityNeedsDatapoints(t *testing.T) {
	isolate(t)
	path := filepath.Join(t.TempDir(), "book.toml")
	m, err := New(path, "")
	if err != nil {
		t.Fatal(err)
	}
	press(t, m, "a")
	if m.mode != modeList || !m.statusErr {
		t.Errorf("mode %v, status %q (err=%v)", m.mode, m.status, m.statusErr)
	}
	if !strings.Contains(m.View(), "add at least one name, email and key first") {
		t.Errorf("view:\n%s", m.View())
	}
	if !strings.Contains(m.View(), "no identitys") && !strings.Contains(m.View(), "press a to add") {
		t.Errorf("empty hint missing:\n%s", m.View())
	}
}

func TestFormErrorsStayInForm(t *testing.T) {
	m, _ := fixture(t, newRepo(t))
	press(t, m, "l", "a", "me", "tab", "dup", "enter") // label "me" exists
	if m.mode != modeForm || m.form.err == "" {
		t.Fatalf("mode %v err %q, want form with error", m.mode, m.form.err)
	}
	if !strings.Contains(m.View(), "already exists") {
		t.Errorf("error not shown:\n%s", m.View())
	}
	press(t, m, "esc")
	if m.mode != modeList {
		t.Errorf("esc did not leave the form")
	}
	if got := reload(t, m).Names["me"]; got != "Dirk Heilig" {
		t.Errorf("name changed to %q", got)
	}

	// Missing key file in the key form.
	press(t, m, "l", "l", "a", "ky9", "tab", "/does/not/exist", "enter", "enter")
	if m.mode != modeForm || !strings.Contains(m.form.err, "key file") {
		t.Errorf("key form: mode %v err %q", m.mode, m.form.err)
	}
}

func TestEdit(t *testing.T) {
	m, _ := fixture(t, newRepo(t))

	// Edit identity work: switch email to priv (emails: priv, work -> work is index 1, left wraps to priv).
	press(t, m, "e", "tab", "left", "enter") // name field focused first; tab -> email; left; enter moves to key
	press(t, m, "enter")                     // key (last) -> save
	if got := reload(t, m).Identities["work"].Email; got != "priv" {
		t.Errorf("identity email = %q, want priv", got)
	}

	// Edit name value: field is preloaded, clear it with backspaces and retype.
	press(t, m, "l", "e")
	for range "Dirk Heilig" {
		m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	}
	press(t, m, "D. Heilig", "enter")
	if got := reload(t, m).Names["me"]; got != "D. Heilig" {
		t.Errorf("name = %q", got)
	}
}

func TestDelete(t *testing.T) {
	m, _ := fixture(t, newRepo(t))

	// Cancel keeps everything.
	press(t, m, "d")
	if m.mode != modeConfirm || !strings.Contains(m.View(), `Delete identity "work"`) {
		t.Fatalf("no confirmation:\n%s", m.View())
	}
	press(t, m, "n")
	if _, ok := reload(t, m).Identities["work"]; !ok || m.mode != modeList {
		t.Fatal("cancel deleted the identity")
	}

	// Datapoint in use: warns, then removes identity too.
	press(t, m, "l", "l", "down", "d") // emails: priv, work -> cursor on work
	if !strings.Contains(m.View(), "used by identities work") {
		t.Fatalf("no in-use warning:\n%s", m.View())
	}
	press(t, m, "y")
	b := reload(t, m)
	if _, ok := b.Emails["work"]; ok {
		t.Error("email work not deleted")
	}
	if _, ok := b.Identities["work"]; ok {
		t.Error("dependent identity not deleted")
	}

	// Unused datapoint: plain confirmation.
	press(t, m, "up", "d")
	if strings.Contains(m.View(), "used by") {
		t.Errorf("unexpected in-use warning:\n%s", m.View())
	}
	press(t, m, "y")
	if _, ok := reload(t, m).Emails["priv"]; ok {
		t.Error("email priv not deleted")
	}
	if m.cursor[tabEmails] != 0 {
		t.Errorf("cursor = %d after deleting last item", m.cursor[tabEmails])
	}
	press(t, m, "d") // nothing left: no-op
	if m.mode != modeList {
		t.Error("delete on empty list opened a dialog")
	}
}

func TestUseAppliesIdentityToRepo(t *testing.T) {
	repo := newRepo(t)
	m, _ := fixture(t, repo)
	press(t, m, "enter")
	for k, want := range map[string]string{
		"user.name":       "Dirk Heilig",
		"user.email":      "dirk@example.com",
		"id.identity":     "work",
		"core.sshCommand": idrepo.SSHCommand(m.path, "work"),
	} {
		if got := gitRun(t, repo, "config", "--local", k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
	v := m.View()
	if !strings.Contains(v, "this repository uses work") || !strings.Contains(v, "* work") {
		t.Errorf("view does not show the active identity:\n%s", v)
	}
	repos, _ := store.LoadRepos(store.RegistryPath(m.path))
	if len(repos) != 1 || repos[0] != repo {
		t.Errorf("registry = %v", repos)
	}
}

func TestUseFailures(t *testing.T) {
	m, _ := fixture(t, t.TempDir()) // not a repo
	press(t, m, "enter")
	if !m.statusErr || !strings.Contains(m.status, "not inside a git repository") {
		t.Errorf("status %q", m.status)
	}

	repo := newRepo(t)
	m, _ = fixture(t, repo)
	gitRun(t, repo, "config", "core.sshCommand", "ssh -v")
	press(t, m, "enter")
	if !m.statusErr || !strings.Contains(m.status, "core.sshCommand") {
		t.Errorf("status %q", m.status)
	}
	if got := gitRun(t, repo, "config", "core.sshCommand"); got != "ssh -v" {
		t.Errorf("foreign core.sshCommand overwritten: %q", got)
	}

	// enter on other tabs does nothing.
	press(t, m, "l", "enter")
	if m.status != "" {
		t.Errorf("status %q on names tab", m.status)
	}
}

func TestCommitKeepsExternalChanges(t *testing.T) {
	m, _ := fixture(t, newRepo(t))
	// The CLI edits the book while the TUI is open.
	b := reload(t, m)
	if err := b.AddName("external", "Added Elsewhere"); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(m.path, b); err != nil {
		t.Fatal(err)
	}
	press(t, m, "l", "a", "mine", "tab", "Mine", "enter")
	got := reload(t, m)
	if got.Names["external"] != "Added Elsewhere" || got.Names["mine"] != "Mine" {
		t.Errorf("names = %v", got.Names)
	}
	if !strings.Contains(m.View(), "external") {
		t.Errorf("view not refreshed:\n%s", m.View())
	}
}

func TestBrokenReferenceIsShown(t *testing.T) {
	m, _ := fixture(t, newRepo(t))
	// Cannot happen through Save (it validates), but the view must not crash
	// or hide an identity whose references went missing.
	b := reload(t, m)
	b.Identities["bad"] = store.Identity{Name: "gone", Email: "work", Key: "k1"}
	m.book = b
	if !strings.Contains(m.View(), "(broken reference)") {
		t.Errorf("view:\n%s", m.View())
	}
}

func TestUseWarnsWhenGitSSHCommandOverrides(t *testing.T) {
	m, _ := fixture(t, newRepo(t))
	t.Setenv("GIT_SSH_COMMAND", "ssh -v")
	press(t, m, "enter")
	if m.statusErr || !strings.Contains(m.status, "using identity work") || !strings.Contains(m.status, "GIT_SSH_COMMAND") {
		t.Errorf("status %q (err=%v)", m.status, m.statusErr)
	}
}
