package xbel

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Parse reads an XBEL document. Only UTF-8 input is supported.
//
// Real-world files are not always well-formed: bookmark titles scraped from
// web pages can contain control characters such as backspace, which XML 1.0
// forbids and Go's decoder rejects. Parse removes those characters first and
// reports how many in Document.Stripped.
func Parse(r io.Reader) (*Document, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("reading xbel: %w", err)
	}
	data, stripped := stripInvalid(data)

	dec := xml.NewDecoder(bytes.NewReader(data))
	doc := &Document{Stripped: stripped}
	for {
		tok, err := dec.RawToken()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parsing xbel: %w", err)
		}
		switch t := tok.(type) {
		case xml.Directive:
			if doc.Root == nil {
				doc.Directive = string(t)
			}
		case xml.StartElement:
			if doc.Root != nil {
				return nil, errors.New("parsing xbel: multiple root elements")
			}
			if rawName(t.Name) != "xbel" {
				return nil, fmt.Errorf("parsing xbel: root element is <%s>, want <xbel>", rawName(t.Name))
			}
			root, err := parseNode(dec, t, KindRoot)
			if err != nil {
				return nil, fmt.Errorf("parsing xbel: %w", err)
			}
			doc.Root = root
		}
	}
	if doc.Root == nil {
		return nil, errors.New("parsing xbel: no <xbel> element found")
	}
	return doc, nil
}

// stripInvalid removes characters that are not allowed in XML 1.0: the C0
// control characters other than tab, line feed and carriage return, and the
// non-characters U+FFFE and U+FFFF. All of them are recognisable byte by byte
// in UTF-8 (continuation bytes are always 0x80 or above), so valid text is
// never damaged. It returns the input itself when there is nothing to strip.
func stripInvalid(data []byte) (clean []byte, count int) {
	var out []byte
	start := 0
	for i := 0; i < len(data); {
		n := invalidLen(data[i:])
		if n == 0 {
			i++
			continue
		}
		if out == nil {
			out = make([]byte, 0, len(data))
		}
		out = append(out, data[start:i]...)
		i += n
		start = i
		count++
	}
	if out == nil {
		return data, 0
	}
	return append(out, data[start:]...), count
}

// invalidLen returns the length of the forbidden character at the start of
// b, or 0 if it starts with an allowed one.
func invalidLen(b []byte) int {
	switch c := b[0]; {
	case c < 0x20 && c != '\t' && c != '\n' && c != '\r':
		return 1
	case c == 0xEF && len(b) >= 3 && b[1] == 0xBF && (b[2] == 0xBE || b[2] == 0xBF):
		return 3 // U+FFFE, U+FFFF
	}
	return 0
}

// rawName rebuilds "prefix:local" from a name produced by RawToken, which
// does not resolve namespaces.
func rawName(n xml.Name) string {
	if n.Space != "" {
		return n.Space + ":" + n.Local
	}
	return n.Local
}

func convertAttrs(in []xml.Attr) []Attr {
	if len(in) == 0 {
		return nil
	}
	out := make([]Attr, len(in))
	for i, a := range in {
		out[i] = Attr{Name: rawName(a.Name), Value: a.Value}
	}
	return out
}

// parseNode parses the body of an XBEL element whose start tag has already
// been read, consuming up to and including its end tag.
func parseNode(dec *xml.Decoder, start xml.StartElement, kind Kind) (*Node, error) {
	n := &Node{Kind: kind, Attrs: convertAttrs(start.Attr)}
	name := rawName(start.Name)
	for {
		tok, err := dec.RawToken()
		if err != nil {
			return nil, unexpectedEOF(err)
		}
		switch t := tok.(type) {
		case xml.EndElement:
			if rawName(t.Name) != name {
				return nil, fmt.Errorf("element <%s> closed by </%s>", name, rawName(t.Name))
			}
			return n, nil
		case xml.StartElement:
			if err := parseChild(dec, n, t); err != nil {
				return nil, err
			}
		case xml.Comment:
			n.Children = append(n.Children, &Node{Kind: KindComment, Text: string(t)})
		}
	}
}

func parseChild(dec *xml.Decoder, parent *Node, t xml.StartElement) error {
	switch name := rawName(t.Name); {
	case name == "title":
		el, err := parseElement(dec, t)
		if err != nil {
			return err
		}
		parent.Title, parent.HasTitle = el.Text, true
	case name == "desc":
		el, err := parseElement(dec, t)
		if err != nil {
			return err
		}
		parent.Desc, parent.HasDesc = el.Text, true
	case name == "info":
		el, err := parseElement(dec, t)
		if err != nil {
			return err
		}
		parent.Info = el
	case name == "folder" && parent.IsContainer():
		return parseInto(dec, parent, t, KindFolder)
	case name == "bookmark" && parent.IsContainer():
		return parseInto(dec, parent, t, KindBookmark)
	case name == "separator" && parent.IsContainer():
		return parseInto(dec, parent, t, KindSeparator)
	case name == "alias" && parent.IsContainer():
		return parseInto(dec, parent, t, KindAlias)
	default:
		el, err := parseElement(dec, t)
		if err != nil {
			return err
		}
		parent.Children = append(parent.Children, &Node{Kind: KindUnknown, Raw: el})
	}
	return nil
}

func parseInto(dec *xml.Decoder, parent *Node, t xml.StartElement, kind Kind) error {
	child, err := parseNode(dec, t, kind)
	if err != nil {
		return err
	}
	parent.Children = append(parent.Children, child)
	return nil
}

// parseElement parses a generic element whose start tag has been read.
func parseElement(dec *xml.Decoder, start xml.StartElement) (*Element, error) {
	el := &Element{Name: rawName(start.Name), Attrs: convertAttrs(start.Attr)}
	var text strings.Builder
	for {
		tok, err := dec.RawToken()
		if err != nil {
			return nil, unexpectedEOF(err)
		}
		switch t := tok.(type) {
		case xml.CharData:
			text.Write(t)
		case xml.StartElement:
			child, err := parseElement(dec, t)
			if err != nil {
				return nil, err
			}
			el.Children = append(el.Children, child)
		case xml.EndElement:
			if rawName(t.Name) != el.Name {
				return nil, fmt.Errorf("element <%s> closed by </%s>", el.Name, rawName(t.Name))
			}
			if len(el.Children) > 0 {
				// Whitespace between child elements is just formatting.
				el.Text = strings.TrimSpace(text.String())
			} else {
				el.Text = text.String()
			}
			return el, nil
		}
	}
}

func unexpectedEOF(err error) error {
	if err == io.EOF {
		return io.ErrUnexpectedEOF
	}
	return err
}
