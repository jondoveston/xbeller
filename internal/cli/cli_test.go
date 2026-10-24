package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

var testInfo = BuildInfo{Version: "1.2.3", Commit: "abc1234", Date: "2026-01-02T03:04:05Z"}

// sample copies the shared test document into a temp dir.
func sample(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("../xbel/testdata/sample.xbel")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "bookmarks.xbel")
	if err := os.WriteFile(path, data, 0o640); err != nil {
		t.Fatal(err)
	}
	return path
}

func run(t *testing.T, prog string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = Execute(testInfo, prog, args, &out, &errOut)
	return out.String(), errOut.String(), code
}

func mustRun(t *testing.T, args ...string) string {
	t.Helper()
	out, errOut, code := run(t, "xbeller", args...)
	if code != 0 {
		t.Fatalf("xbeller %v failed (%d): %s", args, code, errOut)
	}
	return out
}

func TestVersion(t *testing.T) {
	out, _, code := run(t, "/usr/local/bin/xbeller", "--version")
	if code != 0 || out != "xbeller 1.2.3 (commit abc1234, built 2026-01-02T03:04:05Z)\n" {
		t.Errorf("got %q (%d)", out, code)
	}
	out, _, _ = run(t, "xb", "--version")
	if !strings.HasPrefix(out, "xb 1.2.3") {
		t.Errorf("invoked as xb, got %q", out)
	}
	out, _, _ = run(t, "something-else", "--version")
	if !strings.HasPrefix(out, "xbeller ") {
		t.Errorf("unknown name should fall back to xbeller, got %q", out)
	}
}

func TestHelpMentionsFile(t *testing.T) {
	out, _, code := run(t, "xb", "help", "mv")
	if code != 0 || !strings.Contains(out, "xb <file> mv <source>... <dest>") {
		t.Errorf("usage line should include <file>:\n%s", out)
	}
}

func TestFileArgumentHandling(t *testing.T) {
	path := sample(t)
	// Flags may come before the file, between, or after the command.
	for _, args := range [][]string{
		{path, "ls"},
		{"-v", path, "ls"},
		{path, "-v", "ls"},
		{path, "ls", "-v"},
	} {
		if out := mustRun(t, args...); out != "Example\nWork/\nDupes/\n" {
			t.Errorf("%v: got %q", args, out)
		}
	}
	if _, errOut, code := run(t, "xbeller", path); code == 0 || !strings.Contains(errOut, "missing command") {
		t.Errorf("file with no command: %d %q", code, errOut)
	}
	if _, errOut, code := run(t, "xbeller", "ls"); code == 0 || !strings.Contains(errOut, "no XBEL file") {
		t.Errorf("command with no file: %d %q", code, errOut)
	}
	if _, errOut, code := run(t, "xbeller", filepath.Join(t.TempDir(), "missing.xbel"), "ls"); code == 0 || !strings.Contains(errOut, "missing.xbel") {
		t.Errorf("missing file: %d %q", code, errOut)
	}
	bad := filepath.Join(t.TempDir(), "bad.xbel")
	if err := os.WriteFile(bad, []byte("not xml"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, errOut, code := run(t, "xbeller", bad, "ls"); code == 0 || !strings.Contains(errOut, "bad.xbel") {
		t.Errorf("bad file: %d %q", code, errOut)
	}
}

func TestLs(t *testing.T) {
	path := sample(t)
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"ls"}, "Example\nWork/\nDupes/\n"},
		{[]string{"ls", "/"}, "Example\nWork/\nDupes/\n"},
		{[]string{"ls", "/Work"}, "Work A\n---\nDeep/\n@b1\n"},
		{[]string{"ls", "/Dupes"}, "Same[1]\nSame[2]\nSame[3]/\n"},
		{[]string{"ls", "/Example"}, "Example\n"},
		{[]string{"ls", "/Dupes/Same[2]"}, "Same[2]\n"},
		{[]string{"ls", "-l", "/Work"}, "" +
			"b  -  Work A  https://example.com/a?x=1&y=2\n" +
			"-  -  ---\n" +
			"d  -  Deep/   1 items\n" +
			"@  -  @b1\n"},
		{[]string{"ls", "-l"}, "" +
			"b  2024-01-02  Example  https://example.com/\n" +
			"d  -           Work/    4 items\n" +
			"d  -           Dupes/   3 items\n"},
		{[]string{"ls", "-R", "/Work"}, "/Work:\nWork A\n---\nDeep/\n@b1\n\n/Work/Deep:\nDeep link\n"},
	}
	for _, tt := range tests {
		got := mustRun(t, append([]string{path}, tt.args...)...)
		if got != tt.want {
			t.Errorf("%v:\n got %q\nwant %q", tt.args, got, tt.want)
		}
	}
	if _, errOut, code := run(t, "xbeller", path, "ls", "/Dupes/Same"); code == 0 || !strings.Contains(errOut, "ambiguous") {
		t.Errorf("ambiguous ls: %d %q", code, errOut)
	}
}

func TestCat(t *testing.T) {
	path := sample(t)

	var v view
	if err := json.Unmarshal([]byte(mustRun(t, path, "cat", "/Work/Deep")), &v); err != nil {
		t.Fatal(err)
	}
	if v.Type != "folder" || v.Title != "Deep" || len(v.Children) != 1 || v.Children[0].URL != "https://deep.example.com/" {
		t.Errorf("json: %+v", v)
	}

	var y view
	if err := yaml.Unmarshal([]byte(mustRun(t, path, "cat", "/Work/Deep", "-o", "yaml")), &y); err != nil {
		t.Fatal(err)
	}
	if y.Title != "Deep" || len(y.Children) != 1 {
		t.Errorf("yaml: %+v", y)
	}

	out := mustRun(t, path, "cat", "/Example", "--output", "xml")
	want := "<bookmark href=\"https://example.com/\" id=\"b1\" added=\"2024-01-02T03:04:05Z\">\n  <title>Example</title>\n  <desc>The example site &amp; more</desc>\n</bookmark>\n"
	if out != want {
		t.Errorf("xml:\n got %q\nwant %q", out, want)
	}

	// URLs must not be HTML-escaped in JSON.
	if out := mustRun(t, path, "cat", "/Work/Work A"); !strings.Contains(out, "x=1&y=2") {
		t.Errorf("json escaped the URL: %s", out)
	}
	// No path means the whole file.
	if err := json.Unmarshal([]byte(mustRun(t, path, "cat")), &v); err != nil || v.Type != "root" || len(v.Children) != 3 {
		t.Errorf("whole file: %v %+v", err, v)
	}
	if _, errOut, code := run(t, "xbeller", path, "cat", "-o", "toml"); code == 0 || !strings.Contains(errOut, "toml") {
		t.Errorf("bad format: %d %q", code, errOut)
	}
}

func TestEditingCommands(t *testing.T) {
	path := sample(t)

	mustRun(t, path, "mkdir", "-p", "/Reading/2026")
	mustRun(t, path, "add", "/Reading/2026", "Go blog", "https://go.dev/blog", "--desc", "news")
	mustRun(t, path, "cp", "/Reading/2026/Go blog", "/Reading")
	mustRun(t, path, "mv", "/Example", "/Reading/Renamed")
	mustRun(t, path, "cp", "-r", "/Work", "/Reading/Work2")
	mustRun(t, path, "rm", "/Dupes/Same[1]", "/Dupes/Same[2]")

	if got := mustRun(t, path, "ls", "/Reading"); got != "2026/\nGo blog\nRenamed\nWork2/\n" {
		t.Errorf("/Reading = %q", got)
	}
	if got := mustRun(t, path, "ls", "/Dupes"); got != "Same/\n" {
		t.Errorf("/Dupes = %q", got)
	}
	if got := mustRun(t, path, "ls", "/"); got != "Work/\nDupes/\nReading/\n" {
		t.Errorf("/ = %q", got)
	}

	// The edited file still parses, keeps the DOCTYPE and unknown elements,
	// and keeps its permissions.
	data, _ := os.ReadFile(path)
	for _, want := range []string{"<!DOCTYPE xbel", `<vendor:thing xmlns:vendor="urn:vendor"`, `id="b1"`} {
		if !strings.Contains(string(data), want) {
			t.Errorf("edited file lost %q", want)
		}
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o640 {
		t.Errorf("mode = %v, want 0640", info.Mode().Perm())
	}
	if matches, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".*tmp*")); len(matches) != 0 {
		t.Errorf("temp files left behind: %v", matches)
	}
}

func TestFailedEditLeavesFileAlone(t *testing.T) {
	path := sample(t)
	before, _ := os.ReadFile(path)
	// The first path is valid, the second is not: nothing may be applied.
	if _, _, code := run(t, "xbeller", path, "rm", "/Example", "/Nope"); code == 0 {
		t.Fatal("expected failure")
	}
	if _, _, code := run(t, "xbeller", path, "rm", "/Work"); code == 0 {
		t.Fatal("expected failure")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Error("file changed after failed commands")
	}
}

func TestDryRunAndVerbose(t *testing.T) {
	path := sample(t)
	before, _ := os.ReadFile(path)

	out := mustRun(t, path, "--dry-run", "mv", "/Example", "/Work")
	if out != "dry-run: mv /Example -> /Work\n" {
		t.Errorf("dry-run output %q", out)
	}
	if after, _ := os.ReadFile(path); !bytes.Equal(before, after) {
		t.Error("dry run modified the file")
	}

	if out := mustRun(t, path, "mv", "/Example", "/Work"); out != "" {
		t.Errorf("should be quiet by default, got %q", out)
	}
	if out := mustRun(t, path, "-v", "mv", "/Work/Example", "/Dupes"); out != "mv /Work/Example -> /Dupes\n" {
		t.Errorf("verbose output %q", out)
	}
}

func TestNoOpDoesNotRewriteFile(t *testing.T) {
	path := sample(t)
	// Put the file in a non-canonical layout; a no-op must not reformat it.
	odd := []byte(`<xbel version="1.0"><folder><title>A</title></folder></xbel>`)
	if err := os.WriteFile(path, odd, 0o644); err != nil {
		t.Fatal(err)
	}
	mustRun(t, path, "mkdir", "-p", "/A")
	if got, _ := os.ReadFile(path); !bytes.Equal(got, odd) {
		t.Errorf("no-op rewrote the file: %s", got)
	}
}

func TestBackup(t *testing.T) {
	path := sample(t)
	before, _ := os.ReadFile(path)
	mustRun(t, path, "--backup", "rm", "/Example")
	bak, err := os.ReadFile(path + ".bak")
	if err != nil || !bytes.Equal(bak, before) {
		t.Errorf("backup missing or different: %v", err)
	}
	if after, _ := os.ReadFile(path); bytes.Equal(after, before) {
		t.Error("original was not changed")
	}
}

func TestSymlinkedFileIsEditedInPlace(t *testing.T) {
	path := sample(t)
	link := filepath.Join(t.TempDir(), "link.xbel")
	if err := os.Symlink(path, link); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	mustRun(t, link, "rm", "/Example")
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Error("symlink was replaced by a regular file")
	}
	if got := mustRun(t, path, "ls"); strings.Contains(got, "Example") {
		t.Errorf("target not edited: %q", got)
	}
}

// floccusFile copies the Floccus-style fixture into a temp dir.
func floccusFile(t *testing.T, mutate func([]byte) []byte) string {
	t.Helper()
	data, err := os.ReadFile("../xbel/testdata/floccus.xbel")
	if err != nil {
		t.Fatal(err)
	}
	if mutate != nil {
		data = mutate(data)
	}
	path := filepath.Join(t.TempDir(), "floccus.xbel")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestFloccusFileEditsAreMinimal(t *testing.T) {
	path := floccusFile(t, nil)
	before, _ := os.ReadFile(path)

	mustRun(t, path, "add", "/Toolbar", "T", "https://x")

	// The only differences are the new bookmark and the bumped counter, and
	// the file keeps Floccus's layout, down to having no final newline.
	want := strings.Replace(string(before), "highestId :9:", "highestId :10:", 1)
	want = strings.Replace(want, "  </folder>\n</folder>", "  </folder>\n"+
		"  <bookmark href=\"https://x\" id=\"10\">\n    <title>T</title>\n  </bookmark>\n</folder>", 1)
	got, _ := os.ReadFile(path)
	if string(got) != want {
		t.Errorf("unexpected edit result:\n--- want\n%s\n--- got\n%s", want, got)
	}
}

func TestFloccusFileCommands(t *testing.T) {
	path := floccusFile(t, nil)

	// The marker comment is invisible to listings.
	if got := mustRun(t, path, "ls"); got != "Toolbar/\n[1]\n" {
		t.Errorf("ls = %q", got)
	}
	if got := mustRun(t, path, "ls", "-l", "/"); !strings.Contains(got, "Toolbar/") || !strings.Contains(got, "3 items") {
		t.Errorf("ls -l = %q", got)
	}
	var v view
	if err := json.Unmarshal([]byte(mustRun(t, path, "cat")), &v); err != nil || len(v.Children) != 2 {
		t.Errorf("cat: %v, %d children (comment must not be listed)", err, len(v.Children))
	}

	mustRun(t, path, "mkdir", "/Inbox")
	mustRun(t, path, "cp", "-r", "/Toolbar/Nested", "/Inbox")
	mustRun(t, path, "mv", "/Inbox/Nested", "/Inbox/Renamed")
	data, _ := os.ReadFile(path)
	text := string(data)
	for _, want := range []string{
		"highestId :12:", // Inbox 10, copied Nested 11, copied Deep 12
		`<folder id="10">`,
		`<folder id="11">`,
		`id="12"`,
		"<title>Renamed</title>",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("file lacks %q:\n%s", want, text)
		}
	}
	if strings.HasSuffix(text, "\n") || !strings.HasSuffix(text, "</xbel>") {
		t.Error("Floccus files end without a newline")
	}
	if strings.Contains(text, "added=") {
		t.Error("added attributes should not be written to Floccus files")
	}
}

func TestControlCharactersAreStrippedOnWrite(t *testing.T) {
	path := floccusFile(t, func(b []byte) []byte {
		return bytes.Replace(b, []byte("<title>Deep</title>"), []byte("<title>De\b\bep</title>"), 1)
	})
	before, _ := os.ReadFile(path)

	// Reading works, quietly, and shows clean titles.
	out, errOut, code := run(t, "xbeller", path, "ls", "/Toolbar/Nested")
	if code != 0 || out != "Deep\n" || errOut != "" {
		t.Errorf("ls: %q %q %d", out, errOut, code)
	}

	// A dry run says what it would do and changes nothing.
	_, errOut, code = run(t, "xbeller", path, "--dry-run", "mkdir", "/New")
	if code != 0 || !strings.Contains(errOut, "would remove 2 character(s)") {
		t.Errorf("dry run stderr: %q (%d)", errOut, code)
	}
	if after, _ := os.ReadFile(path); !bytes.Equal(before, after) {
		t.Error("dry run changed the file")
	}

	// Writing reports it and produces a file any XML parser accepts.
	_, errOut, code = run(t, "xb", path, "mkdir", "/New")
	if code != 0 || !strings.HasPrefix(errOut, "xb: removed 2 character(s)") {
		t.Errorf("stderr: %q (%d)", errOut, code)
	}
	data, _ := os.ReadFile(path)
	if bytes.IndexByte(data, '\b') >= 0 {
		t.Error("backspace still in the output")
	}
	if out := mustRun(t, path, "ls", "/Toolbar/Nested"); out != "Deep\n" {
		t.Errorf("after write: %q", out)
	}
	// ... and is quiet from then on.
	if _, errOut, _ := run(t, "xbeller", path, "mkdir", "/New2"); errOut != "" {
		t.Errorf("second write should not warn: %q", errOut)
	}
}

func TestNoOpDoesNotMentionStripping(t *testing.T) {
	path := floccusFile(t, func(b []byte) []byte {
		return bytes.Replace(b, []byte("Deep"), []byte("De\bep"), 1)
	})
	before, _ := os.ReadFile(path)
	_, errOut, _ := run(t, "xbeller", path, "mkdir", "-p", "/Toolbar")
	if errOut != "" {
		t.Errorf("nothing was written, so nothing should be reported: %q", errOut)
	}
	if after, _ := os.ReadFile(path); !bytes.Equal(before, after) {
		t.Error("no-op rewrote the file")
	}
}
