package cleanup

import (
	"bufio"
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"

	"git-id/internal/store"
)

func testBook(t *testing.T) (*store.Book, store.Resolved) {
	t.Helper()
	b := store.New()
	for _, err := range []error{
		b.AddName("me", "Dirk Heilig"),
		b.AddName("old", "Old Name"),
		b.AddEmail("work", "dirk@example.com"),
		b.AddEmail("old", "old@example.org"),
		b.AddKey("k", store.Key{Private: "/k"}),
		b.AddIdentity("work", store.Identity{Name: "me", Email: "work", Key: "k"}),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}
	r, err := b.Resolve("work")
	if err != nil {
		t.Fatal(err)
	}
	return b, r
}

func TestFindReportsOnlyKnownForeignValues(t *testing.T) {
	b, cur := testBook(t)
	commits := []Commit{
		{"1", "Old Name", "old@example.org", "Old Name", "old@example.org"}, // author == committer: counted once per value
		{"2", "Old Name", "OLD@example.org", "Dirk Heilig", "dirk@example.com"},
		{"3", "Dirk Heilig", "dirk@example.com", "Dirk Heilig", "DIRK@example.com"}, // current, case differs
		{"4", "Stranger", "s@else.net", "Stranger", "s@else.net"},                   // not in the book
	}
	names, emails := Find(commits, KnownFrom(b, cur))
	if want := []Finding{{Name, "Old Name", 2}}; !reflect.DeepEqual(names, want) {
		t.Errorf("names = %+v, want %+v", names, want)
	}
	want := []Finding{{Email, "OLD@example.org", 1}, {Email, "old@example.org", 1}}
	if !reflect.DeepEqual(emails, want) {
		t.Errorf("emails = %+v, want %+v", emails, want)
	}
}

func TestFindOrdersByCountThenValue(t *testing.T) {
	b, cur := testBook(t)
	var commits []Commit
	for i := 0; i < 3; i++ {
		commits = append(commits, Commit{"h", "Dirk Heilig", "old@example.org", "Dirk Heilig", "old@example.org"})
	}
	commits = append(commits, Commit{"h", "Dirk Heilig", "OLD@example.org", "Dirk Heilig", "dirk@example.com"})
	_, emails := Find(commits, KnownFrom(b, cur))
	if len(emails) != 2 || emails[0].Value != "old@example.org" || emails[0].Count != 3 {
		t.Errorf("emails = %+v", emails)
	}
}

func TestPlanAffected(t *testing.T) {
	p := NewPlan()
	p.Add(Finding{Name, "Old Name", 1}, "New")
	p.Add(Finding{Email, "old@example.org", 1}, "new@x")
	commits := []Commit{
		{"a", "Old Name", "z", "z", "z"}, // author name
		{"b", "z", "z", "Old Name", "z"}, // committer name
		{"c", "z", "old@example.org", "z", "z"},
		{"d", "z", "z", "z", "old@example.org"},
		{"e", "z", "z", "z", "z"},
	}
	if got := p.Affected(commits); !reflect.DeepEqual(got, []string{"a", "b", "c", "d"}) {
		t.Errorf("affected = %v", got)
	}
	if p.Empty() || !NewPlan().Empty() {
		t.Error("Empty wrong")
	}
}

func TestCallbackIsSafePython(t *testing.T) {
	got := callback("name", map[string]string{`O"Brien\`: "Zoë\n"})
	want := `return {b"\x4f\x22\x42\x72\x69\x65\x6e\x5c": b"\x5a\x6f\xc3\xab\x0a"}.get(name, name)`
	if got != want {
		t.Errorf("callback = %s\nwant      %s", got, want)
	}
	if callback("name", nil) != "" {
		t.Error("empty map must give no callback")
	}
}

func ask(t *testing.T, input string, names, emails []Finding) (Plan, string, error) {
	t.Helper()
	var out bytes.Buffer
	p, err := Ask(bufio.NewReader(strings.NewReader(input)), &out, Target{"NEW", "new@x"}, names, emails)
	return p, out.String(), err
}

var (
	n1, n2 = Finding{Name, "A", 3}, Finding{Name, "B", 2}
	e1, e2 = Finding{Email, "a@x", 3}, Finding{Email, "b@x", 2}
)

func TestAskOneByOne(t *testing.T) {
	p, out, err := ask(t, "y\nn\nN\nY\n", []Finding{n1, n2}, []Finding{e1, e2})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(p.Names, map[string]string{"A": "NEW"}) || !reflect.DeepEqual(p.Emails, map[string]string{"b@x": "new@x"}) {
		t.Errorf("plan = %+v", p)
	}
	if !strings.Contains(out, `"A"  (3 commits)  ->  "NEW"`) {
		t.Errorf("prompt lacks details:\n%s", out)
	}
}

func TestAskBatchPerKind(t *testing.T) {
	// "a" on the first name takes all names; "s" on the first email skips all emails.
	p, _, err := ask(t, "a\ns\n", []Finding{n1, n2}, []Finding{e1, e2})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Names) != 2 || len(p.Emails) != 0 {
		t.Errorf("plan = %+v", p)
	}
	// and the other way around, with no further questions asked
	p, out, err := ask(t, "s\na\n", []Finding{n1, n2}, []Finding{e1, e2})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Names) != 0 || len(p.Emails) != 2 {
		t.Errorf("plan = %+v", p)
	}
	if strings.Count(out, "replace?") != 2 {
		t.Errorf("asked %d times, want 2:\n%s", strings.Count(out, "replace?"), out)
	}
}

func TestAskQuitAndEOF(t *testing.T) {
	for _, in := range []string{"y\nq\n", "y\n", ""} {
		if _, _, err := ask(t, in, []Finding{n1, n2}, nil); !errors.Is(err, ErrAborted) {
			t.Errorf("input %q: err = %v, want ErrAborted", in, err)
		}
	}
}

func TestAskRepromptsOnGarbage(t *testing.T) {
	p, out, err := ask(t, "what\n\ny\n", []Finding{n1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Names) != 1 || !strings.Contains(out, "please answer") {
		t.Errorf("plan %+v out:\n%s", p, out)
	}
}

func TestConfirmNeedsFullWord(t *testing.T) {
	for in, want := range map[string]bool{"yes\n": true, "y\n": false, "YES\n": false, "\n": false, "": false} {
		var out bytes.Buffer
		if got := Confirm(bufio.NewReader(strings.NewReader(in)), &out, "Go?"); got != want {
			t.Errorf("Confirm(%q) = %v", in, got)
		}
	}
}

func TestMissingFilterRepoGivesInstallHint(t *testing.T) {
	old := filterRepoSub
	filterRepoSub = "filter-repo-that-does-not-exist"
	defer func() { filterRepoSub = old }()
	err := CheckFilterRepo(t.TempDir())
	if !errors.Is(err, ErrNoFilterRepo) || !strings.Contains(err.Error(), "install") {
		t.Errorf("err = %v", err)
	}
}
