// Package xbel implements a lossless in-memory model of XBEL 1.0 bookmark
// files, along with a parser and serializer.
//
// The model keeps attributes in document order, preserves <info> blocks and
// unknown elements, and remembers the DOCTYPE, so that parsing and then
// writing a file does not lose data.
package xbel

// Kind identifies what sort of XBEL element a Node represents.
type Kind int

const (
	// KindRoot is the top-level <xbel> element. It behaves like a folder.
	KindRoot Kind = iota
	KindFolder
	KindBookmark
	KindSeparator
	KindAlias
	// KindUnknown is an element xbeller does not understand. It is carried
	// through untouched in Node.Raw.
	KindUnknown
	// KindComment is an XML comment inside the root or a folder. Its text is
	// in Node.Text. Comments are kept so that, for example, Floccus's
	// highestId marker survives an edit.
	KindComment
)

func (k Kind) String() string {
	switch k {
	case KindRoot:
		return "root"
	case KindFolder:
		return "folder"
	case KindBookmark:
		return "bookmark"
	case KindSeparator:
		return "separator"
	case KindAlias:
		return "alias"
	case KindComment:
		return "comment"
	default:
		return "unknown"
	}
}

// elementName returns the XML element name for the kind.
func (k Kind) elementName() string {
	if k == KindRoot {
		return "xbel"
	}
	return k.String()
}

// Attr is a single XML attribute. Attributes are kept in document order.
type Attr struct {
	Name  string
	Value string
}

// Element is a generic XML element, used for <info> blocks and for elements
// that XBEL does not define.
type Element struct {
	Name     string
	Attrs    []Attr
	Text     string
	Children []*Element
}

// Clone returns a deep copy of the element.
func (e *Element) Clone() *Element {
	if e == nil {
		return nil
	}
	c := &Element{
		Name:  e.Name,
		Attrs: append([]Attr(nil), e.Attrs...),
		Text:  e.Text,
	}
	for _, ch := range e.Children {
		c.Children = append(c.Children, ch.Clone())
	}
	return c
}

// Node is one element of an XBEL tree: the root, a folder, a bookmark, a
// separator, an alias, or an unknown element.
type Node struct {
	Kind  Kind
	Attrs []Attr

	// Title and Desc are the text of the <title> and <desc> children.
	// HasTitle and HasDesc record whether the element is present at all, so
	// that an absent element is not turned into an empty one on write.
	Title    string
	HasTitle bool
	Desc     string
	HasDesc  bool

	// Info is the <info> block, if any.
	Info *Element

	// Children holds nested folders, bookmarks, aliases and separators for
	// containers. Unknown elements found inside any node are kept here too.
	Children []*Node

	// Raw is set when Kind is KindUnknown.
	Raw *Element

	// Text is the comment text (without "<!--" and "-->") when Kind is
	// KindComment.
	Text string
}

// Hidden reports whether the node is bookkeeping (an unknown element or a
// comment) that listings should not show as a bookmark, folder or separator.
func (n *Node) Hidden() bool {
	return n.Kind == KindUnknown || n.Kind == KindComment
}

// NewFolder returns an empty folder with the given title.
func NewFolder(title string) *Node {
	return &Node{Kind: KindFolder, Title: title, HasTitle: true}
}

// NewBookmark returns a bookmark with the given title and URL.
func NewBookmark(title, href string) *Node {
	n := &Node{Kind: KindBookmark, Title: title, HasTitle: true}
	n.SetAttr("href", href)
	return n
}

// IsContainer reports whether the node can hold bookmarks and folders.
func (n *Node) IsContainer() bool {
	return n.Kind == KindRoot || n.Kind == KindFolder
}

// Lookup returns the value of the named attribute and whether it exists.
func (n *Node) Lookup(name string) (string, bool) {
	for _, a := range n.Attrs {
		if a.Name == name {
			return a.Value, true
		}
	}
	return "", false
}

// Attr returns the value of the named attribute, or "" if it is absent.
func (n *Node) Attr(name string) string {
	v, _ := n.Lookup(name)
	return v
}

// SetAttr sets the named attribute, replacing an existing value in place or
// appending a new attribute.
func (n *Node) SetAttr(name, value string) {
	for i := range n.Attrs {
		if n.Attrs[i].Name == name {
			n.Attrs[i].Value = value
			return
		}
	}
	n.Attrs = append(n.Attrs, Attr{Name: name, Value: value})
}

// DelAttr removes the named attribute if present.
func (n *Node) DelAttr(name string) {
	for i := range n.Attrs {
		if n.Attrs[i].Name == name {
			n.Attrs = append(n.Attrs[:i], n.Attrs[i+1:]...)
			return
		}
	}
}

// Href returns the URL of a bookmark.
func (n *Node) Href() string { return n.Attr("href") }

// Clone returns a deep copy of the node and everything below it.
func (n *Node) Clone() *Node {
	c := *n
	c.Attrs = append([]Attr(nil), n.Attrs...)
	c.Info = n.Info.Clone()
	c.Raw = n.Raw.Clone()
	c.Children = nil
	for _, ch := range n.Children {
		c.Children = append(c.Children, ch.Clone())
	}
	return &c
}

// Walk calls fn for n and every descendant, depth first, in document order.
func (n *Node) Walk(fn func(*Node)) {
	fn(n)
	for _, ch := range n.Children {
		ch.Walk(fn)
	}
}

// Contains reports whether other is n or one of its descendants.
func (n *Node) Contains(other *Node) bool {
	if n == other {
		return true
	}
	for _, ch := range n.Children {
		if ch.Contains(other) {
			return true
		}
	}
	return false
}

// Document is a parsed XBEL file.
type Document struct {
	// Directive is the text of the <!DOCTYPE ...> declaration without the
	// surrounding "<!" and ">", or "" if the file had none.
	Directive string
	Root      *Node

	// Stripped is how many characters that XML 1.0 forbids (such as the
	// backspace control character) were removed while parsing. Browsers and
	// sync tools write these into titles; they cannot be represented in
	// well-formed XML, so they are not written back.
	Stripped int
}
