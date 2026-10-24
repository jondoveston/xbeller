package tree

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jondoveston/xbeller/internal/xbel"
)

func load(t *testing.T) *xbel.Node {
	t.Helper()
	f, err := os.Open("../xbel/testdata/sample.xbel")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	doc, err := xbel.Parse(f)
	if err != nil {
		t.Fatal(err)
	}
	return doc.Root
}

func deterministic(t *testing.T) {
	t.Helper()
	oldNow, oldID := Now, NewID
	n := 0
	Now = func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) }
	NewID = func() string { n++; return fmt.Sprintf("new%d", n) }
	t.Cleanup(func() { Now, NewID = oldNow, oldID })
}

// titles lists the titles of a folder's named children.
func titles(t *testing.T, root *xbel.Node, path string) []string {
	t.Helper()
	e, err := Resolve(root, path)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, c := range e.Node.Children {
		if isNamed(c) {
			out = append(out, c.Title)
		}
	}
	return out
}

func TestHostGlobs(t *testing.T) {
	g := HostGlobs{"example.com", "*.Example.org"}
	for href, want := range map[string]bool{
		"https://example.com/x?y=1":       true,
		"https://EXAMPLE.com:8080/":       true,
		"https://www.example.com/":        false,
		"https://a.b.example.org/":        true,
		"https://example.org/":            false,
		"mailto:me@example.com":           false,
		"javascript:void(0)":              false,
		"://bad":                          false,
		"":                                false,
		"https://user@example.com:1/path": true,
	} {
		if got := g.MatchURL(href); got != want {
			t.Errorf("MatchURL(%q) = %v, want %v", href, got, want)
		}
	}
	if err := (HostGlobs{"ok", "["}).Validate(); err == nil {
		t.Error("invalid pattern accepted")
	}
	if err := g.Validate(); err != nil {
		t.Error(err)
	}
}

func TestStripQuery(t *testing.T) {
	for in, want := range map[string]string{
		"https://a.com/p?x=1":          "https://a.com/p",
		"https://a.com/p?":             "https://a.com/p",
		"https://a.com/p#frag":         "https://a.com/p",
		"https://a.com/p?a=1#frag":     "https://a.com/p",
		"https://a.com:8080/p?a=1":     "https://a.com:8080/p",
		"https://a.com/p":              "https://a.com/p",
		"https://a.com/a%20b?x=1":      "https://a.com/a%20b",
		"mailto:me@a.com?subject=hi":   "mailto:me@a.com",
		"://bad?x=1":                   "://bad?x=1",
		"https://a.com/p?v=1&w=%7Efoo": "https://a.com/p",
	} {
		if got := StripQuery(in); got != want {
			t.Errorf("StripQuery(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRemoveMatching(t *testing.T) {
	root := load(t)
	// Searches one level only: Deep link is in a subfolder.
	log, err := RemoveMatching(root, []string{"/Work"}, HostGlobs{"*.example.com"})
	if err != nil || len(log) != 0 {
		t.Fatalf("log %v err %v", log, err)
	}
	log, err = RemoveMatching(root, []string{"/Work"}, HostGlobs{"example.com"})
	if err != nil || !reflect.DeepEqual(log, []string{"rm /Work/Work A"}) {
		t.Fatalf("log %v err %v", log, err)
	}
	if got := titles(t, root, "/Work"); !reflect.DeepEqual(got, []string{"Deep"}) {
		t.Errorf("/Work = %v", got)
	}
	// Overlapping paths remove each bookmark once, and folders stay.
	log, err = RemoveMatching(root, []string{"/Dupes", "/Dupes/Same[1]", "/"}, HostGlobs{"*"})
	want := []string{"rm /Dupes/Same[1]", "rm /Dupes/Same[2]", "rm /Example"}
	if err != nil || !reflect.DeepEqual(log, want) {
		t.Fatalf("log %v err %v, want %v", log, err, want)
	}
	if got := titles(t, root, "/Dupes"); !reflect.DeepEqual(got, []string{"Same"}) {
		t.Errorf("/Dupes = %v", got)
	}
	// Errors change nothing.
	if _, err := RemoveMatching(root, []string{"/Work"}, HostGlobs{"["}); err == nil {
		t.Error("bad pattern accepted")
	}
	if _, err := RemoveMatching(root, []string{"/Work", "/Missing"}, HostGlobs{"*"}); err == nil {
		t.Error("missing path accepted")
	}
}

func TestSplitPath(t *testing.T) {
	tests := []struct {
		in   string
		want []Segment
	}{
		{"", nil},
		{"/", nil},
		{"//a//b/", []Segment{{"a", 0}, {"b", 0}}},
		{"/Work/Deep", []Segment{{"Work", 0}, {"Deep", 0}}},
		{`/a\/b/c`, []Segment{{"a/b", 0}, {"c", 0}}},
		{`/a\\b`, []Segment{{`a\b`, 0}}},
		{"/Same[2]", []Segment{{"Same", 2}}},
		{"/[1]", []Segment{{"", 1}}},
		{`/Same\[2]`, []Segment{{"Same[2]", 0}}},
		{"/[PDF] doc", []Segment{{"[PDF] doc", 0}}},
		{"/x[]", []Segment{{"x[]", 0}}},
		{"/x[a]", []Segment{{"x[a]", 0}}},
		{"/a[1]/b[12]", []Segment{{"a", 1}, {"b", 12}}},
	}
	for _, tt := range tests {
		got, err := SplitPath(tt.in)
		if err != nil {
			t.Errorf("SplitPath(%q): %v", tt.in, err)
			continue
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("SplitPath(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
	for _, bad := range []string{`/a\`, "/a[0]", "/a[99999999999999999999]"} {
		if _, err := SplitPath(bad); err == nil {
			t.Errorf("SplitPath(%q) should fail", bad)
		}
	}
}

func TestEscapeRoundTrip(t *testing.T) {
	for _, name := range []string{"plain", "a/b", `a\b`, "x[1]", `x\[1]`, "[PDF] x", "", "a[1][2]", `\`, "//"} {
		segs, err := SplitPath("/" + Escape(name))
		if name == "" {
			if len(segs) != 0 {
				t.Errorf("empty name should vanish, got %v", segs)
			}
			continue
		}
		if err != nil || len(segs) != 1 || segs[0].Name != name || segs[0].Index != 0 {
			t.Errorf("Escape(%q) = %q parsed back as %v, %v", name, Escape(name), segs, err)
		}
	}
}

func TestResolve(t *testing.T) {
	root := load(t)
	ok := map[string]string{
		"/":                    "Bookmarks",
		"":                     "Bookmarks",
		"/Example":             "Example",
		"/Work":                "Work",
		"/Work/Deep/Deep link": "Deep link",
		"/Dupes/Same[2]":       "Same",
	}
	for path, title := range ok {
		e, err := Resolve(root, path)
		if err != nil {
			t.Errorf("Resolve(%q): %v", path, err)
			continue
		}
		if e.Node.Title != title {
			t.Errorf("Resolve(%q) title = %q, want %q", path, e.Node.Title, title)
		}
	}
	if e, _ := Resolve(root, "/Dupes/Same[2]"); e.Node.Href() != "https://two.example.com/" {
		t.Errorf("[2] picked the wrong node: %+v", e.Node)
	}
	if e, _ := Resolve(root, "/Dupes/Same[3]"); e.Node.Kind != xbel.KindFolder {
		t.Errorf("[3] should be the folder")
	}

	errs := map[string]error{
		"/Nope":                  ErrNotFound,
		"/Work/Nope":             ErrNotFound,
		"/Dupes/Same":            ErrAmbiguous,
		"/Dupes/Same[4]":         ErrNotFound,
		"/Example/child":         ErrNotFolder,
		"/Work/Deep/Deep link/x": ErrNotFolder,
	}
	for path, want := range errs {
		if _, err := Resolve(root, path); !errors.Is(err, want) {
			t.Errorf("Resolve(%q) error = %v, want %v", path, err, want)
		}
	}
}

func TestDisplayNames(t *testing.T) {
	root := load(t)
	dupes, _ := Resolve(root, "/Dupes")
	want := []string{"Same[1]", "Same[2]", "Same[3]"}
	if got := DisplayNames(dupes.Node.Children); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	work, _ := Resolve(root, "/Work")
	want = []string{"Work A", "---", "Deep", "@b1", "?"}
	if got := DisplayNames(work.Node.Children); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	// Every displayed name must resolve back to the node it came from.
	for i, name := range DisplayNames(dupes.Node.Children) {
		e, err := Resolve(root, "/Dupes/"+name)
		if err != nil || e.Node != dupes.Node.Children[i] {
			t.Errorf("%q did not resolve back: %v", name, err)
		}
	}
}

func TestAdd(t *testing.T) {
	deterministic(t)
	root := load(t)
	if _, err := Add(root, "/Work/Deep", "New", "https://new.example.com/", "a note"); err != nil {
		t.Fatal(err)
	}
	e, err := Resolve(root, "/Work/Deep/New")
	if err != nil {
		t.Fatal(err)
	}
	n := e.Node
	if n.Href() != "https://new.example.com/" || n.Desc != "a note" ||
		n.Attr("added") != "2026-01-02T03:04:05Z" || n.Attr("id") != "new1" {
		t.Errorf("unexpected bookmark %+v", n)
	}
	if _, err := Add(root, "/Example", "x", "https://x/", ""); !errors.Is(err, ErrNotFolder) {
		t.Errorf("adding into a bookmark: %v", err)
	}
	if _, err := Add(root, "/Nope", "x", "https://x/", ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("adding into a missing folder: %v", err)
	}
	if _, err := Add(root, "/", "x", "", ""); err == nil {
		t.Error("empty URL should be rejected")
	}
}

func TestAddWithoutIDs(t *testing.T) {
	deterministic(t)
	root := xbel.NewFolder("")
	root.Kind = xbel.KindRoot
	if _, err := Add(root, "/", "x", "https://x/", ""); err != nil {
		t.Fatal(err)
	}
	if _, ok := root.Children[0].Lookup("id"); ok {
		t.Error("ids should not be introduced into a document that has none")
	}
}

func TestMkdir(t *testing.T) {
	deterministic(t)
	root := load(t)
	if _, err := Mkdir(root, "/Work/New", false); err != nil {
		t.Fatal(err)
	}
	if e, err := Resolve(root, "/Work/New"); err != nil || e.Node.Kind != xbel.KindFolder || e.Node.Attr("id") == "" {
		t.Errorf("folder not created properly: %v %+v", err, e.Node)
	}
	if _, err := Mkdir(root, "/Work/New", false); !errors.Is(err, ErrExists) {
		t.Errorf("duplicate mkdir: %v", err)
	}
	if _, err := Mkdir(root, "/Work", false); !errors.Is(err, ErrExists) {
		t.Errorf("mkdir of existing folder: %v", err)
	}
	if _, err := Mkdir(root, "/A/B/C", false); !errors.Is(err, ErrNotFound) {
		t.Errorf("mkdir without parent: %v", err)
	}
	if _, err := Mkdir(root, "/A/B/C", true); err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve(root, "/A/B/C"); err != nil {
		t.Error(err)
	}
	if log, err := Mkdir(root, "/A/B", true); err != nil || len(log) != 0 {
		t.Errorf("mkdir -p on existing: %v %v", log, err)
	}
	if _, err := Mkdir(root, "/Example/x", true); !errors.Is(err, ErrNotFolder) {
		t.Errorf("mkdir under a bookmark: %v", err)
	}
	if _, err := Mkdir(root, "/", true); !errors.Is(err, ErrExists) {
		t.Errorf("mkdir root: %v", err)
	}
}

func TestMove(t *testing.T) {
	root := load(t)
	// Into an existing folder: keeps the id.
	if _, err := Move(root, []string{"/Example"}, "/Work"); err != nil {
		t.Fatal(err)
	}
	e, err := Resolve(root, "/Work/Example")
	if err != nil || e.Node.Attr("id") != "b1" {
		t.Fatalf("move lost the node or its id: %v", err)
	}
	if _, err := Resolve(root, "/Example"); !errors.Is(err, ErrNotFound) {
		t.Error("source still present")
	}

	// Rename in place keeps position.
	before := titles(t, root, "/Work")
	if _, err := Move(root, []string{"/Work/Work A"}, "/Work/Renamed"); err != nil {
		t.Fatal(err)
	}
	after := titles(t, root, "/Work")
	if after[0] != "Renamed" || len(after) != len(before) {
		t.Errorf("rename changed order or count: %v -> %v", before, after)
	}

	// Move and rename at once.
	if _, err := Move(root, []string{"/Work/Renamed"}, "/Dupes/Fresh"); err != nil {
		t.Fatal(err)
	}
	if e, err := Resolve(root, "/Dupes/Fresh"); err != nil || e.Node.Attr("id") != "b2" {
		t.Errorf("move+rename: %v", err)
	}

	// Several sources, plus duplicates of the same one.
	if _, err := Move(root, []string{"/Dupes/Same[1]", "/Dupes/Same[1]", "/Dupes/Fresh"}, "/Work/Deep"); err != nil {
		t.Fatal(err)
	}
	if got := titles(t, root, "/Work/Deep"); !reflect.DeepEqual(got, []string{"Deep link", "Same", "Fresh"}) {
		t.Errorf("deep = %v", got)
	}

	// Moving into the folder it is already in is a no-op.
	if log, err := Move(root, []string{"/Work/Deep"}, "/Work"); err != nil || len(log) != 0 {
		t.Errorf("no-op move: %v %v", log, err)
	}
}

func TestMoveSelectorsAreResolvedUpFront(t *testing.T) {
	root := load(t)
	// After moving Same[1], the old Same[2] would become Same[1]; both
	// selectors must still refer to the original nodes.
	if _, err := Move(root, []string{"/Dupes/Same[1]", "/Dupes/Same[2]"}, "/Work"); err != nil {
		t.Fatal(err)
	}
	var hrefs []string
	e, _ := Resolve(root, "/Work")
	for _, c := range e.Node.Children {
		if c.Kind == xbel.KindBookmark && c.Title == "Same" {
			hrefs = append(hrefs, c.Href())
		}
	}
	if !reflect.DeepEqual(hrefs, []string{"https://one.example.com/", "https://two.example.com/"}) {
		t.Errorf("hrefs = %v", hrefs)
	}
}

func TestMoveErrors(t *testing.T) {
	root := load(t)
	cases := []struct {
		srcs []string
		dest string
		want error
	}{
		{[]string{"/Work"}, "/Work/Deep", nil},                   // into own descendant
		{[]string{"/Work"}, "/Work", nil},                        // into itself
		{[]string{"/"}, "/Work", nil},                            // the root
		{[]string{"/Nope"}, "/Work", ErrNotFound},                // missing source
		{[]string{"/Example"}, "/Dupes/Same[1]", ErrExists},      // onto a bookmark
		{[]string{"/Example"}, "/Nope/x", ErrNotFound},           // missing parent
		{[]string{"/Example"}, "/Example/x", ErrNotFolder},       // parent is a bookmark
		{[]string{"/Example", "/Work"}, "/NewName", ErrNotFound}, // many sources, no folder
		{[]string{"/Example"}, "/Work/x[2]", nil},                // index in new name
		{[]string{"/Dupes/Same"}, "/Work", ErrAmbiguous},
	}
	for _, c := range cases {
		_, err := Move(root, c.srcs, c.dest)
		if err == nil {
			t.Errorf("Move(%v, %q) should fail", c.srcs, c.dest)
		} else if c.want != nil && !errors.Is(err, c.want) {
			t.Errorf("Move(%v, %q) = %v, want %v", c.srcs, c.dest, err, c.want)
		}
	}
	// None of the failures may have changed the tree.
	if got := titles(t, root, "/"); !reflect.DeepEqual(got, []string{"Example", "Work", "Dupes"}) {
		t.Errorf("tree changed after failed moves: %v", got)
	}
}

func TestCopy(t *testing.T) {
	deterministic(t)
	root := load(t)

	// Copy a bookmark: new id, everything else equal, original untouched.
	if _, err := Copy(root, []string{"/Example"}, "/Work", false); err != nil {
		t.Fatal(err)
	}
	orig, _ := Resolve(root, "/Example")
	cp, err := Resolve(root, "/Work/Example")
	if err != nil {
		t.Fatal(err)
	}
	if orig.Node.Attr("id") != "b1" || cp.Node.Attr("id") == "b1" || cp.Node.Attr("id") == "" {
		t.Errorf("ids: orig %q copy %q", orig.Node.Attr("id"), cp.Node.Attr("id"))
	}
	if cp.Node.Href() != orig.Node.Href() || cp.Node.Desc != orig.Node.Desc {
		t.Error("copy differs from the original")
	}

	// Copy with rename.
	if _, err := Copy(root, []string{"/Example"}, "/Copy", false); err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve(root, "/Copy"); err != nil {
		t.Error(err)
	}

	// Folders need recursive, and every nested id is refreshed.
	if _, err := Copy(root, []string{"/Work"}, "/Backup", false); err == nil {
		t.Error("copying a folder without -r should fail")
	}
	if _, err := Copy(root, []string{"/Work"}, "/Backup", true); err != nil {
		t.Fatal(err)
	}
	seen := map[string]int{}
	root.Walk(func(n *xbel.Node) {
		if id, ok := n.Lookup("id"); ok {
			seen[id]++
		}
	})
	for id, n := range seen {
		if n > 1 {
			t.Errorf("id %q appears %d times", id, n)
		}
	}
	if _, err := Resolve(root, "/Backup/Deep/Deep link"); err != nil {
		t.Error(err)
	}

	// Not into itself.
	if _, err := Copy(root, []string{"/Work"}, "/Work/Deep", true); err == nil {
		t.Error("copy into own descendant should fail")
	}
	// Same source twice makes two copies.
	n := len(titles(t, root, "/Dupes"))
	if _, err := Copy(root, []string{"/Example", "/Example"}, "/Dupes", false); err != nil {
		t.Fatal(err)
	}
	if got := len(titles(t, root, "/Dupes")); got != n+2 {
		t.Errorf("dupes has %d children, want %d", got, n+2)
	}
}

func TestRemove(t *testing.T) {
	root := load(t)
	if _, err := Remove(root, []string{"/Work"}, false); err == nil {
		t.Error("removing a folder without -r should fail")
	}
	if _, err := Remove(root, []string{"/"}, true); err == nil {
		t.Error("removing the root should fail")
	}
	if _, err := Remove(root, []string{"/Example", "/Nope"}, false); !errors.Is(err, ErrNotFound) {
		t.Errorf("got %v", err)
	}
	if got := titles(t, root, "/"); len(got) != 3 {
		t.Errorf("a failed remove must not delete anything: %v", got)
	}
	// Selectors refer to the original positions, and naming a node twice
	// is harmless.
	paths := []string{"/Example", "/Dupes/Same[1]", "/Dupes/Same[2]", "/Dupes/Same[1]"}
	log, err := Remove(root, paths, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(log) != 3 {
		t.Errorf("log = %v, want 3 entries", log)
	}
	if got := titles(t, root, "/Dupes"); !reflect.DeepEqual(got, []string{"Same"}) {
		t.Errorf("dupes = %v", got)
	}
	if e, _ := Resolve(root, "/Dupes/Same"); e.Node.Kind != xbel.KindFolder {
		t.Error("the folder named Same should be the one left")
	}
}

func TestRemoveRecursive(t *testing.T) {
	root := load(t)
	if _, err := Remove(root, []string{"/Work", "/Example"}, true); err != nil {
		t.Fatal(err)
	}
	if got := titles(t, root, "/"); !reflect.DeepEqual(got, []string{"Dupes"}) {
		t.Errorf("got %v", got)
	}
}

func TestErrorMessagesMentionPath(t *testing.T) {
	root := load(t)
	_, err := Resolve(root, "/Work/Nope")
	if err == nil || !strings.Contains(err.Error(), "/Work/Nope") {
		t.Errorf("error should include the path: %v", err)
	}
}
