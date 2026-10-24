// Package tree resolves slash-separated paths inside an XBEL tree and
// implements the list/add/copy/move/remove/mkdir operations on it.
//
// A path is a list of folder/bookmark titles separated by "/". Titles are
// matched exactly. Because XBEL allows several siblings to share a title, a
// segment may end in "[n]" (1-based) to pick the n-th sibling with that
// title. A literal "/" or "\" in a title is written "\/" or "\\", and a
// title that itself ends in "[digits]" is written with "\[".
package tree

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/jondoveston/xbeller/internal/xbel"
)

var (
	ErrNotFound  = errors.New("no such bookmark or folder")
	ErrAmbiguous = errors.New("ambiguous name")
	ErrNotFolder = errors.New("not a folder")
	ErrExists    = errors.New("already exists")
)

// Segment is one component of a parsed path.
type Segment struct {
	Name string
	// Index is the 1-based selector among siblings sharing Name, or 0 when
	// no selector was given.
	Index int
}

type char struct {
	r   rune
	esc bool
}

// SplitPath parses a path into segments. Empty segments (leading, trailing
// or doubled slashes) are ignored, so "" and "/" both mean the root.
func SplitPath(p string) ([]Segment, error) {
	var (
		segs []Segment
		cur  []char
	)
	flush := func() error {
		if len(cur) == 0 {
			return nil
		}
		seg, err := makeSegment(cur)
		cur = nil
		if err != nil {
			return err
		}
		segs = append(segs, seg)
		return nil
	}
	rs := []rune(p)
	for i := 0; i < len(rs); i++ {
		switch rs[i] {
		case '\\':
			if i+1 >= len(rs) {
				return nil, errors.New("path ends with a lone backslash")
			}
			i++
			cur = append(cur, char{rs[i], true})
		case '/':
			if err := flush(); err != nil {
				return nil, err
			}
		default:
			cur = append(cur, char{rs[i], false})
		}
	}
	if err := flush(); err != nil {
		return nil, err
	}
	return segs, nil
}

func makeSegment(cur []char) (Segment, error) {
	toString := func(cs []char) string {
		rs := make([]rune, len(cs))
		for i, c := range cs {
			rs[i] = c.r
		}
		return string(rs)
	}
	n := len(cur)
	if n >= 3 && cur[n-1].r == ']' && !cur[n-1].esc {
		j := n - 2
		for j >= 0 && !cur[j].esc && cur[j].r >= '0' && cur[j].r <= '9' {
			j--
		}
		if j >= 0 && j < n-2 && cur[j].r == '[' && !cur[j].esc {
			idx, err := strconv.Atoi(toString(cur[j+1 : n-1]))
			if err != nil || idx < 1 {
				return Segment{}, fmt.Errorf("invalid index in %q: must be 1 or greater", toString(cur))
			}
			return Segment{Name: toString(cur[:j]), Index: idx}, nil
		}
	}
	return Segment{Name: toString(cur)}, nil
}

var trailingIndex = regexp.MustCompile(`\[[0-9]+\]$`)

// Escape converts a title into a path segment that SplitPath parses back to
// the same title.
func Escape(name string) string {
	name = strings.ReplaceAll(name, `\`, `\\`)
	name = strings.ReplaceAll(name, `/`, `\/`)
	if loc := trailingIndex.FindStringIndex(name); loc != nil {
		name = name[:loc[0]] + `\` + name[loc[0]:]
	}
	return name
}

// Join appends an escaped segment to a parent path.
func Join(parent, segment string) string {
	return strings.TrimSuffix(parent, "/") + "/" + segment
}

// DisplayNames returns, for each child, the name to show in listings. Folders
// and bookmarks are shown as an escaped path segment, with a "[n]" selector
// added when the title is shared with a sibling (or empty) so the result can
// be pasted into another command.
func DisplayNames(children []*xbel.Node) []string {
	counts := map[string]int{}
	for _, c := range children {
		if isNamed(c) {
			counts[c.Title]++
		}
	}
	seen := map[string]int{}
	out := make([]string, len(children))
	for i, c := range children {
		switch {
		case isNamed(c):
			seen[c.Title]++
			out[i] = Escape(c.Title)
			if counts[c.Title] > 1 || c.Title == "" {
				out[i] += "[" + strconv.Itoa(seen[c.Title]) + "]"
			}
		case c.Kind == xbel.KindSeparator:
			out[i] = "---"
		case c.Kind == xbel.KindAlias:
			out[i] = "@" + c.Attr("ref")
		default:
			out[i] = "?"
		}
	}
	return out
}

// isNamed reports whether a node can be addressed by title.
func isNamed(n *xbel.Node) bool {
	return n.Kind == xbel.KindFolder || n.Kind == xbel.KindBookmark
}

// Entry is a resolved node together with its parent (nil for the root).
type Entry struct {
	Node   *xbel.Node
	Parent *xbel.Node
}

// lookup finds the child of parent matching seg.
func lookup(parent *xbel.Node, seg Segment) (*xbel.Node, error) {
	var matches []*xbel.Node
	for _, c := range parent.Children {
		if isNamed(c) && c.Title == seg.Name {
			matches = append(matches, c)
		}
	}
	switch {
	case len(matches) == 0:
		return nil, ErrNotFound
	case seg.Index > len(matches):
		return nil, fmt.Errorf("%w (only %d match %q)", ErrNotFound, len(matches), seg.Name)
	case seg.Index > 0:
		return matches[seg.Index-1], nil
	case len(matches) > 1:
		return nil, fmt.Errorf("%w: %d items are named %q; select one with %s[1] to %s[%d]",
			ErrAmbiguous, len(matches), seg.Name, Escape(seg.Name), Escape(seg.Name), len(matches))
	}
	return matches[0], nil
}

// Resolve finds the node at path. The empty path and "/" resolve to root.
func Resolve(root *xbel.Node, path string) (Entry, error) {
	segs, err := SplitPath(path)
	if err != nil {
		return Entry{}, fmt.Errorf("%s: %w", path, err)
	}
	return resolveSegs(root, segs, path)
}

func resolveSegs(root *xbel.Node, segs []Segment, display string) (Entry, error) {
	cur := Entry{Node: root}
	for _, seg := range segs {
		if !cur.Node.IsContainer() {
			return Entry{}, fmt.Errorf("%s: %w", display, ErrNotFolder)
		}
		child, err := lookup(cur.Node, seg)
		if err != nil {
			return Entry{}, fmt.Errorf("%s: %w", display, err)
		}
		cur = Entry{Node: child, Parent: cur.Node}
	}
	return cur, nil
}

// resolveFolder resolves path and requires it to be a folder (or the root).
func resolveFolder(root *xbel.Node, path string) (*xbel.Node, error) {
	e, err := Resolve(root, path)
	if err != nil {
		return nil, err
	}
	if !e.Node.IsContainer() {
		return nil, fmt.Errorf("%s: %w", path, ErrNotFolder)
	}
	return e.Node, nil
}
