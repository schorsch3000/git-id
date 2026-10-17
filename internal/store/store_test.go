package store

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func sample(t *testing.T) *Book {
	t.Helper()
	b := New()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(b.AddName("me", "Dirk Heilig"))
	must(b.AddEmail("work", "dirk@example.com"))
	must(b.AddEmail("priv", "dirk@example.org"))
	must(b.AddKey("k1", Key{Private: "/k/id_work", Public: "/k/id_work.pub"}))
	must(b.AddIdentity("work", Identity{Name: "me", Email: "work", Key: "k1"}))
	must(b.AddIdentity("priv", Identity{Name: "me", Email: "priv", Key: "k1"}))
	return b
}

func TestLabelValidation(t *testing.T) {
	for _, l := range []string{"", "-x", " a", "a b", "a/b", ".a"} {
		if ValidateLabel(l) == nil {
			t.Errorf("label %q should be invalid", l)
		}
	}
	for _, l := range []string{"a", "work-1", "a.b_c", "9x"} {
		if err := ValidateLabel(l); err != nil {
			t.Errorf("label %q should be valid: %v", l, err)
		}
	}
}

func TestAddDuplicateAndEmpty(t *testing.T) {
	b := sample(t)
	if err := b.AddName("me", "X"); !errors.Is(err, ErrExists) {
		t.Errorf("want ErrExists, got %v", err)
	}
	if err := b.AddEmail("new", ""); err == nil {
		t.Error("empty value accepted")
	}
	if err := b.AddIdentity("x", Identity{Name: "nope", Email: "work", Key: "k1"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestEditPropagates(t *testing.T) {
	b := sample(t)
	if err := b.SetEmail("work", "new@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := b.SetKey("k1", Key{Private: "/k/rotated"}); err != nil {
		t.Fatal(err)
	}
	r, err := b.Resolve("work")
	if err != nil {
		t.Fatal(err)
	}
	if r.Email != "new@example.com" || r.Key.Private != "/k/rotated" {
		t.Errorf("edit not propagated: %+v", r)
	}
	if err := b.SetName("ghost", "x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestRemoveRefusedWhenInUse(t *testing.T) {
	b := sample(t)
	if _, err := b.Remove(KindEmail, "work", false); !errors.Is(err, ErrInUse) {
		t.Fatalf("want ErrInUse, got %v", err)
	}
	if _, ok := b.Emails["work"]; !ok {
		t.Fatal("datapoint removed despite refusal")
	}
}

func TestRemoveForceRemovesDependents(t *testing.T) {
	b := sample(t)
	gone, err := b.Remove(KindEmail, "work", true)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gone, []string{"work"}) {
		t.Errorf("removed identities = %v", gone)
	}
	if _, ok := b.Identities["work"]; ok {
		t.Error("dependent identity kept")
	}
	if _, ok := b.Identities["priv"]; !ok {
		t.Error("unrelated identity removed")
	}
	if err := b.Validate(); err != nil {
		t.Errorf("book invalid after force remove: %v", err)
	}
}

func TestRemoveUnusedAndMissing(t *testing.T) {
	b := sample(t)
	if err := b.AddName("spare", "Spare"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Remove(KindName, "spare", false); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Remove(KindName, "spare", false); !errors.Is(err, ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestIdentityEditAndRemove(t *testing.T) {
	b := sample(t)
	if err := b.SetIdentity("work", Identity{Name: "me", Email: "priv", Key: "k1"}); err != nil {
		t.Fatal(err)
	}
	if err := b.SetIdentity("work", Identity{Name: "me", Email: "zzz", Key: "k1"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
	if err := b.RemoveIdentity("work"); err != nil {
		t.Fatal(err)
	}
	if err := b.RemoveIdentity("work"); !errors.Is(err, ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
	if _, ok := b.Emails["priv"]; !ok {
		t.Error("datapoint removed with identity")
	}
}

func TestLabelsSorted(t *testing.T) {
	b := sample(t)
	if got := b.Labels(KindEmail); !reflect.DeepEqual(got, []string{"priv", "work"}) {
		t.Errorf("labels = %v", got)
	}
	if got := b.IdentityLabels(); !reflect.DeepEqual(got, []string{"priv", "work"}) {
		t.Errorf("identity labels = %v", got)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	b := sample(t)
	path := filepath.Join(t.TempDir(), "sub", "book.toml")
	if err := Save(path, b); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b, got) {
		t.Errorf("round trip mismatch:\nwant %+v\ngot  %+v", b, got)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("file mode = %v, want 0600", fi.Mode().Perm())
	}
	di, _ := os.Stat(filepath.Dir(path))
	if di.Mode().Perm() != 0o700 {
		t.Errorf("dir mode = %v, want 0700", di.Mode().Perm())
	}
}

func TestLoadMissingIsEmpty(t *testing.T) {
	b, err := Load(filepath.Join(t.TempDir(), "none.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Identities) != 0 || b.Names == nil {
		t.Errorf("unexpected book: %+v", b)
	}
}

func TestLoadRejectsDanglingReference(t *testing.T) {
	path := filepath.Join(t.TempDir(), "book.toml")
	bad := "[identities.x]\nname = \"a\"\nemail = \"b\"\nkey = \"c\"\n"
	if err := os.WriteFile(path, []byte(bad), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); !errors.Is(err, ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestDefaultPathXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/x")
	p, err := DefaultPath()
	if err != nil || p != "/x/git-id/book.toml" {
		t.Errorf("path = %q, err %v", p, err)
	}
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "/h")
	p, _ = DefaultPath()
	if p != "/h/.config/git-id/book.toml" {
		t.Errorf("path = %q", p)
	}
}
