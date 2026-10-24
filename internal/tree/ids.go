package tree

import (
	"strconv"

	"github.com/jondoveston/xbeller/internal/xbel"
)

// idAlloc decides whether new nodes get an id attribute and what it is.
//
//   - Floccus files (those with a highestId comment) need an integer id on
//     every bookmark and folder, and the comment must track the largest one:
//     Floccus hands out highestId+1 for its next item, so a stale counter
//     would make it reuse ids. New ids continue from the larger of the
//     comment and the biggest id in the tree, and the comment is updated.
//   - Other files whose ids are all integers get the next integer.
//   - Files with other ids (like "b1") get random ids; files with no ids at
//     all stay that way.
type idAlloc struct {
	used      map[string]bool
	enabled   bool // new nodes get ids at all
	numeric   bool // ids are sequential integers
	mandatory bool // every bookmark and folder must have one (Floccus)
	comment   *xbel.Node
	start     int // largest integer id or counter seen before any allocation
	highest   int // largest integer id seen or handed out
}

func newIDAlloc(root *xbel.Node) *idAlloc {
	a := &idAlloc{used: map[string]bool{}}
	allInts := true
	root.Walk(func(n *xbel.Node) {
		id, ok := n.Lookup("id")
		if !ok {
			return
		}
		a.used[id] = true
		if v, err := strconv.Atoi(id); err != nil || v < 0 {
			allInts = false
		} else if v > a.highest {
			a.highest = v
		}
	})
	if c, v, ok := xbel.FindHighestID(root); ok {
		a.comment, a.mandatory = c, true
		a.highest = max(a.highest, v)
	}
	a.numeric = a.mandatory || (len(a.used) > 0 && allInts)
	a.enabled = a.numeric || len(a.used) > 0
	a.start = a.highest
	return a
}

// next returns an unused id.
func (a *idAlloc) next() string {
	for {
		var id string
		if a.numeric {
			a.highest++
			id = strconv.Itoa(a.highest)
		} else {
			id = NewID()
		}
		if !a.used[id] {
			a.used[id] = true
			return id
		}
	}
}

// finish records the ids handed out in the Floccus counter comment. Call it
// once an operation has succeeded.
func (a *idAlloc) finish() {
	if a.comment != nil && a.highest > a.start {
		xbel.SetHighestID(a.comment, a.highest)
	}
}

// stamp gives a newly created node its id (if the document uses them) and,
// except in Floccus files, an added timestamp.
func (a *idAlloc) stamp(n *xbel.Node) {
	if a.enabled {
		n.SetAttr("id", a.next())
	}
	if !a.mandatory {
		n.SetAttr("added", timestamp())
	}
}

// renumber gives every node in a freshly copied subtree a new id. Nodes that
// had one get a new one; in Floccus files every bookmark and folder gets one.
func (a *idAlloc) renumber(copy *xbel.Node) {
	copy.Walk(func(n *xbel.Node) {
		if _, has := n.Lookup("id"); has || (a.mandatory && isNamed(n)) {
			n.SetAttr("id", a.next())
		}
	})
}
