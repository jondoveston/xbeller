// Command xbeller lists and edits bookmarks in XBEL files.
package main

import (
	"os"

	"github.com/jondoveston/xbeller/internal/cli"
)

// Set at build time with -ldflags "-X main.version=...".
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	info := cli.BuildInfo{Version: version, Commit: commit, Date: date}
	os.Exit(cli.Execute(info, os.Args[0], os.Args[1:], os.Stdout, os.Stderr))
}
