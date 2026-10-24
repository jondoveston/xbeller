package cli

import (
	"encoding/json"
	"fmt"

	"github.com/jondoveston/xbeller/internal/xbel"
	"gopkg.in/yaml.v3"
)

// view is the JSON/YAML representation of a node printed by cat.
type view struct {
	Type        string  `json:"type" yaml:"type"`
	Title       string  `json:"title,omitempty" yaml:"title,omitempty"`
	URL         string  `json:"url,omitempty" yaml:"url,omitempty"`
	Ref         string  `json:"ref,omitempty" yaml:"ref,omitempty"`
	ID          string  `json:"id,omitempty" yaml:"id,omitempty"`
	Added       string  `json:"added,omitempty" yaml:"added,omitempty"`
	Modified    string  `json:"modified,omitempty" yaml:"modified,omitempty"`
	Visited     string  `json:"visited,omitempty" yaml:"visited,omitempty"`
	Description string  `json:"description,omitempty" yaml:"description,omitempty"`
	Children    []*view `json:"children,omitempty" yaml:"children,omitempty"`
}

func toView(n *xbel.Node) *view {
	v := &view{
		Type:        n.Kind.String(),
		Title:       n.Title,
		URL:         n.Href(),
		Ref:         n.Attr("ref"),
		ID:          n.Attr("id"),
		Added:       n.Attr("added"),
		Modified:    n.Attr("modified"),
		Visited:     n.Attr("visited"),
		Description: n.Desc,
	}
	for _, c := range n.Children {
		if !c.Hidden() {
			v.Children = append(v.Children, toView(c))
		}
	}
	return v
}

func (a *app) printNode(n *xbel.Node, format string) error {
	switch format {
	case "json":
		enc := json.NewEncoder(a.stdout)
		enc.SetEscapeHTML(false) // keep & in URLs readable
		enc.SetIndent("", "  ")
		return enc.Encode(toView(n))
	case "yaml":
		enc := yaml.NewEncoder(a.stdout)
		enc.SetIndent(2)
		if err := enc.Encode(toView(n)); err != nil {
			return err
		}
		return enc.Close()
	case "xml":
		return n.WriteXML(a.stdout)
	default:
		return fmt.Errorf("unknown output format %q (want json, yaml or xml)", format)
	}
}
