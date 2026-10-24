// Package cli implements the xbeller command line interface.
package cli

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// BuildInfo describes the running binary. It is filled in at link time.
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

func (b BuildInfo) String() string {
	return fmt.Sprintf("%s (commit %s, built %s)", b.Version, b.Commit, b.Date)
}

type app struct {
	stdout, stderr io.Writer
	name           string // what the program was invoked as: xb or xbeller

	file    string
	dryRun  bool
	backup  bool
	verbose bool
}

const rootLong = `xbeller lists and edits bookmarks in XBEL files using familiar file commands.

The XBEL file is always the first argument. Every command is stateless and
refers to folders and bookmarks by their full path from the top of the file.

Paths:
  /Work/Docs/Spec    folders and bookmarks are matched by title
  /                  the top level of the file
  Name[2]            the second item titled Name (when titles repeat)
  A\/B               a title that contains a slash
  Name\[2]           a title that really ends in "[2]"
ls shows names in this form, so they can be copied into other commands.

Examples:
  xbeller bookmarks.xbel ls -l /Work
  xbeller bookmarks.xbel mkdir -p /Reading/2026
  xbeller bookmarks.xbel add /Reading/2026 "Go blog" https://go.dev/blog
  xbeller bookmarks.xbel mv /Work/Spec /Reading/2026
  xbeller bookmarks.xbel cat /Reading/2026 -o yaml
  xbeller bookmarks.xbel rm -r /Old`

// programName reduces argv[0] to one of the two names the binary is
// distributed under.
func programName(argv0 string) string {
	base := strings.TrimSuffix(filepath.Base(argv0), ".exe")
	if base == "xb" {
		return "xb"
	}
	return "xbeller"
}

func newRoot(a *app, name string, info BuildInfo) *cobra.Command {
	root := &cobra.Command{
		Use:     name,
		Short:   "List and edit bookmarks in XBEL files",
		Long:    rootLong,
		Version: info.String(),
		// Subcommand usage lines should read "xb <file> ls [path]".
		Annotations:   map[string]string{cobra.CommandDisplayNameAnnotation: name + " <file>"},
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
	}
	root.SetVersionTemplate("{{.Name}} {{.Version}}\n")
	root.SetOut(a.stdout)
	root.SetErr(a.stderr)
	root.CompletionOptions.DisableDefaultCmd = true

	pf := root.PersistentFlags()
	pf.BoolVarP(&a.dryRun, "dry-run", "n", false, "show what would change without writing the file")
	pf.BoolVar(&a.backup, "backup", false, "keep the original as <file>.bak before writing")
	pf.BoolVarP(&a.verbose, "verbose", "v", false, "describe each change made")

	root.AddCommand(
		a.lsCmd(),
		a.catCmd(),
		a.addCmd(),
		a.mkdirCmd(),
		a.cpCmd(),
		a.mvCmd(),
		a.rmCmd(),
	)
	return root
}

// extractFile pulls the leading positional <file> argument out of args. The
// first argument that is not a flag is the file unless it names a command
// (or "help"), in which case there is no file.
func extractFile(root *cobra.Command, args []string) (file string, rest []string, found bool) {
	known := map[string]bool{"help": true}
	for _, c := range root.Commands() {
		known[c.Name()] = true
	}
	for i, arg := range args {
		if arg == "--" {
			break
		}
		if strings.HasPrefix(arg, "-") {
			continue // every root-level flag is a boolean
		}
		if known[arg] {
			return "", args, false
		}
		rest = append(rest, args[:i]...)
		rest = append(rest, args[i+1:]...)
		return arg, rest, true
	}
	return "", args, false
}

// printf writes to standard output, reporting failures such as a closed pipe.
func (a *app) printf(format string, args ...any) error {
	_, err := fmt.Fprintf(a.stdout, format, args...)
	return err
}

var errNoFile = errors.New("no XBEL file given; usage: <file> <command> [args]")

// Execute runs the CLI and returns the process exit code. progName is
// os.Args[0] and decides whether the command calls itself xb or xbeller.
func Execute(info BuildInfo, progName string, args []string, stdout, stderr io.Writer) int {
	a := &app{stdout: stdout, stderr: stderr, name: programName(progName)}
	root := newRoot(a, a.name, info)

	file, rest, found := extractFile(root, args)
	a.file = file
	if found && len(rest) == 0 {
		_, _ = fmt.Fprintf(stderr, "%s: missing command after %q (usage: %s <file> <command>; try '%s help')\n",
			root.Name(), file, root.Name(), root.Name())
		return 2
	}
	root.SetArgs(rest)
	if err := root.Execute(); err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", root.Name(), err)
		return 1
	}
	return 0
}
