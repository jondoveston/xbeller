package tree

import (
	"os"
	"strconv"
	"testing"

	"github.com/jondoveston/xbeller/internal/xbel"
)

func loadFixture(t *testing.T, name string) *xbel.Node {
	t.Helper()
	f, err := os.Open("../xbel/testdata/" + name)
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

func highest(t *testing.T, root *xbel.Node) int {
	t.Helper()
	_, id, ok := xbel.FindHighestID(root)
	if !ok {
		t.Fatal("no highestId comment")
	}
	return id
}

func idOf(t *testing.T, root *xbel.Node, path string) string {
	t.Helper()
	e, err := Resolve(root, path)
	if err != nil {
		t.Fatal(err)
	}
	return e.Node.Attr("id")
}

// requireFloccusValid checks what Floccus insists on when it reads the file:
// every bookmark and folder has an integer id, and ids are unique.
func requireFloccusValid(t *testing.T, root *xbel.Node) {
	t.Helper()
	seen := map[string]bool{}
	maxID := 0
	root.Walk(func(n *xbel.Node) {
		if n.Kind != xbel.KindFolder && n.Kind != xbel.KindBookmark {
			return
		}
		id, ok := n.Lookup("id")
		v, err := strconv.Atoi(id)
		switch {
		case !ok || err != nil:
			t.Errorf("%s %q has no integer id (%q)", n.Kind, n.Title, id)
		case seen[id]:
			t.Errorf("id %s is used twice", id)
		}
		seen[id] = true
		maxID = max(maxID, v)
	})
	if h := highest(t, root); h < maxID {
		t.Errorf("highestId comment says %d but the largest id is %d", h, maxID)
	}
}

func TestFloccusAddAndMkdirContinueTheCounter(t *testing.T) {
	deterministic(t)
	root := loadFixture(t, "floccus.xbel")

	if _, err := Add(root, "/Toolbar", "New", "https://new.example.com/", ""); err != nil {
		t.Fatal(err)
	}
	if got := idOf(t, root, "/Toolbar/New"); got != "10" {
		t.Errorf("first new id = %s, want 10", got)
	}
	if _, err := Mkdir(root, "/A/B", true); err != nil {
		t.Fatal(err)
	}
	if a, b := idOf(t, root, "/A"), idOf(t, root, "/A/B"); a != "11" || b != "12" {
		t.Errorf("mkdir -p ids = %s, %s; want 11, 12", a, b)
	}
	if got := highest(t, root); got != 12 {
		t.Errorf("highestId = %d, want 12", got)
	}
	requireFloccusValid(t, root)

	// Floccus does not store these, and drops them on its next sync.
	e, _ := Resolve(root, "/Toolbar/New")
	if _, ok := e.Node.Lookup("added"); ok {
		t.Error("added attribute written to a Floccus file")
	}
}

func TestFloccusCopyRenumbersEverything(t *testing.T) {
	deterministic(t)
	root := loadFixture(t, "floccus.xbel")

	if _, err := Copy(root, []string{"/Toolbar"}, "/Backup", true); err != nil {
		t.Fatal(err)
	}
	// Pre-order: Backup 10, its bookmark 11, Nested 12, Deep 13, Empty 14.
	want := map[string]string{
		"/Backup":                     "10",
		"/Backup/It's \"quoted\" <b>": "11",
		"/Backup/Nested":              "12",
		"/Backup/Nested/Deep":         "13",
		"/Backup/Empty":               "14",
	}
	for path, id := range want {
		if got := idOf(t, root, path); got != id {
			t.Errorf("%s id = %s, want %s", path, got, id)
		}
	}
	// The originals keep theirs.
	if got := idOf(t, root, "/Toolbar/Nested/Deep"); got != "4" {
		t.Errorf("original changed to %s", got)
	}
	if got := highest(t, root); got != 14 {
		t.Errorf("highestId = %d, want 14", got)
	}
	requireFloccusValid(t, root)
}

func TestFloccusCopyGivesIDsToNodesThatLackThem(t *testing.T) {
	deterministic(t)
	root := loadFixture(t, "floccus.xbel")
	// Strip an id from a bookmark, as a hand-edited file might have.
	e, _ := Resolve(root, "/Toolbar/Nested/Deep")
	e.Node.DelAttr("id")

	if _, err := Copy(root, []string{"/Toolbar/Nested"}, "/Nested2", true); err != nil {
		t.Fatal(err)
	}
	if got := idOf(t, root, "/Nested2/Deep"); got != "11" {
		t.Errorf("copy of an id-less bookmark got id %q, want 11", got)
	}
}

func TestFloccusMoveAndRemoveKeepIDsAndCounter(t *testing.T) {
	root := loadFixture(t, "floccus.xbel")
	if _, err := Move(root, []string{"/Toolbar/Nested"}, "/"); err != nil {
		t.Fatal(err)
	}
	if got := idOf(t, root, "/Nested"); got != "3" {
		t.Errorf("mv changed the id to %s", got)
	}
	if _, err := Remove(root, []string{"/Nested", "/Toolbar/Empty"}, true); err != nil {
		t.Fatal(err)
	}
	// Ids of deleted items are never reused: the counter does not go down.
	if got := highest(t, root); got != 9 {
		t.Errorf("highestId = %d, want 9", got)
	}
}

func TestFloccusCounterIsRepairedWhenStale(t *testing.T) {
	deterministic(t)

	// The comment is behind the biggest id in the file.
	root := loadFixture(t, "floccus.xbel")
	c, _, _ := xbel.FindHighestID(root)
	xbel.SetHighestID(c, 3)
	if _, err := Add(root, "/", "x", "https://x/", ""); err != nil {
		t.Fatal(err)
	}
	if got := idOf(t, root, "/x"); got != "10" {
		t.Errorf("stale counter: id = %s, want 10 (above the largest existing id)", got)
	}
	if got := highest(t, root); got != 10 {
		t.Errorf("highestId = %d, want 10", got)
	}

	// The comment is ahead of the biggest id: Floccus may have used the gap.
	root = loadFixture(t, "floccus.xbel")
	c, _, _ = xbel.FindHighestID(root)
	xbel.SetHighestID(c, 100)
	if _, err := Add(root, "/", "x", "https://x/", ""); err != nil {
		t.Fatal(err)
	}
	if got := idOf(t, root, "/x"); got != "101" {
		t.Errorf("counter ahead: id = %s, want 101", got)
	}
}

func TestFailedOperationLeavesCounterAlone(t *testing.T) {
	root := loadFixture(t, "floccus.xbel")
	if _, err := Copy(root, []string{"/Toolbar"}, "/Backup", false); err == nil {
		t.Fatal("copying a folder without -r should fail")
	}
	if _, err := Add(root, "/Nope", "x", "https://x/", ""); err == nil {
		t.Fatal("expected failure")
	}
	if got := highest(t, root); got != 9 {
		t.Errorf("highestId = %d, want 9", got)
	}
}

func TestIntegerIDsWithoutFloccusComment(t *testing.T) {
	deterministic(t)
	root := &xbel.Node{Kind: xbel.KindRoot}
	a := xbel.NewBookmark("a", "https://a/")
	a.SetAttr("id", "7")
	b := xbel.NewFolder("b")
	b.SetAttr("id", "20")
	root.Children = []*xbel.Node{a, b}

	if _, err := Add(root, "/", "c", "https://c/", ""); err != nil {
		t.Fatal(err)
	}
	e, _ := Resolve(root, "/c")
	if e.Node.Attr("id") != "21" || e.Node.Attr("added") == "" {
		t.Errorf("got id %q, added %q; want 21 and a timestamp", e.Node.Attr("id"), e.Node.Attr("added"))
	}
	for _, c := range root.Children {
		if c.Kind == xbel.KindComment {
			t.Error("a comment appeared in a file that had none")
		}
	}
}
