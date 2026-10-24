package xbel

import (
	"bytes"
	"io"
	"strings"
)

const header = `<?xml version="1.0" encoding="UTF-8"?>` + "\n"

var (
	textEscaper = strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\r", "&#xD;",
	)
	attrEscaper = strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"\t", "&#x9;",
		"\n", "&#xA;",
		"\r", "&#xD;",
	)

	// Floccus escapes quotes in text as well as in attributes.
	floccusTextEscaper = strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`'`, "&apos;",
		`"`, "&quot;",
		"\r", "&#xD;",
	)
	floccusAttrEscaper = strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`'`, "&apos;",
		`"`, "&quot;",
		"\t", "&#x9;",
		"\n", "&#xA;",
		"\r", "&#xD;",
	)
)

// encoder writes XBEL in one of two layouts. The regular layout indents
// everything by two spaces. The Floccus layout reproduces, byte for byte,
// what Floccus itself writes (see createXBEL and XbelSerializer in the
// Floccus source), so that editing a Floccus file produces a minimal diff:
// the root's children start in column 0, quotes are escaped as entities in
// text, and there is no newline after </xbel>.
type encoder struct {
	b       bytes.Buffer
	floccus bool
}

func (e *encoder) escText(s string) string {
	if e.floccus {
		return floccusTextEscaper.Replace(s)
	}
	return textEscaper.Replace(s)
}

func (e *encoder) escAttr(s string) string {
	if e.floccus {
		return floccusAttrEscaper.Replace(s)
	}
	return attrEscaper.Replace(s)
}

// Encode writes the document, including the XML declaration and DOCTYPE.
func (d *Document) Encode(w io.Writer) error {
	e := &encoder{floccus: d.IsFloccus()}
	e.b.WriteString(header)
	if d.Directive != "" {
		e.b.WriteString("<!" + d.Directive + ">\n")
	}
	if e.floccus {
		e.floccusRoot(d.Root)
	} else {
		e.node(d.Root, 0)
	}
	_, err := w.Write(e.b.Bytes())
	return err
}

// WriteXML writes the node and its descendants as an indented XML fragment,
// without an XML declaration.
func (n *Node) WriteXML(w io.Writer) error {
	e := &encoder{}
	e.node(n, 0)
	_, err := w.Write(e.b.Bytes())
	return err
}

func indent(depth int) string { return strings.Repeat("  ", depth) }

func (e *encoder) attrs(attrs []Attr) {
	for _, a := range attrs {
		e.b.WriteString(" " + a.Name + `="` + e.escAttr(a.Value) + `"`)
	}
}

// floccusRoot writes the <xbel> element the way Floccus does: any comments
// at the start (the highestId marker), a blank line, then the children
// unindented, and no newline after the closing tag.
func (e *encoder) floccusRoot(n *Node) {
	e.b.WriteString("<xbel")
	e.attrs(n.Attrs)
	e.b.WriteString(">\n")
	// Floccus never writes these; keep them rather than lose data.
	if n.HasTitle {
		e.textElement("title", n.Title, 1)
	}
	if n.Info != nil {
		e.element(n.Info, 1)
	}
	if n.HasDesc {
		e.textElement("desc", n.Desc, 1)
	}
	i := 0
	for ; i < len(n.Children) && n.Children[i].Kind == KindComment; i++ {
		e.node(n.Children[i], 0)
	}
	e.b.WriteString("\n")
	for _, c := range n.Children[i:] {
		e.node(c, 0)
	}
	e.b.WriteString("</xbel>")
}

func (e *encoder) node(n *Node, depth int) {
	switch n.Kind {
	case KindUnknown:
		e.element(n.Raw, depth)
		return
	case KindComment:
		e.b.WriteString(indent(depth) + "<!--" + n.Text + "-->\n")
		return
	}
	ind, name := indent(depth), n.Kind.elementName()
	e.b.WriteString(ind + "<" + name)
	e.attrs(n.Attrs)
	if !n.HasTitle && n.Info == nil && !n.HasDesc && len(n.Children) == 0 {
		e.b.WriteString("/>\n")
		return
	}
	e.b.WriteString(">\n")
	if n.HasTitle {
		e.textElement("title", n.Title, depth+1)
	}
	if n.Info != nil {
		e.element(n.Info, depth+1)
	}
	if n.HasDesc {
		e.textElement("desc", n.Desc, depth+1)
	}
	for _, ch := range n.Children {
		e.node(ch, depth+1)
	}
	e.b.WriteString(ind + "</" + name + ">\n")
}

func (e *encoder) textElement(name, text string, depth int) {
	e.b.WriteString(indent(depth) + "<" + name)
	if text == "" {
		if e.floccus {
			e.b.WriteString("></" + name + ">\n")
		} else {
			e.b.WriteString("/>\n")
		}
		return
	}
	e.b.WriteString(">" + e.escText(text) + "</" + name + ">\n")
}

func (e *encoder) element(el *Element, depth int) {
	ind := indent(depth)
	e.b.WriteString(ind + "<" + el.Name)
	e.attrs(el.Attrs)
	switch {
	case len(el.Children) == 0 && el.Text == "":
		e.b.WriteString("/>\n")
	case len(el.Children) == 0:
		e.b.WriteString(">" + e.escText(el.Text) + "</" + el.Name + ">\n")
	default:
		e.b.WriteString(">\n")
		if el.Text != "" {
			e.b.WriteString(indent(depth+1) + e.escText(el.Text) + "\n")
		}
		for _, ch := range el.Children {
			e.element(ch, depth+1)
		}
		e.b.WriteString(ind + "</" + el.Name + ">\n")
	}
}
