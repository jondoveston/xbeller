package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/jondoveston/xbeller/internal/xbel"
)

func (a *app) load() (*xbel.Document, error) {
	if a.file == "" {
		return nil, errNoFile
	}
	f, err := os.Open(a.file)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }() // read-only: nothing to lose
	doc, err := xbel.Parse(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", a.file, err)
	}
	return doc, nil
}

// mutate loads the file, applies fn, and writes the result back. fn returns
// a description of each change; if it returns none, the file is left alone.
func (a *app) mutate(fn func(root *xbel.Node) ([]string, error)) error {
	doc, err := a.load()
	if err != nil {
		return err
	}
	changes, err := fn(doc.Root)
	if err != nil {
		return err
	}
	if len(changes) == 0 {
		return nil
	}
	if doc.Stripped > 0 {
		verb := "removed"
		if a.dryRun {
			verb = "would remove"
		}
		_, _ = fmt.Fprintf(a.stderr, "%s: %s %d character(s) that XML 1.0 does not allow (control characters) from %s\n",
			a.name, verb, doc.Stripped, a.file)
	}
	if a.dryRun {
		for _, c := range changes {
			if err := a.printf("dry-run: %s\n", c); err != nil {
				return err
			}
		}
		return nil
	}
	if err := writeFile(a.file, doc, a.backup); err != nil {
		return err
	}
	if a.verbose {
		for _, c := range changes {
			if err := a.printf("%s\n", c); err != nil {
				return err
			}
		}
	}
	return nil
}

// writeFile replaces path with the encoded document. The new content is
// written to a temporary file in the same directory and renamed into place,
// so an interrupted write never leaves a half-written bookmarks file.
func writeFile(path string, doc *xbel.Document, backup bool) (err error) {
	target, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	info, err := os.Stat(target)
	if err != nil {
		return err
	}
	if backup {
		data, err := os.ReadFile(target)
		if err != nil {
			return err
		}
		if err := os.WriteFile(target+".bak", data, info.Mode().Perm()); err != nil {
			return fmt.Errorf("writing backup: %w", err)
		}
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), "."+filepath.Base(target)+".tmp-*")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
		}
	}()
	if err = doc.Encode(tmp); err != nil {
		return err
	}
	if err = tmp.Chmod(info.Mode().Perm()); err != nil {
		return err
	}
	if err = tmp.Sync(); err != nil {
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), target)
}
