package tui

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"golang.org/x/crypto/ssh"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
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
	t.Setenv("USERPROFILE", home) // Windows
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
	if !strings.Contains(m.View(), "(no identities, press a to add one)") {
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
	if m.statusErr || !strings.Contains(m.status, "using identity work") || strings.Contains(m.status, "GIT_SSH_COMMAND") {
		t.Errorf("status %q (err=%v)", m.status, m.statusErr)
	}
	if !strings.Contains(m.warn, "GIT_SSH_COMMAND is set") || !strings.Contains(m.View(), m.warn) {
		t.Errorf("warning %q not shown:\n%s", m.warn, m.View())
	}
	press(t, m, "down") // any further key clears it
	if m.warn != "" {
		t.Errorf("warning survived the next key: %q", m.warn)
	}
}

func TestSSHWarningIsYellowAndNotRed(t *testing.T) {
	m, _ := fixture(t, newRepo(t))
	t.Setenv("GIT_SSH_COMMAND", "ssh -v")
	withColor(t)
	press(t, m, "enter")
	v := m.View()
	if !strings.Contains(v, warnSty.Render(idrepo.SSHOverrideWarning)) {
		t.Errorf("warning is not highlighted yellow:\n%q", v)
	}
	if strings.Contains(v, errSty.Render(idrepo.SSHOverrideWarning)) {
		t.Errorf("warning is red:\n%q", v)
	}
	if !strings.Contains(v, okSty.Render("using identity work in")) && !strings.Contains(ansiSeq.ReplaceAllString(v, ""), "using identity work in") {
		t.Errorf("success status missing:\n%q", v)
	}
}

// sshDirFixture makes a directory shaped like ~/.ssh: two private keys (one
// with a real public key), and files that are not private keys.
func sshDirFixture(t *testing.T) (dir, ed, edPub, fingerprint string) {
	t.Helper()
	dir = t.TempDir()
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKey(privKey, "")
	if err != nil {
		t.Fatal(err)
	}
	sp, err := ssh.NewPublicKey(pubKey)
	if err != nil {
		t.Fatal(err)
	}
	ed = filepath.Join(dir, "id_ed25519")
	edPub = ed + ".pub"
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("id_ed25519", string(pem.EncodeToMemory(block)))
	write("id_ed25519.pub", string(ssh.MarshalAuthorizedKey(sp)))
	write("id_rsa", "-----BEGIN RSA PRIVATE KEY-----\nMIIB...\n-----END RSA PRIVATE KEY-----\n")
	write("lonely.pub", "ssh-ed25519 AAAA... only a public key\n")
	write("known_hosts", "github.com ssh-ed25519 AAAA...\n")
	write("config", "Host *\n  IdentitiesOnly yes\n")
	write("notes.txt", "-----BEGIN something else-----\n")
	// A passphrase protected key, one a level down, one too deep to be found.
	enc, err := ssh.MarshalPrivateKeyWithPassphrase(privKey, "", []byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	write("id_secret", string(pem.EncodeToMemory(enc)))
	for _, sub := range []string{"sub", filepath.Join("deep", "er", "est")} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	write("sub/id_work", string(pem.EncodeToMemory(block)))
	write("deep/er/est/id_toodeep", string(pem.EncodeToMemory(block)))
	write("sub/notes.txt", "not a key\n")
	return dir, ed, edPub, ssh.FingerprintSHA256(sp)
}

func TestDiscoverKeysFindsOnlyPrivateKeys(t *testing.T) {
	dir, ed, edPub, fp := sshDirFixture(t)
	keys, err := discoverKeys(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, k := range keys {
		names = append(names, k.Name)
	}
	want := []string{"id_ed25519", "id_rsa", "id_secret", "sub/id_work"} // not deep/er/est/id_toodeep
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("keys = %v, want %v", names, want)
	}
	for _, k := range keys {
		if k.Encrypted != (k.Name == "id_secret") {
			t.Errorf("%s: Encrypted = %v", k.Name, k.Encrypted)
		}
	}
	if keys[3].Path != filepath.Join(dir, "sub", "id_work") {
		t.Errorf("path of the nested key = %q", keys[3].Path)
	}
	if keys[0].Path != ed || keys[0].Public != edPub || keys[0].Fingerprint != fp {
		t.Errorf("id_ed25519 = %+v, want public %s fingerprint %s", keys[0], edPub, fp)
	}
	if keys[1].Public != "" || keys[1].Fingerprint != "" {
		t.Errorf("id_rsa has no .pub but got %+v", keys[1])
	}
	if _, err := discoverKeys(filepath.Join(dir, "missing")); err == nil {
		t.Error("missing directory gave no error")
	}
}

func TestSuggestLabel(t *testing.T) {
	for in, want := range map[string]string{
		"id_ed25519": "ed25519", "id_work": "work", "deploy key": "deploy-key", "sub/id_work": "work", "a/b/c": "c",
		"github.pem": "github.pem", "id_": "", "---": "", "": "",
	} {
		if got := suggestLabel(in); got != want {
			t.Errorf("suggestLabel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPickKeyFillsTheForm(t *testing.T) {
	m, _ := fixture(t, newRepo(t))
	dir, ed, edPub, fp := sshDirFixture(t)
	m.sshDir = dir

	press(t, m, "l", "l", "l", "a") // keys tab, add form
	if !strings.Contains(m.View(), "ctrl+o pick a key from ~/.ssh") {
		t.Errorf("form does not advertise the picker:\n%s", m.View())
	}
	press(t, m, "ctrl+o")
	if m.mode != modePicker {
		t.Fatalf("mode = %v, want picker", m.mode)
	}
	v := m.View()
	for _, want := range []string{dir, "id_ed25519", fp, "id_rsa", "(no .pub file)", "id_secret", "(passphrase)", "sub/id_work"} {
		if !strings.Contains(v, want) {
			t.Errorf("picker lacks %q:\n%s", want, v)
		}
	}
	for _, unwanted := range []string{"lonely", "known_hosts", "config", "notes.txt", "toodeep"} {
		if strings.Contains(v, unwanted) {
			t.Errorf("picker lists %q:\n%s", unwanted, v)
		}
	}

	press(t, m, "enter") // first key
	if m.mode != modeForm {
		t.Fatalf("mode = %v, want form", m.mode)
	}
	if got := m.form.get("private"); got != ed {
		t.Errorf("private = %q, want %q", got, ed)
	}
	if got := m.form.get("public"); got != edPub {
		t.Errorf("public = %q, want %q", got, edPub)
	}
	if got := m.form.get("label"); got != "ed25519" {
		t.Errorf("suggested label = %q", got)
	}

	press(t, m, "ctrl+s")
	k := reload(t, m).Keys["ed25519"]
	if k.Private != ed || k.Public != edPub {
		t.Errorf("saved key = %+v", k)
	}
	if !strings.Contains(m.View(), "added key ed25519") {
		t.Errorf("no success status:\n%s", m.View())
	}
}

func TestPickKeyChoosesWithCursorAndKeepsLabel(t *testing.T) {
	m, _ := fixture(t, newRepo(t))
	dir, _, _, _ := sshDirFixture(t)
	m.sshDir = dir
	press(t, m, "l", "l", "l", "a", "mine", "ctrl+o", "down", "enter")
	if got := m.form.get("private"); got != filepath.Join(dir, "id_rsa") {
		t.Errorf("private = %q", got)
	}
	if got := m.form.get("public"); got != "" {
		t.Errorf("public = %q, want empty for a key without .pub", got)
	}
	if got := m.form.get("label"); got != "mine" {
		t.Errorf("an already typed label was replaced: %q", got)
	}
}

func TestPickKeyDoesNotSuggestATakenLabel(t *testing.T) {
	m, _ := fixture(t, newRepo(t)) // the book already has a key labelled k1
	dir, _, _, _ := sshDirFixture(t)
	// Sorted: id_ed25519, id_k1, id_rsa. The suggestion for id_k1 is "k1", taken.
	if err := os.WriteFile(filepath.Join(dir, "id_k1"), []byte("-----BEGIN OPENSSH PRIVATE KEY-----\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m.sshDir = dir
	press(t, m, "l", "l", "l", "a", "ctrl+o", "down", "enter")
	if got := m.form.get("private"); got != filepath.Join(dir, "id_k1") {
		t.Fatalf("private = %q", got)
	}
	if got := m.form.get("label"); got != "" {
		t.Errorf("label = %q, want it left empty because k1 is taken", got)
	}
}

func TestPickerEscapeEmptyAndMissingDirectory(t *testing.T) {
	m, _ := fixture(t, newRepo(t))
	m.sshDir = t.TempDir() // empty
	press(t, m, "l", "l", "l", "a", "ctrl+o")
	if !strings.Contains(m.View(), "no private keys found") {
		t.Errorf("empty directory:\n%s", m.View())
	}
	press(t, m, "enter") // nothing to choose: stays
	if m.mode != modePicker {
		t.Error("enter on an empty picker left it")
	}
	press(t, m, "esc")
	if m.mode != modeForm || m.form.get("private") != "" {
		t.Errorf("esc: mode %v private %q", m.mode, m.form.get("private"))
	}

	m.sshDir = filepath.Join(t.TempDir(), "nope")
	press(t, m, "ctrl+o")
	if m.picker.err == "" || !strings.Contains(m.View(), m.picker.err) {
		t.Errorf("missing directory not reported:\n%s", m.View())
	}
}

func TestPickerOnlyOnKeyForms(t *testing.T) {
	m, _ := fixture(t, newRepo(t))
	press(t, m, "l", "a") // names add form
	press(t, m, "ctrl+o")
	if m.mode != modeForm {
		t.Errorf("ctrl+o opened the picker in a name form (mode %v)", m.mode)
	}
}

func TestPickKeyInEditFormAndMarksKnownKeys(t *testing.T) {
	m, keyFile := fixture(t, newRepo(t))
	dir, ed, edPub, _ := sshDirFixture(t)
	// The address book already knows id_rsa-like path: register one of the discovered keys.
	b := reload(t, m)
	if err := b.AddKey("known", store.Key{Private: ed}); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(m.path, b); err != nil {
		t.Fatal(err)
	}
	m.book = b
	m.sshDir = dir

	press(t, m, "l", "l", "l", "e") // edit the first key (k1)
	press(t, m, "ctrl+o")
	if !strings.Contains(m.View(), "[in address book: known]") {
		t.Errorf("known key not marked:\n%s", m.View())
	}
	press(t, m, "enter")
	if m.form.target != "k1" || m.form.get("private") != ed || m.form.get("public") != edPub {
		t.Errorf("edit form: target %q private %q public %q", m.form.target, m.form.get("private"), m.form.get("public"))
	}
	press(t, m, "ctrl+s")
	if got := reload(t, m).Keys["k1"]; got.Private != ed || got.Public != edPub {
		t.Errorf("k1 = %+v, want the picked key (was %s)", got, keyFile)
	}
}

var ansiSeq = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// withColor forces colour output (tests run without a terminal) and restores
// the previous profile afterwards.
func withColor(t *testing.T) {
	t.Helper()
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(old) })
}

func TestKeyHintsAreHighlighted(t *testing.T) {
	m, _ := fixture(t, newRepo(t))
	m.sshDir = t.TempDir()

	plain := func() string { return ansiSeq.ReplaceAllString(m.View(), "") }
	// Each step: the keys that must be coloured and the plain footer text.
	check := func(name string, keys []string, footer string) {
		t.Helper()
		v := m.View()
		for _, k := range keys {
			if !strings.Contains(v, keySty.Render(k)) {
				t.Errorf("%s: key %q is not highlighted:\n%q", name, k, v)
			}
		}
		if !strings.Contains(plain(), footer) {
			t.Errorf("%s: footer text changed, want %q in:\n%s", name, footer, plain())
		}
	}

	// Without colour the output is plain text: no escape sequences at all.
	if strings.Contains(m.View(), "\x1b[") {
		t.Error("escape sequences emitted although colours are off")
	}

	withColor(t)
	if keySty.Render("a") == "a" {
		t.Fatal("test setup: styles produce no colour")
	}
	check("identities", []string{"←/→", "↑/↓", "/", "enter", "a", "e", "d", "q"},
		"←/→ tab  ↑/↓ move  / filter  enter use here  a add  e edit  d delete  q quit")

	press(t, m, "l")
	check("names", []string{"←/→", "↑/↓", "/", "a", "e", "d", "q"}, "←/→ tab  ↑/↓ move  / filter  a add  e edit  d delete  q quit")

	press(t, m, "a")
	check("name form", []string{"tab/shift+tab", "←/→", "enter", "ctrl+s", "esc"},
		"tab/shift+tab field  ←/→ choose  enter next/save  ctrl+s save  esc cancel")
	press(t, m, "esc", "l", "l", "a")
	check("key form", []string{"ctrl+o", "enter", "ctrl+s", "esc"},
		"tab/shift+tab field  ctrl+o pick a key from ~/.ssh  enter next/save  ctrl+s save  esc cancel")
	press(t, m, "ctrl+o")
	check("picker", []string{"↑/↓", "enter", "esc"}, "↑/↓ move  enter choose  esc back to the form")
	press(t, m, "esc", "esc", "d")
	check("confirm footer", []string{"y"}, "y confirm  any other key cancels")
	if !strings.Contains(m.View(), "["+keySty.Render("y")+"/N]") {
		t.Errorf("the y of [y/N] is not highlighted:\n%q", m.View())
	}
	press(t, m, "n", "l", "l") // back on a tab; remove all keys to see the empty hint
	b := reload(t, m)
	if _, err := b.Remove("key", "k1", true); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(m.path, b); err != nil {
		t.Fatal(err)
	}
	m.book = b
	m.tab = tabKeys
	if v := m.View(); !strings.Contains(v, keySty.Render("a")) || !strings.Contains(plain(), "(no keys, press a to add one)") {
		t.Errorf("empty-list hint:\n%q", v)
	}
}

// clearAndType empties the focused text field and types s.
func clearAndType(t *testing.T, m *Model, current, s string) {
	t.Helper()
	for range current {
		m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	}
	press(t, m, s)
}

func TestRenameThroughTheEditForm(t *testing.T) {
	m, _ := fixture(t, newRepo(t))
	press(t, m, "l", "e", "up") // names tab, edit "me", focus the label field
	if m.form.get("label") != "me" || m.form.focus != 0 {
		t.Fatalf("label field: %q, focus %d", m.form.get("label"), m.form.focus)
	}
	clearAndType(t, m, "me", "dirk")
	press(t, m, "ctrl+s")
	b := reload(t, m)
	if _, old := b.Names["me"]; old || b.Names["dirk"] != "Dirk Heilig" {
		t.Errorf("names = %v", b.Names)
	}
	if got := b.Identities["work"].Name; got != "dirk" {
		t.Errorf("identity work still points at %q", got)
	}
	if !strings.Contains(m.status, "saved name dirk (renamed from me)") {
		t.Errorf("status %q", m.status)
	}
	if it, _ := m.current(); it.label != "dirk" {
		t.Errorf("cursor on %q after the rename", it.label)
	}
}

func TestRenamingAnIdentityUpdatesTheRepository(t *testing.T) {
	repo := newRepo(t)
	m, _ := fixture(t, repo)
	press(t, m, "enter") // use work here
	press(t, m, "e", "up")
	clearAndType(t, m, "work", "company")
	press(t, m, "ctrl+s")
	if got := gitRun(t, repo, "config", "--local", "id.identity"); got != "company" {
		t.Errorf("id.identity = %q", got)
	}
	if got := gitRun(t, repo, "config", "--local", "core.sshCommand"); got != idrepo.SSHCommand(m.path, "company") {
		t.Errorf("core.sshCommand = %q", got)
	}
	if m.repoLabel != "company" || !strings.Contains(m.View(), "this repository uses company") {
		t.Errorf("header not refreshed: %q\n%s", m.repoLabel, m.View())
	}
}

func TestRenameRefusals(t *testing.T) {
	m, _ := fixture(t, newRepo(t))
	press(t, m, "l", "l", "e", "up") // emails: priv, work -> cursor on priv
	clearAndType(t, m, "priv", "work")
	press(t, m, "ctrl+s")
	if m.mode != modeForm || !strings.Contains(m.form.err, "already exists") {
		t.Errorf("collision: mode %v err %q", m.mode, m.form.err)
	}
	clearAndType(t, m, "work", "bad label")
	press(t, m, "ctrl+s")
	if m.mode != modeForm || !strings.Contains(m.form.err, "invalid label") {
		t.Errorf("invalid label: mode %v err %q", m.mode, m.form.err)
	}
	clearAndType(t, m, "bad label", "")
	press(t, m, "ctrl+s")
	if m.mode != modeForm || m.form.err == "" {
		t.Errorf("empty label accepted: mode %v", m.mode)
	}
	if _, ok := reload(t, m).Emails["priv"]; !ok {
		t.Error("the email vanished")
	}
}

func TestFilter(t *testing.T) {
	m, _ := fixture(t, newRepo(t))
	press(t, m, "l", "l") // emails: priv (dirk@example.org), work (dirk@example.com)
	if !strings.Contains(m.View(), "/ filter") {
		t.Errorf("filter not advertised:\n%s", m.View())
	}

	press(t, m, "/", "PRI") // typing narrows live, case-insensitive
	if m.mode != modeFilter {
		t.Fatalf("mode %v", m.mode)
	}
	if v := m.View(); !strings.Contains(v, "priv") || strings.Contains(v, "example.com") {
		t.Errorf("filter on the label:\n%s", v)
	}
	press(t, m, "enter") // keep it
	if m.mode != modeList || m.filter != "PRI" {
		t.Fatalf("mode %v filter %q", m.mode, m.filter)
	}
	if v := m.View(); !strings.Contains(v, "filter: PRI") || !strings.Contains(v, "esc clear filter") {
		t.Errorf("active filter not shown:\n%s", v)
	}
	// Actions act on the filtered list: delete asks about priv, not work.
	press(t, m, "d")
	if !strings.Contains(m.View(), `Delete email "priv"`) {
		t.Errorf("delete does not follow the filter:\n%s", m.View())
	}
	press(t, m, "n")

	press(t, m, "esc") // clears it from the list
	if m.filter != "" || !strings.Contains(m.View(), "example.com") {
		t.Errorf("esc did not clear the filter: %q", m.filter)
	}

	press(t, m, "/", "example.com") // matches the value too
	if v := m.View(); !strings.Contains(v, "work") || strings.Contains(v, "example.org") {
		t.Errorf("filter on the value:\n%s", v)
	}
	press(t, m, "esc") // esc while typing clears and leaves
	if m.mode != modeList || m.filter != "" {
		t.Errorf("mode %v filter %q", m.mode, m.filter)
	}

	press(t, m, "/", "zzz", "enter")
	if !strings.Contains(m.View(), `(no emails match "zzz")`) {
		t.Errorf("empty result:\n%s", m.View())
	}
	press(t, m, "d", "e") // nothing to act on
	if m.mode != modeList {
		t.Errorf("an action opened a dialog on an empty result (mode %v)", m.mode)
	}
	press(t, m, "right") // switching tabs drops the filter
	if m.filter != "" {
		t.Errorf("filter survived a tab switch: %q", m.filter)
	}
}

func TestFilterKeepsTheCursorValid(t *testing.T) {
	m, _ := fixture(t, newRepo(t))
	press(t, m, "l", "l", "down") // second email selected
	press(t, m, "/", "priv")
	if it, ok := m.current(); !ok || it.label != "priv" {
		t.Errorf("current after filtering = %+v, %v", it, ok)
	}
	press(t, m, "enter", "esc", "q") // quitting with a filter set must not panic
}

func TestPickKeyFromASubdirectoryAndMarkPassphrases(t *testing.T) {
	m, _ := fixture(t, newRepo(t))
	dir, _, _, _ := sshDirFixture(t)
	m.sshDir = dir
	press(t, m, "l", "l", "l", "a", "ctrl+o", "down", "down", "down", "enter") // sub/id_work
	if got := m.form.get("private"); got != filepath.Join(dir, "sub", "id_work") {
		t.Errorf("private = %q", got)
	}
	if got := m.form.get("label"); got != "work" {
		t.Errorf("label suggested from the base name: %q", got)
	}
	// Only the passphrase protected key carries the marker.
	press(t, m, "ctrl+o")
	if v := m.View(); strings.Count(v, "(passphrase)") != 1 || !strings.Contains(v, "id_secret") {
		t.Errorf("passphrase marker:\n%s", v)
	}
}
