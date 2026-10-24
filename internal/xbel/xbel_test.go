package xbel

import (
	"bytes"
	"encoding/xml"
	"io"
	"os"
	"strings"
	"testing"
)

func readSample(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/sample.xbel")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestRoundTripIsLossless(t *testing.T) {
	in := readSample(t)
	doc, err := Parse(bytes.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := doc.Encode(&out); err != nil {
		t.Fatal(err)
	}
	if out.String() != string(in) {
		t.Errorf("round trip changed the document:\n--- want\n%s\n--- got\n%s", in, out.String())
	}
}

func TestParseStructure(t *testing.T) {
	doc, err := Parse(bytes.NewReader(readSample(t)))
	if err != nil {
		t.Fatal(err)
	}
	root := doc.Root
	if root.Kind != KindRoot || root.Title != "Bookmarks" || root.Attr("version") != "1.0" {
		t.Fatalf("unexpected root: %+v", root)
	}
	if root.Info == nil || root.Info.Children[0].Attrs[0].Value != "example" {
		t.Errorf("info block not preserved: %+v", root.Info)
	}
	if !strings.HasPrefix(doc.Directive, "DOCTYPE xbel") {
		t.Errorf("directive = %q", doc.Directive)
	}
	if len(root.Children) != 3 {
		t.Fatalf("root has %d children, want 3", len(root.Children))
	}
	bm := root.Children[0]
	if bm.Kind != KindBookmark || bm.Href() != "https://example.com/" || bm.Desc != "The example site & more" {
		t.Errorf("unexpected bookmark: %+v", bm)
	}
	work := root.Children[1]
	kinds := []Kind{KindBookmark, KindSeparator, KindFolder, KindAlias, KindUnknown}
	if len(work.Children) != len(kinds) {
		t.Fatalf("work has %d children, want %d", len(work.Children), len(kinds))
	}
	for i, k := range kinds {
		if work.Children[i].Kind != k {
			t.Errorf("work child %d kind = %v, want %v", i, work.Children[i].Kind, k)
		}
	}
}

func TestParseErrors(t *testing.T) {
	tests := map[string]string{
		"empty":         ``,
		"wrong root":    `<html></html>`,
		"unclosed":      `<xbel><folder><title>x</title></xbel>`,
		"mismatch":      `<xbel><folder></bookmark></xbel>`,
		"truncated":     `<xbel><folder>`,
		"two roots":     `<xbel/><xbel/>`,
		"not xml":       `hello`,
		"bad charset":   `<?xml version="1.0" encoding="ISO-8859-1"?><xbel/>`,
		"unclosed attr": `<xbel version="1.0>`,
	}
	for name, in := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse(strings.NewReader(in)); err == nil {
				t.Errorf("expected an error for %q", in)
			}
		})
	}
}

func TestEscaping(t *testing.T) {
	root := &Node{Kind: KindRoot, Attrs: []Attr{{"version", "1.0"}}}
	bm := NewBookmark("a <b> & \"c\"", "https://x/?a=1&b=\"2\"\n")
	bm.Desc, bm.HasDesc = "line1\nline2 & <tag>", true
	root.Children = append(root.Children, bm)

	var buf bytes.Buffer
	if err := (&Document{Root: root}).Encode(&buf); err != nil {
		t.Fatal(err)
	}
	doc, err := Parse(&buf)
	if err != nil {
		t.Fatalf("re-parse: %v\n%s", err, buf.String())
	}
	got := doc.Root.Children[0]
	if got.Title != bm.Title || got.Href() != bm.Href() || got.Desc != bm.Desc {
		t.Errorf("escaping did not round trip: %+v", got)
	}
}

func TestCloneIsDeep(t *testing.T) {
	doc, err := Parse(bytes.NewReader(readSample(t)))
	if err != nil {
		t.Fatal(err)
	}
	orig := doc.Root.Children[1]
	c := orig.Clone()
	c.Title = "changed"
	c.SetAttr("id", "other")
	c.Children[0].Title = "changed too"
	c.Children[2].Children[0].SetAttr("href", "x")
	c.Children[4].Raw.Attrs[0].Value = "x"

	if orig.Title != "Work" || orig.Attr("id") != "f1" ||
		orig.Children[0].Title != "Work A" ||
		orig.Children[2].Children[0].Href() != "https://deep.example.com/" ||
		orig.Children[4].Raw.Attrs[0].Value != "urn:vendor" {
		t.Error("modifying the clone changed the original")
	}
}

func TestAttrHelpers(t *testing.T) {
	n := NewBookmark("t", "u")
	n.SetAttr("id", "1")
	n.SetAttr("href", "v")
	if n.Href() != "v" || len(n.Attrs) != 2 {
		t.Errorf("SetAttr should replace in place: %+v", n.Attrs)
	}
	n.DelAttr("href")
	if _, ok := n.Lookup("href"); ok {
		t.Error("DelAttr did not remove the attribute")
	}
}

func TestContains(t *testing.T) {
	a, b := NewFolder("a"), NewFolder("b")
	a.Children = append(a.Children, b)
	if !a.Contains(b) || !a.Contains(a) || b.Contains(a) {
		t.Error("Contains gave the wrong answer")
	}
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func encode(t *testing.T, doc *Document) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := doc.Encode(&out); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// requireWellFormed fails unless a strict XML parser accepts data.
func requireWellFormed(t *testing.T, data []byte) {
	t.Helper()
	dec := xml.NewDecoder(bytes.NewReader(data))
	for {
		if _, err := dec.Token(); err == io.EOF {
			return
		} else if err != nil {
			t.Fatalf("output is not well-formed XML: %v", err)
		}
	}
}

func TestFloccusFileRoundTripsByteForByte(t *testing.T) {
	in := readFixture(t, "floccus.xbel")
	doc, err := Parse(bytes.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if !doc.IsFloccus() {
		t.Fatal("highestId comment not recognised")
	}
	if got := encode(t, doc); !bytes.Equal(got, in) {
		t.Errorf("round trip changed the file:\n--- want\n%s\n--- got\n%s", in, got)
	}
}

func TestSampleIsNotFloccus(t *testing.T) {
	doc, err := Parse(bytes.NewReader(readSample(t)))
	if err != nil {
		t.Fatal(err)
	}
	if doc.IsFloccus() {
		t.Error("sample has no highestId comment")
	}
}

func TestStripInvalid(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
		n    int
	}{
		{"clean", "plain text", "plain text", 0},
		{"allowed whitespace", "a\tb\nc\rd", "a\tb\nc\rd", 0},
		{"unicode untouched", "素人 ＿Big ñ \U0001F600", "素人 ＿Big ñ \U0001F600", 0},
		{"backspaces", "x\b\bJapanese", "xJapanese", 2},
		{"other controls", "\x00a\x0bb\x0cc\x1fd\x01", "abcd", 5},
		{"non-characters", "a\uFFFEb\uFFFFc", "abc", 2},
		{"replacement char kept", "a\uFFFDb", "a\uFFFDb", 0},
		{"at the edges", "\bx\b", "x", 2},
		{"only invalid", "\b\b", "", 2},
		{"empty", "", "", 0},
	}
	for _, tt := range tests {
		got, n := stripInvalid([]byte(tt.in))
		if string(got) != tt.want || n != tt.n {
			t.Errorf("%s: got %q, %d; want %q, %d", tt.name, got, n, tt.want, tt.n)
		}
	}
}

func TestParseStripsControlCharacters(t *testing.T) {
	in := bytes.Replace(readFixture(t, "floccus.xbel"),
		[]byte("<title>Deep</title>"), []byte("<title>De\b\bep</title>"), 1)
	// Sanity check: this is the failure the user saw from a strict parser.
	var strictErr error
	for dec := xml.NewDecoder(bytes.NewReader(in)); strictErr == nil; {
		if _, err := dec.Token(); err == io.EOF {
			break
		} else {
			strictErr = err
		}
	}
	if strictErr == nil || !strings.Contains(strictErr.Error(), "illegal character code U+0008") {
		t.Fatalf("the test input should be rejected by a strict parser, got %v", strictErr)
	}

	doc, err := Parse(bytes.NewReader(in))
	if err != nil {
		t.Fatalf("a control character must not make parsing fail: %v", err)
	}
	if doc.Stripped != 2 {
		t.Errorf("Stripped = %d, want 2", doc.Stripped)
	}
	deep := doc.Root.Children[1].Children[1].Children[0] // Toolbar > Nested > Deep
	if deep.Title != "Deep" {
		t.Errorf("title = %q, want Deep", deep.Title)
	}

	// What we write is valid XML again, and is the original document.
	out := encode(t, doc)
	requireWellFormed(t, out)
	if !bytes.Equal(out, readFixture(t, "floccus.xbel")) {
		t.Errorf("output differs from the clean fixture:\n%s", out)
	}
	again, err := Parse(bytes.NewReader(out))
	if err != nil || again.Stripped != 0 {
		t.Errorf("re-parse: %v, stripped %d", err, again.Stripped)
	}
}

func TestCommentsAreKept(t *testing.T) {
	in := `<?xml version="1.0" encoding="UTF-8"?>
<xbel version="1.0">
  <!-- top -->
  <folder>
    <title>A</title>
    <!-- inside -->
    <bookmark href="https://x/">
      <title>x</title>
    </bookmark>
  </folder>
</xbel>
`
	doc, err := Parse(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	root := doc.Root
	if len(root.Children) != 2 || root.Children[0].Kind != KindComment || root.Children[0].Text != " top " {
		t.Fatalf("root children: %+v", root.Children)
	}
	folder := root.Children[1]
	if len(folder.Children) != 2 || folder.Children[0].Kind != KindComment || !folder.Children[0].Hidden() {
		t.Fatalf("folder children: %+v", folder.Children)
	}
	if got := string(encode(t, doc)); got != in {
		t.Errorf("comments not preserved:\n%s", got)
	}
}

func TestHighestID(t *testing.T) {
	doc, err := Parse(bytes.NewReader(readFixture(t, "floccus.xbel")))
	if err != nil {
		t.Fatal(err)
	}
	c, id, ok := FindHighestID(doc.Root)
	if !ok || id != 9 {
		t.Fatalf("FindHighestID = %v, %d, %v", c, id, ok)
	}
	SetHighestID(c, 1234)
	want := "- highestId :1234: for Floccus bookmark sync browser extension "
	if c.Text != want {
		t.Errorf("text = %q, want %q", c.Text, want)
	}
	if _, id, _ := FindHighestID(doc.Root); id != 1234 {
		t.Errorf("id = %d after SetHighestID", id)
	}

	for _, text := range []string{" highestId :5: x", "- highestId :x: y", "- highestId :5", "unrelated"} {
		n := &Node{Kind: KindRoot, Children: []*Node{{Kind: KindComment, Text: text}}}
		if _, _, ok := FindHighestID(n); ok {
			t.Errorf("%q should not be recognised", text)
		}
	}
}

func TestFloccusLayoutWithEmptyRoot(t *testing.T) {
	in := "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<xbel version=\"1.0\">\n" +
		"<!--- highestId :0: for Floccus bookmark sync browser extension -->\n\n</xbel>"
	doc, err := Parse(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(encode(t, doc)); got != in {
		t.Errorf("got %q, want %q", got, in)
	}
}
