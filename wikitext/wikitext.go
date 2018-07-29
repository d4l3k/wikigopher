package wikitext

import (
	"bytes"
	"log"
	"regexp"
	"strings"

	"github.com/pkg/errors"
	"golang.org/x/net/html"
)

//go:generate pigeon -o wikitext.peg.go wikitext.peg

// Convert converts wikitext to HTML.
func Convert(text []byte) ([]byte, error) {
	v, err := Parse(
		"file.wikitext",
		append(text, '\n'),
		GlobalStore("len", len(text)),
		GlobalStore("text", text),
		//Memoize(true),
		Recover(false),
		//Debug(true),
	)
	if err != nil {
		return nil, err
	}

	//spew.Dump(v)

	var doc *html.Node

	for doc == nil && v != nil {
		switch val := v.(type) {
		case *html.Node:
			doc = val
		case debugRun:
			v = val.Value
		}
	}

	if doc == nil {
		return nil, errors.Errorf("expected *html.Node got: %T", v)
	}

	var buf bytes.Buffer
	if err := html.Render(&buf, doc); err != nil {
		return nil, err
	}

	return bytes.TrimSpace(buf.Bytes()), nil
}

func concat(fields ...interface{}) string {
	var b strings.Builder
	for _, f := range fields {
		if f == nil {
			continue
		}

		switch f := f.(type) {
		case string:
			b.WriteString(f)

		case []interface{}:
			b.WriteString(concat(f...))

		case []byte:
			b.Write(f)

		default:
			panic(errors.Errorf("concat: unsupported f type %T: %+v", f, f))
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
	v, ok := c.state[tag].(int)
	if ok {
		v--
		c.state[tag] = v
	}
	return false, nil
}

func count(c *current, tag string) int {
	v, _ := c.state[tag].(int)
	return v
}

type stack []interface{}

func (s stack) Clone() interface{} {
	log.Printf("clone!")
	out := make(stack, len(s))
	for k, v := range s {
		if c, ok := v.(Cloner); ok {
			out[k] = c.Clone()
		} else {
			out[k] = v
		}
	}
	return out
}

var _ Cloner = stack{}

func push(c *current, tag string, val interface{}) int {
	v, _ := c.state[tag].(stack)
	v = append(v, val)
	c.state[tag] = v
	return len(v)
}

func pop(c *current, tag string) {
	v, _ := c.state[tag].(stack)
	if len(v) > 0 {
		c.state[tag] = v[:len(v)-1]
	}
}

func popTo(c *current, tag string, n int) {
	v, _ := c.state[tag].(stack)
	if len(v) > n {
		c.state[tag] = v[:n]
	}
}

func peek(c *current, tag string) interface{} {
	v, _ := c.state[tag].(stack)
	if len(v) == 0 {
		return nil
	}
	return v[len(v)-1]
}

var inlineBreaksRegexp = regexp.MustCompile(`[=|!{}:;\r\n[\]<\-]`)

func match(pattern string, input []byte) bool {
	match, err := regexp.Match(pattern, input)
	if err != nil {
		panic(err)
	}
	return match
}

func inlineBreaks(c *current) (bool, error) {
	input := c.globalStore["text"].([]byte)
	if len(input) <= c.pos.offset {
		return false, nil
	}
	pos := c.pos.offset
	ch := input[pos]
	if !inlineBreaksRegexp.Match([]byte{ch}) {
		return false, nil
	}

	switch ch {
	case '=':
		if arrow, _ := peek(c, "arrow").(bool); arrow && input[pos+1] == '>' {
			return true, nil
		}
		equal, _ := peek(c, "equal").(bool)
		return equal || (count(c, "h") > 0 && (pos == len(input)-1 ||
			// possibly more equals followed by spaces or comments
			//TODO: use match(`^=*(?:[ \t]|<\!--(?:(?!-->)[^])*-->)*(?:[\r\n]|$)`, input[pos+1:]))), nil
			match(`^=*(?:[ \t]|<\!--.*-->)*(?:[\r\n]|$)`, input[pos+1:]))), nil

	case '|':
		templateArg, _ := peek(c, "templateArg").(bool)
		extTag, _ := peek(c, "extTag").(bool)
		tableCellArg, _ := peek(c, "tableCellArg").(bool)
		linkdesc, _ := peek(c, "linkdesc").(bool)
		table, _ := peek(c, "table").(bool)

		return (templateArg &&
			!(extTag)) ||
			tableCellArg ||
			linkdesc ||
			(table && (pos < len(input)-1 &&
				match(`[}|]`, []byte{input[pos+1]}))), nil

	case '!':
		th, _ := peek(c, "th").(bool)
		return th &&
			count(c, "templatedepth") == 0 &&
			input[pos+1] == '!', nil

	case '{':
		// {{!}} pipe templates..
		// FIXME: Presumably these should mix with and match | above.
		tableCellArg, _ := peek(c, "tableCellArg").(bool)
		table, _ := peek(c, "table").(bool)
		return ((tableCellArg && string(input[pos:pos+5]) == "{{!}}") ||
			(table && string(input[pos:pos+10]) == "{{!}}{{!}}")), nil

	case '}':
		preproc, _ := peek(c, "preproc").(string)
		return string(input[pos:pos+2]) == preproc, nil

	case ':':
		return count(c, "colon") > 0 &&
			!peek(c, "extlink").(bool) &&
			count(c, "templatedepth") == 0 &&
			!peek(c, "linkdesc").(bool) &&
			!(peek(c, "preproc").(string) == "}-"), nil

	case ';':
		semicolon, _ := peek(c, "semicolon").(bool)
		return semicolon, nil

	case '\r':
		table, _ := peek(c, "table").(bool)
		return table && match(`\r\n?\s*[!|]`, input[pos:]), nil

	case '\n':
		// The code below is just a manual / efficient
		// version of this check.
		//
		// peek(c,'table') && /^\n\s*[!|]/.test(input.substr(pos));
		//
		// It eliminates a substr on the string and eliminates
		// a potential perf problem since "\n" and the inline_breaks
		// test is common during tokenization.
		if table, _ := peek(c, "table").(bool); !table {
			return false, nil
		}

		// Allow leading whitespace in tables

		// Since we switched on 'c' which is input[pos],
		// we know that input[pos] is "\n".
		// So, the /^\n/ part of the regexp is already satisfied.
		// Look for /\s*[!|]/ below.
		n := len(input)
		for i := pos + 1; i < n; i++ {
			d := input[i]
			if match(`[!|]`, []byte{d}) {
				return true, nil
			} else if !match(`\s`, []byte{d}) {
				return false, nil
			}
		}
		return false, nil

	case '[':
		// This is a special case in php's doTableStuff, added in
		// response to T2553.  If it encounters a `[[`, it bails on
		// parsing attributes and interprets it all as content.
		tableCellArg, _ := peek(c, "tableCellArg").(bool)
		return tableCellArg && string(input[pos:pos+2]) == "[[", nil

	case '-':
		// Same as above: a special case in doTableStuff, added
		// as part of T153140
		tableCellArg, _ := peek(c, "tableCellArg").(bool)
		return tableCellArg && string(input[pos:pos+2]) == "-{", nil

	case ']':
		extlink, _ := peek(c, "extlink").(bool)
		if extlink {
			return true, nil
		}
		preproc, _ := peek(c, "preproc").(string)
		return string(input[pos:pos+2]) == preproc, nil

	case '<':
		return (count(c, "noinclude") > 0 && string(input[pos:pos+12]) == "</noinclude>") ||
			(count(c, "includeonly") > 0 && string(input[pos:pos+14]) == "</includeonly>") ||
			(count(c, "onlyinclude") > 0 && string(input[pos:pos+14]) == "</onlyinclude>"), nil
	default:
		return false, errors.Errorf("Unhandled case!")
	}
}
