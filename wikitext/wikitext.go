package wikitext

import (
	"bytes"
	"log"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"golang.org/x/net/html"
)

//go:generate pigeon -o wikitext.peg.go wikitext.peg

// Convert converts wikitext to HTML.
func Convert(text []byte) ([]byte, error) {
	v, err := Parse(
		"file.wikitext",
		text,
		GlobalStore("len", len(text)),
		//Debug(true),
	)
	if err != nil {
		return nil, err
	}

	spew.Dump(v)

	var buf bytes.Buffer
	if err := html.Render(&buf, v.(*html.Node)); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func concat(fields ...interface{}) string {
	var b strings.Builder
	for _, f := range fields {
		switch f := f.(type) {
		case string:
			b.WriteString(f)

		case []interface{}:
			b.WriteString(concat(f...))

		case []byte:
			b.Write(f)

		default:
			log.Fatalf("concat: unsupported f type %T: %#v", f, f)
		}
	}
	return b.String()
}

func addChild(n *html.Node, children interface{}) {
	if children == nil {
		return
	}

	switch children := children.(type) {
	case []interface{}:
		for _, c := range children {
			addChild(n, c)
		}

	case *html.Node:
		n.AppendChild(children)

	case []byte:
		addChild(n, string(children))

	case string:
		addChild(n, &html.Node{
			Type: html.TextNode,
			Data: children,
		})

	default:
		log.Fatalf("unsupported children type %T: %#v", children, children)
	}
}

func inc(c *current, tag string) (bool, error) {
	v, _ := c.state[tag].(int)
	v++
	c.state[tag] = v
	return true, nil
}

func dec(c *current, tag string) (bool, error) {
	v, _ := c.state[tag].(int)
	if v == 0 {
		return false, nil
	}
	v--
	c.state[tag] = v
	return true, nil
}
