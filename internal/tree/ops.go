package tree

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"time"

	"github.com/jondoveston/xbeller/internal/xbel"
)

// Now and NewID are variables so tests can make results deterministic.
var (
	Now   = time.Now
	NewID = func() string {
		var b [6]byte
		if _, err := rand.Read(b[:]); err != nil {
			panic(err)
		}
		return "xb" + hex.EncodeToString(b[:])
	}
)

func timestamp() string { return Now().UTC().Format(time.RFC3339) }

// Add creates a bookmark in the folder at folderPath.
func Add(root *xbel.Node, folderPath, title, href, desc string) ([]string, error) {
	folder, err := resolveFolder(root, folderPath)
	if err != nil {
		return nil, err
	}
	u, err := url.Parse(href)
	if err != nil || href == "" {
		return nil, fmt.Errorf("invalid URL %q", href)
	}
	bm := xbel.NewBookmark(title, u.String())
	if desc != "" {
		bm.Desc, bm.HasDesc = desc, true
	}
	ids := newIDAlloc(root)
	ids.stamp(bm)
	folder.Children = append(folder.Children, bm)
	ids.finish()
	return []string{fmt.Sprintf("add %q to %s", title, displayPath(folderPath))}, nil
}

// Mkdir creates the folder at path. With parents set, missing parent folders
// are created too and an existing folder is not an error.
func Mkdir(root *xbel.Node, path string, parents bool) ([]string, error) {
	segs, err := SplitPath(path)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if len(segs) == 0 {
		return nil, fmt.Errorf("%s: the root folder %w", path, ErrExists)
	}
	ids := newIDAlloc(root)
	var log []string
	cur := root
	for i, seg := range segs {
		last := i == len(segs)-1
		child, err := lookup(cur, seg)
		switch {
		case err == nil:
			if last && !parents {
				return nil, fmt.Errorf("%s: %w", path, ErrExists)
			}
			if !child.IsContainer() {
				return nil, fmt.Errorf("%s: %w", path, ErrNotFolder)
			}
		case errors.Is(err, ErrNotFound) && seg.Index == 0 && (last || parents):
			child = xbel.NewFolder(seg.Name)
			ids.stamp(child)
			cur.Children = append(cur.Children, child)
			log = append(log, "mkdir "+displayPath(joinSegs(segs[:i+1])))
		case errors.Is(err, ErrNotFound) && seg.Index == 0:
			return nil, fmt.Errorf("%s: parent folder %s: %w (use -p to create it)", path, joinSegs(segs[:i]), ErrNotFound)
		default:
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		cur = child
	}
	ids.finish()
	return log, nil
}

// Copy copies each source into dest. If dest is not an existing folder it
// must be a new name inside an existing folder, and then exactly one source
// is allowed.
func Copy(root *xbel.Node, srcs []string, dest string, recursive bool) ([]string, error) {
	entries, _, err := resolveSources(root, srcs, "copy", false)
	if err != nil {
		return nil, err
	}
	folder, newName, rename, err := resolveDest(root, dest, len(entries))
	if err != nil {
		return nil, err
	}
	for i, e := range entries {
		if e.Node.Kind == xbel.KindFolder && !recursive {
			return nil, fmt.Errorf("%s: is a folder (use -r to copy it)", srcs[i])
		}
		if e.Node.Contains(folder) {
			return nil, fmt.Errorf("%s: cannot copy a folder into itself", srcs[i])
		}
	}
	ids := newIDAlloc(root)
	var log []string
	for i, e := range entries {
		c := e.Node.Clone()
		ids.renumber(c)
		if rename {
			c.Title = newName
		}
		folder.Children = append(folder.Children, c)
		log = append(log, fmt.Sprintf("cp %s -> %s", srcs[i], displayPath(dest)))
	}
	ids.finish()
	return log, nil
}

// Move moves each source into dest, or renames a single source if dest is a
// new name. Moved nodes keep their attributes, including id.
func Move(root *xbel.Node, srcs []string, dest string) ([]string, error) {
	entries, srcs, err := resolveSources(root, srcs, "move", true)
	if err != nil {
		return nil, err
	}
	folder, newName, rename, err := resolveDest(root, dest, len(entries))
	if err != nil {
		return nil, err
	}
	for i, e := range entries {
		if e.Node.Contains(folder) {
			return nil, fmt.Errorf("%s: cannot move a folder into itself", srcs[i])
		}
	}
	var log []string
	for i, e := range entries {
		switch {
		case rename && e.Parent == folder:
			e.Node.Title = newName // plain rename: keep its position
		case rename:
			detach(e.Parent, e.Node)
			e.Node.Title = newName
			folder.Children = append(folder.Children, e.Node)
		case e.Parent == folder:
			continue // already there
		default:
			detach(e.Parent, e.Node)
			folder.Children = append(folder.Children, e.Node)
		}
		log = append(log, fmt.Sprintf("mv %s -> %s", srcs[i], displayPath(dest)))
	}
	return log, nil
}

// Remove deletes each path. Folders need recursive set.
func Remove(root *xbel.Node, paths []string, recursive bool) ([]string, error) {
	entries, paths, err := resolveSources(root, paths, "remove", true)
	if err != nil {
		return nil, err
	}
	for i, e := range entries {
		if e.Node.Kind == xbel.KindFolder && !recursive {
			return nil, fmt.Errorf("%s: is a folder (use -r to remove it)", paths[i])
		}
	}
	var log []string
	for i, e := range entries {
		detach(e.Parent, e.Node)
		log = append(log, "rm "+paths[i])
	}
	return log, nil
}

// RemoveMatching removes the bookmarks whose URL host matches globs. Each path
// names a folder to search (one level only, subfolders are left alone) or a
// single bookmark. The root folder is allowed. Folders are never removed.
func RemoveMatching(root *xbel.Node, paths []string, globs HostGlobs) ([]string, error) {
	if err := globs.Validate(); err != nil {
		return nil, err
	}
	type hit struct {
		parent, node *xbel.Node
		path         string
	}
	var hits []hit
	seen := map[*xbel.Node]bool{}
	for _, p := range paths {
		e, err := Resolve(root, p)
		if err != nil {
			return nil, err
		}
		if !e.Node.IsContainer() {
			if e.Node.Kind == xbel.KindBookmark && !seen[e.Node] && globs.MatchURL(e.Node.Href()) {
				seen[e.Node] = true
				hits = append(hits, hit{e.Parent, e.Node, p})
			}
			continue
		}
		names := DisplayNames(e.Node.Children)
		for i, c := range e.Node.Children {
			if c.Kind == xbel.KindBookmark && !seen[c] && globs.MatchURL(c.Href()) {
				seen[c] = true
				hits = append(hits, hit{e.Node, c, Join(p, names[i])})
			}
		}
	}
	var log []string
	for _, h := range hits {
		detach(h.parent, h.node)
		log = append(log, "rm "+displayPath(h.path))
	}
	return log, nil
}

// resolveSources resolves every path before anything is changed, since later
// paths with [n] selectors would shift after an earlier removal. It returns
// the entries together with the paths they came from. If unique is set, a
// node named more than once is only returned the first time.
func resolveSources(root *xbel.Node, paths []string, verb string, unique bool) ([]Entry, []string, error) {
	entries := make([]Entry, 0, len(paths))
	kept := make([]string, 0, len(paths))
	seen := map[*xbel.Node]bool{}
	for _, p := range paths {
		e, err := Resolve(root, p)
		if err != nil {
			return nil, nil, err
		}
		if e.Parent == nil {
			return nil, nil, fmt.Errorf("%s: cannot %s the root folder", p, verb)
		}
		if unique && seen[e.Node] {
			continue
		}
		seen[e.Node] = true
		entries = append(entries, e)
		kept = append(kept, p)
	}
	return entries, kept, nil
}

// resolveDest interprets dest the way cp and mv do: an existing folder means
// "put it inside", while a missing last component inside an existing folder
// means "give it this name".
func resolveDest(root *xbel.Node, dest string, nsrc int) (folder *xbel.Node, name string, rename bool, err error) {
	e, err := Resolve(root, dest)
	if err == nil {
		if !e.Node.IsContainer() {
			return nil, "", false, fmt.Errorf("%s: destination %w", dest, ErrExists)
		}
		return e.Node, "", false, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return nil, "", false, err
	}
	segs, _ := SplitPath(dest) // cannot fail: Resolve parsed it already
	last := segs[len(segs)-1]
	if last.Index != 0 {
		return nil, "", false, fmt.Errorf("%s: cannot use an index selector in a new name", dest)
	}
	if nsrc > 1 {
		return nil, "", false, fmt.Errorf("%s: %w (needed as the target for several sources)", dest, ErrNotFound)
	}
	parent, err := resolveSegs(root, segs[:len(segs)-1], dest)
	if err != nil {
		return nil, "", false, err
	}
	if !parent.Node.IsContainer() {
		return nil, "", false, fmt.Errorf("%s: %w", dest, ErrNotFolder)
	}
	return parent.Node, last.Name, true, nil
}

func detach(parent, n *xbel.Node) {
	if i := slices.Index(parent.Children, n); i >= 0 {
		parent.Children = slices.Delete(parent.Children, i, i+1)
	}
}

func joinSegs(segs []Segment) string {
	p := ""
	for _, s := range segs {
		p = Join(p, Escape(s.Name))
	}
	if p == "" {
		return "/"
	}
	return p
}

func displayPath(p string) string {
	if p == "" {
		return "/"
	}
	return p
}
