package store

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
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
	if runtime.GOOS == "windows" {
		return // POSIX permission bits do not exist there
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
	if err != nil || p != filepath.Join("/x", "git-id", "book.toml") {
		t.Errorf("path = %q, err %v", p, err)
	}
	if runtime.GOOS == "windows" {
		return // falls back to %AppData%, not $HOME
	}
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "/h")
	p, _ = DefaultPath()
	if p != "/h/.config/git-id/book.toml" {
		t.Errorf("path = %q", p)
	}
}

func TestRenameDatapointFollowsIdentities(t *testing.T) {
	b := sample(t)
	if err := b.RenameDatapoint(KindEmail, "work", "company"); err != nil {
		t.Fatal(err)
	}
	if _, ok := b.Emails["work"]; ok || b.Emails["company"] != "dirk@example.com" {
		t.Errorf("emails = %v", b.Emails)
	}
	if got := b.Identities["work"].Email; got != "company" {
		t.Errorf("identity email = %q, want company", got)
	}
	if got := b.Identities["priv"].Email; got != "priv" {
		t.Errorf("unrelated identity changed: %q", got)
	}
	if err := b.Validate(); err != nil {
		t.Errorf("book invalid after rename: %v", err)
	}
	r, err := b.Resolve("work")
	if err != nil || r.Email != "dirk@example.com" {
		t.Errorf("resolve = %+v, %v", r, err)
	}

	// The key of an identity follows too, and names/keys use their own namespace.
	if err := b.RenameDatapoint(KindKey, "k1", "main-key"); err != nil {
		t.Fatal(err)
	}
	if b.Identities["work"].Key != "main-key" || b.Identities["priv"].Key != "main-key" {
		t.Errorf("identities = %+v", b.Identities)
	}
	if err := b.RenameDatapoint(KindName, "me", "dirk"); err != nil {
		t.Fatal(err)
	}
	if b.Identities["work"].Name != "dirk" || b.Identities["work"].Key != "main-key" {
		t.Errorf("identity = %+v", b.Identities["work"])
	}
}

func TestRenameDatapointRefusals(t *testing.T) {
	b := sample(t)
	if err := b.RenameDatapoint(KindEmail, "work", "priv"); !errors.Is(err, ErrExists) {
		t.Errorf("collision: %v", err)
	}
	if err := b.RenameDatapoint(KindEmail, "ghost", "x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing: %v", err)
	}
	if err := b.RenameDatapoint(KindEmail, "work", "bad label"); err == nil {
		t.Error("invalid label accepted")
	}
	if err := b.RenameDatapoint(KindEmail, "work", "work"); err != nil {
		t.Errorf("rename to itself: %v", err)
	}
	// A name label equal to an email label is fine: kinds are separate.
	if err := b.RenameDatapoint(KindName, "me", "work"); err != nil {
		t.Errorf("cross-kind label reuse refused: %v", err)
	}
}

func TestRenameIdentity(t *testing.T) {
	b := sample(t)
	if err := b.RenameIdentity("work", "company"); err != nil {
		t.Fatal(err)
	}
	if _, ok := b.Identities["work"]; ok {
		t.Error("old label still present")
	}
	if _, err := b.Resolve("company"); err != nil {
		t.Errorf("renamed identity unusable: %v", err)
	}
	if err := b.RenameIdentity("company", "priv"); !errors.Is(err, ErrExists) {
		t.Errorf("collision: %v", err)
	}
	if err := b.RenameIdentity("ghost", "x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing: %v", err)
	}
	if err := b.RenameIdentity("company", "-bad"); err == nil {
		t.Error("invalid label accepted")
	}
}
