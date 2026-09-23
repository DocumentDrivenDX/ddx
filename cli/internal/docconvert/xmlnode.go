package docconvert

import "encoding/xml"

// xmlNode is a generic, order-preserving XML element tree. Office Open XML
// mixes many element types inside one shape, and the reading order of the
// original document is exactly the order the elements appear in, so the
// converters walk this tree rather than unmarshalling into typed structs.
type xmlNode struct {
	XMLName  xml.Name
	Attrs    []xml.Attr `xml:",any,attr"`
	Text     string     `xml:",chardata"`
	Children []xmlNode  `xml:",any"`
}

// parseXML decodes an OOXML part into a node tree.
func parseXML(data []byte) (*xmlNode, error) {
	var root xmlNode
	if err := xml.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	return &root, nil
}

// walk calls fn on this node and then on every descendant, in document order.
func (n *xmlNode) walk(fn func(*xmlNode)) {
	fn(n)
	for i := range n.Children {
		n.Children[i].walk(fn)
	}
}

// is reports whether the node is the named element in the given namespace.
func (n *xmlNode) is(space, local string) bool {
	return n.XMLName.Space == space && n.XMLName.Local == local
}

// descendants returns this node and every descendant matching the element
// name, in document order.
func (n *xmlNode) descendants(space, local string) []*xmlNode {
	var out []*xmlNode
	n.walk(func(node *xmlNode) {
		if node.is(space, local) {
			out = append(out, node)
		}
	})
	return out
}

// childElements returns the direct children matching the element name.
func (n *xmlNode) childElements(space, local string) []*xmlNode {
	var out []*xmlNode
	for i := range n.Children {
		if n.Children[i].is(space, local) {
			out = append(out, &n.Children[i])
		}
	}
	return out
}

// attr returns the value of the named attribute, ignoring its namespace.
func (n *xmlNode) attr(name string) string {
	for _, attr := range n.Attrs {
		if attr.Name.Local == name {
			return attr.Value
		}
	}
	return ""
}
