package cli

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/jondoveston/xbeller/internal/tree"
	"github.com/jondoveston/xbeller/internal/xbel"
	"github.com/spf13/cobra"
)

// urlFilter selects and cleans the URLs printed by ls -u.
type urlFilter struct {
	hosts   tree.HostGlobs // list only these hosts (all if empty)
	noQuery tree.HostGlobs // strip query and fragment for these hosts
}

// apply returns the URL to print, or false if it should not be listed.
func (f urlFilter) apply(href string) (string, bool) {
	if href == "" {
		return "", false
	}
	if len(f.hosts) > 0 && !f.hosts.MatchURL(href) {
		return "", false
	}
	if f.noQuery.MatchURL(href) {
		href = tree.StripQuery(href)
	}
	return href, true
}

// listURLs prints the URL of each bookmark in dir, descending into
// subfolders if recursive.
func (a *app) listURLs(dir *xbel.Node, recursive bool, f urlFilter) error {
	for _, c := range dir.Children {
		switch {
		case c.Hidden():
		case c.Kind == xbel.KindBookmark:
			if u, ok := f.apply(c.Href()); ok {
				if err := a.printf("%s\n", u); err != nil {
					return err
				}
			}
		case c.Kind == xbel.KindFolder && recursive:
			if err := a.listURLs(c, recursive, f); err != nil {
				return err
			}
		}
	}
	return nil
}

func (a *app) lsCmd() *cobra.Command {
	var long, recursive, urls bool
	var hosts, noQuery []string
	cmd := &cobra.Command{
		Use:   "ls [path]",
		Short: "List the contents of a folder",
		Long: `List the contents of a folder (the top level by default). Folders are shown
with a trailing slash. Names that are shared by several items carry a [n]
selector so they can be used in other commands.

With -l each line shows the type (d folder, b bookmark, - separator,
@ alias), the date added, the name, and the URL or number of items.

With -u only the URL of each bookmark is printed, one per line. With -R this
is one flat list for the whole subtree. Two options refine it:

  --host GLOB      list only bookmarks whose host matches
  --no-query GLOB  remove the query string and fragment from URLs whose
                   host matches ('*' matches every host)

Both may be repeated and are independent of each other. Patterns use glob
syntax (* ? [abc]) and are matched against the host alone, without port,
ignoring case: example.com does not match www.example.com, so give
'*.example.com' as well. '*' also matches dots.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			f := urlFilter{hosts: hosts, noQuery: noQuery}
			if !urls && (len(hosts) > 0 || len(noQuery) > 0) {
				return errors.New("--host and --no-query need -u")
			}
			if err := f.hosts.Validate(); err != nil {
				return err
			}
			if err := f.noQuery.Validate(); err != nil {
				return err
			}
			path := "/"
			if len(args) == 1 {
				path = args[0]
			}
			doc, err := a.load()
			if err != nil {
				return err
			}
			e, err := tree.Resolve(doc.Root, path)
			if err != nil {
				return err
			}
			if urls {
				if e.Node.Kind == xbel.KindBookmark {
					if u, ok := f.apply(e.Node.Href()); ok {
						return a.printf("%s\n", u)
					}
					return nil
				}
				if !e.Node.IsContainer() {
					return nil
				}
				return a.listURLs(e.Node, recursive, f)
			}
			if !e.Node.IsContainer() {
				// Like ls on a file: show just that entry.
				names := tree.DisplayNames(e.Parent.Children)
				i := slices.Index(e.Parent.Children, e.Node)
				return a.listEntries([]*xbel.Node{e.Node}, names[i:i+1], long)
			}
			first := true
			return a.listFolder(e.Node, path, long, recursive, &first)
		},
	}
	cmd.Flags().BoolVarP(&long, "long", "l", false, "use a long listing format")
	cmd.Flags().BoolVarP(&recursive, "recursive", "R", false, "list subfolders recursively")
	cmd.Flags().BoolVarP(&urls, "urls", "u", false, "print only bookmark URLs, one per line")
	cmd.Flags().StringArrayVar(&hosts, "host", nil, "with -u, list only URLs whose host matches `GLOB` (repeatable)")
	cmd.Flags().StringArrayVar(&noQuery, "no-query", nil, "with -u, remove query and fragment from URLs whose host matches `GLOB` (repeatable)")
	cmd.MarkFlagsMutuallyExclusive("long", "urls")
	return cmd
}

// listFolder prints one folder, then its subfolders if recursive.
func (a *app) listFolder(dir *xbel.Node, path string, long, recursive bool, first *bool) error {
	var shown []*xbel.Node
	for _, c := range dir.Children {
		if !c.Hidden() {
			shown = append(shown, c)
		}
	}
	names := tree.DisplayNames(shown)
	if recursive {
		if !*first {
			if err := a.printf("\n"); err != nil {
				return err
			}
		}
		if err := a.printf("%s:\n", displayRoot(path)); err != nil {
			return err
		}
	}
	*first = false
	if err := a.listEntries(shown, names, long); err != nil {
		return err
	}
	if !recursive {
		return nil
	}
	for i, c := range shown {
		if c.Kind == xbel.KindFolder {
			if err := a.listFolder(c, tree.Join(path, names[i]), long, recursive, first); err != nil {
				return err
			}
		}
	}
	return nil
}

func displayRoot(path string) string {
	if segs, err := tree.SplitPath(path); err == nil && len(segs) == 0 {
		return "/"
	}
	return path
}

func (a *app) listEntries(nodes []*xbel.Node, names []string, long bool) error {
	if !long {
		for i, n := range nodes {
			name := names[i]
			if n.Kind == xbel.KindFolder {
				name += "/"
			}
			if err := a.printf("%s\n", name); err != nil {
				return err
			}
		}
		return nil
	}
	var buf bytes.Buffer
	tw := tabwriter.NewWriter(&buf, 0, 4, 2, ' ', 0)
	for i, n := range nodes {
		typ, detail := "-", ""
		name := names[i]
		switch n.Kind {
		case xbel.KindFolder:
			typ, name = "d", name+"/"
			detail = fmt.Sprintf("%d items", countItems(n))
		case xbel.KindBookmark:
			typ, detail = "b", n.Href()
		case xbel.KindAlias:
			typ = "@"
		}
		added := "-"
		if d := n.Attr("added"); d != "" {
			added = d[:min(len(d), 10)]
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", typ, added, name, detail) // writes to a buffer
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	// Rows without a detail column would otherwise end in padding.
	for _, line := range strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n") {
		if err := a.printf("%s\n", strings.TrimRight(line, " ")); err != nil {
			return err
		}
	}
	return nil
}

func countItems(n *xbel.Node) int {
	count := 0
	for _, c := range n.Children {
		if !c.Hidden() {
			count++
		}
	}
	return count
}

func (a *app) catCmd() *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "cat [path]",
		Short: "Print a bookmark or folder as JSON, YAML or XML",
		Long: `Print the bookmark or folder at path (the whole file by default). Folders
include everything below them.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "/"
			if len(args) == 1 {
				path = args[0]
			}
			doc, err := a.load()
			if err != nil {
				return err
			}
			e, err := tree.Resolve(doc.Root, path)
			if err != nil {
				return err
			}
			return a.printNode(e.Node, format)
		},
	}
	cmd.Flags().StringVarP(&format, "output", "o", "json", "output format: json, yaml or xml")
	return cmd
}

func (a *app) addCmd() *cobra.Command {
	var desc string
	cmd := &cobra.Command{
		Use:   "add <folder> <title> <url>",
		Short: "Add a bookmark to a folder",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.mutate(func(root *xbel.Node) ([]string, error) {
				return tree.Add(root, args[0], args[1], args[2], desc)
			})
		},
	}
	cmd.Flags().StringVarP(&desc, "desc", "d", "", "description for the bookmark")
	return cmd
}

func (a *app) mkdirCmd() *cobra.Command {
	var parents bool
	cmd := &cobra.Command{
		Use:   "mkdir [-p] <path>...",
		Short: "Create folders",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.mutate(func(root *xbel.Node) ([]string, error) {
				var all []string
				for _, p := range args {
					log, err := tree.Mkdir(root, p, parents)
					if err != nil {
						return nil, err
					}
					all = append(all, log...)
				}
				return all, nil
			})
		},
	}
	cmd.Flags().BoolVarP(&parents, "parents", "p", false, "create missing parent folders; no error if it exists")
	return cmd
}

func (a *app) cpCmd() *cobra.Command {
	var recursive bool
	cmd := &cobra.Command{
		Use:   "cp [-r] <source>... <dest>",
		Short: "Copy bookmarks or folders",
		Long: `Copy each source into dest. If dest is an existing folder the sources are
placed inside it; otherwise dest is a new name inside an existing folder
(one source only). Copies get new ids; everything else is kept.`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			srcs, dest := args[:len(args)-1], args[len(args)-1]
			return a.mutate(func(root *xbel.Node) ([]string, error) {
				return tree.Copy(root, srcs, dest, recursive)
			})
		},
	}
	cmd.Flags().BoolVarP(&recursive, "recursive", "r", false, "copy folders and their contents")
	return cmd
}

func (a *app) mvCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mv <source>... <dest>",
		Short: "Move or rename bookmarks or folders",
		Long: `Move each source into dest. If dest is an existing folder the sources are
moved inside it; otherwise dest is a new name inside an existing folder
(one source only), which also renames. Moved items keep their ids.`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			srcs, dest := args[:len(args)-1], args[len(args)-1]
			return a.mutate(func(root *xbel.Node) ([]string, error) {
				return tree.Move(root, srcs, dest)
			})
		},
	}
}

func (a *app) rmCmd() *cobra.Command {
	var recursive bool
	var hosts []string
	cmd := &cobra.Command{
		Use:   "rm [-r] [--host GLOB] <path>...",
		Short: "Remove bookmarks or folders",
		Long: `Remove bookmarks or folders.

With --host, each path is a folder to search (or a single bookmark) and only
the bookmarks directly inside it whose host matches are removed. Subfolders
are not searched and no folder is removed. This selects exactly the
bookmarks that 'ls -u --host' lists, with the same glob rules, so a list can
be exported and then removed. --host cannot be combined with -r.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(hosts) > 0 {
				if recursive {
					return errors.New("--host only works on the folder given; -r is for removing folders")
				}
				return a.mutate(func(root *xbel.Node) ([]string, error) {
					return tree.RemoveMatching(root, args, hosts)
				})
			}
			return a.mutate(func(root *xbel.Node) ([]string, error) {
				return tree.Remove(root, args, recursive)
			})
		},
	}
	cmd.Flags().BoolVarP(&recursive, "recursive", "r", false, "remove folders and their contents")
	cmd.Flags().StringArrayVar(&hosts, "host", nil, "remove only bookmarks whose host matches `GLOB` (repeatable)")
	return cmd
}
