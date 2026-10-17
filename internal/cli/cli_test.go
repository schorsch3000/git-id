package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// run executes the CLI against a book in dir and returns stdout and the error.
func run(t *testing.T, book string, args ...string) (string, error) {
	t.Helper()
	cmd := NewRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(append([]string{"--book", book}, args...))
	err := cmd.Execute()
	return out.String(), err
}

func mustRun(t *testing.T, book string, args ...string) string {
	t.Helper()
	out, err := run(t, book, args...)
	if err != nil {
		t.Fatalf("git id %v: %v\n%s", args, err, out)
	}
	return out
}

func setup(t *testing.T) (book, key, pub string) {
	t.Helper()
	dir := t.TempDir()
	key = filepath.Join(dir, "id_work")
	pub = key + ".pub"
	for _, f := range []string{key, pub} {
		if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return filepath.Join(dir, "book.toml"), key, pub
}

func TestFullFlow(t *testing.T) {
	book, key, pub := setup(t)
	mustRun(t, book, "name", "add", "me", "Dirk Heilig")
	mustRun(t, book, "email", "add", "work", "dirk@example.com")
	mustRun(t, book, "key", "add", "k1", key, "--public", pub)
	mustRun(t, book, "add", "work", "--name", "me", "--email", "work", "--key", "k1")

	if out := mustRun(t, book, "ls"); !strings.Contains(out, "work") {
		t.Errorf("ls: %q", out)
	}
	if out := mustRun(t, book, "name", "ls"); !strings.Contains(out, "Dirk Heilig") {
		t.Errorf("name ls: %q", out)
	}
	if out := mustRun(t, book, "key", "ls"); !strings.Contains(out, key) || !strings.Contains(out, pub) {
		t.Errorf("key ls: %q", out)
	}

	mustRun(t, book, "email", "edit", "work", "new@example.com")
	out := mustRun(t, book, "show", "work")
	for _, want := range []string{"Dirk Heilig", "new@example.com", key, pub} {
		if !strings.Contains(out, want) {
			t.Errorf("show missing %q:\n%s", want, out)
		}
	}
}

func TestRmInUseThenForce(t *testing.T) {
	book, key, _ := setup(t)
	mustRun(t, book, "name", "add", "me", "A")
	mustRun(t, book, "email", "add", "e", "a@b.c")
	mustRun(t, book, "key", "add", "k", key)
	mustRun(t, book, "add", "x", "--name", "me", "--email", "e", "--key", "k")

	if _, err := run(t, book, "email", "rm", "e"); err == nil {
		t.Fatal("rm of used email succeeded")
	}
	out := mustRun(t, book, "email", "rm", "e", "--force")
	if !strings.Contains(out, "removed identity x") {
		t.Errorf("force output: %q", out)
	}
	if out := mustRun(t, book, "ls"); strings.TrimSpace(out) != "" {
		t.Errorf("identity survived: %q", out)
	}
}

func TestKeyValidation(t *testing.T) {
	book, key, _ := setup(t)
	if _, err := run(t, book, "key", "add", "k", filepath.Join(filepath.Dir(key), "missing")); err == nil {
		t.Error("missing key file accepted")
	}
	if _, err := run(t, book, "key", "add", "k", filepath.Dir(key)); err == nil {
		t.Error("directory accepted as key")
	}
}

func TestKeyEditAndPublicClear(t *testing.T) {
	book, key, pub := setup(t)
	mustRun(t, book, "key", "add", "k", key, "--public", pub)
	mustRun(t, book, "key", "edit", "k", "--public", "")
	if out := mustRun(t, book, "key", "ls"); strings.Contains(out, pub) {
		t.Errorf("public not cleared: %q", out)
	}
	if _, err := run(t, book, "key", "edit", "k"); err == nil {
		t.Error("edit without flags succeeded")
	}
}

func TestIdentityEditAndRm(t *testing.T) {
	book, key, _ := setup(t)
	mustRun(t, book, "name", "add", "me", "A")
	mustRun(t, book, "email", "add", "e1", "a@b.c")
	mustRun(t, book, "email", "add", "e2", "d@e.f")
	mustRun(t, book, "key", "add", "k", key)
	mustRun(t, book, "add", "x", "--name", "me", "--email", "e1", "--key", "k")
	mustRun(t, book, "edit", "x", "--email", "e2")
	if out := mustRun(t, book, "show", "x"); !strings.Contains(out, "d@e.f") {
		t.Errorf("show: %q", out)
	}
	if _, err := run(t, book, "edit", "x", "--email", "ghost"); err == nil {
		t.Error("edit to missing datapoint succeeded")
	}
	mustRun(t, book, "rm", "x")
	if _, err := run(t, book, "show", "x"); err == nil {
		t.Error("show of removed identity succeeded")
	}
}

func TestAddIdentityRequiresFlags(t *testing.T) {
	book, _, _ := setup(t)
	if _, err := run(t, book, "add", "x", "--name", "me"); err == nil {
		t.Error("add without --email/--key succeeded")
	}
}

func TestBookEnv(t *testing.T) {
	book, _, _ := setup(t)
	t.Setenv(bookEnv, book)
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"name", "add", "me", "A"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(book); err != nil {
		t.Errorf("book not written to env path: %v", err)
	}
}
