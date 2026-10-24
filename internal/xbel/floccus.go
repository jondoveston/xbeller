package xbel

import (
	"regexp"
	"strconv"
)

// Floccus (https://floccus.org) syncs bookmarks through an XBEL file. It
// keeps a counter for the next bookmark id in a comment near the top:
//
//	<!--- highestId :53547: for Floccus bookmark sync browser extension -->
//
// and refuses files in which a bookmark or folder lacks an integer id. The
// comment is how xbeller recognises such a file.
var highestIDRe = regexp.MustCompile(`(?s)^(- highestId :)([0-9]+)(:.*)$`)

// FindHighestID looks for Floccus's highestId comment among the children of
// root and returns the comment node and the number in it.
func FindHighestID(root *Node) (comment *Node, id int, ok bool) {
	for _, c := range root.Children {
		if c.Kind != KindComment {
			continue
		}
		m := highestIDRe.FindStringSubmatch(c.Text)
		if m == nil {
			continue
		}
		n, err := strconv.Atoi(m[2])
		if err != nil {
			continue
		}
		return c, n, true
	}
	return nil, 0, false
}

// SetHighestID rewrites the number in a highestId comment found by
// FindHighestID.
func SetHighestID(comment *Node, id int) {
	comment.Text = highestIDRe.ReplaceAllString(comment.Text, "${1}"+strconv.Itoa(id)+"${3}")
}

// IsFloccus reports whether the document carries Floccus's highestId marker.
// Such files are written in the layout Floccus itself uses.
func (d *Document) IsFloccus() bool {
	_, _, ok := FindHighestID(d.Root)
	return ok
}
