package wikitext

import (
	"golang.org/x/net/html"
)

func hasAttr(n *html.Node, key string) bool {
	for _, attr := range n.Attr {
		if attr.Key == key {
			return true
		}
	}
	return false
}

func removeAttr(n *html.Node, key string) {
	var attrs []html.Attribute
	for _, attr := range n.Attr {
		if attr.Key == key {
			continue
		}
		attrs = append(attrs, attr)
	}
	n.Attr = attrs
}

func processTokens(n *html.Node) []*html.Node {
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if hasAttr(child, "_parsestart") {
			removeAttr(child, "_parsestart")
			remaining := removeSiblingsAfter(child)
			//log.Printf("children: %q, %s", child.Data, spew.Sdump(remaining))
			addChildren(child, remaining)
		} else if hasAttr(child, "_parseend") {
			remaining := removeSiblingsAfter(child)
			child.Parent.RemoveChild(child)
			return remaining
		}
		addChildren(child.Parent, processTokens(child))
	}
	return nil
}

func removeSiblingsAfter(n *html.Node) []*html.Node {
	var children []*html.Node
	for child := n.NextSibling; child != nil; child = child.NextSibling {
		children = append(children, child)
	}
	parent := n.Parent
	for _, child := range children {
		parent.RemoveChild(child)
	}
	return children
}

func addChildren(n *html.Node, children []*html.Node) {
	for _, child := range children {
		n.AppendChild(child)
	}
}

func numChildren(n *html.Node) int {
	count := 0
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		count++
	}
	return count
}
