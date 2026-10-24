package cli

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/jondoveston/xbeller/internal/tree"
	"github.com/jondoveston/xbeller/internal/xbel"
	"github.com/spf13/cobra"
)

func (a *app) lsCmd() *cobra.Command {
	var long, recursive bool
	cmd := &cobra.Command{
		Use:   "ls [path]",
		Short: "List the contents of a folder",
		Long: `List the contents of a folder (the top level by default). Folders are shown
with a trailing slash. Names that are shared by several items carry a [n]
selector so they can be used in other commands.

With -l each line shows the type (d folder, b bookmark, - separator,
@ alias), the date added, the name, and the URL or number of items.`,
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
	cmd := &cobra.Command{
		Use:   "rm [-r] <path>...",
		Short: "Remove bookmarks or folders",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.mutate(func(root *xbel.Node) ([]string, error) {
				return tree.Remove(root, args, recursive)
			})
		},
	}
	cmd.Flags().BoolVarP(&recursive, "recursive", "r", false, "remove folders and their contents")
	return cmd
}
