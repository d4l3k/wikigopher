package wikitext

import (
	"bytes"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"

	"github.com/microcosm-cc/bluemonday"
	"github.com/pkg/errors"
	"golang.org/x/net/html"
)

//go:generate pigeon -o wikitext.peg.go wikitext.peg

// Convert converts wikitext to HTML.
func Convert(text []byte, options ...ConvertOption) ([]byte, error) {
	var opts opts
	for _, opt := range options {
		opt(&opts)
	}
	v, err := Parse(
		"file.wikitext",
		append(text, '\n'),
		GlobalStore("len", len(text)),
		GlobalStore("text", text),
		GlobalStore("opts", opts),
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

	//log.Printf("Token doc: %q", concat(doc))

	remaining := processTokens(doc)
	if opts.strict && len(remaining) > 0 {
		return nil, errors.Errorf("got %d extra children: doc %q, children %q", len(remaining), concat(doc), concat(remaining))
	}
	addChildren(doc, remaining)

	var buf bytes.Buffer
	if err := html.Render(&buf, doc); err != nil {
		return nil, err
	}

	body := buf.Bytes()
	body = wikitextPolicy().SanitizeBytes(body)
	body = bytes.TrimSpace(body)
	return body, nil
}

func wikitextPolicy() *bluemonday.Policy {
	policy := bluemonday.UGCPolicy()

	policy.AllowNoAttrs().OnElements("ref")

	policy.RequireNoFollowOnLinks(false)
	policy.RequireNoFollowOnFullyQualifiedLinks(true)
	policy.AllowStyling()
	policy.AllowAttrs("id", "name", "style").Globally()
	policy.AllowAttrs("_parsestart", "_parseend", "_parsetoken").Globally()

	return policy
}

type Attribute struct {
	Key, Val interface{}
}

func (a Attribute) String() string {
	if a.Val == nil {
		return concat(a.Key)
	}
	return fmt.Sprintf("%s=%s", concat(a.Key), concat(a.Val))
}

type opts struct {
	templateHandler func(name string, attrs []Attribute) (interface{}, error)
	strict          bool
}

type ConvertOption func(opts *opts)

// TemplateHandler sets the function that runs when a template is found. The
// return value is included in the final document. Either *html.Node or string
// values may be returned. String values will be inserted as escaped text.
func TemplateHandler(f func(name string, attrs []Attribute) (interface{}, error)) ConvertOption {
	return func(opts *opts) {
		opts.templateHandler = f
	}
}

func strict() ConvertOption {
	return func(opts *opts) {
		opts.strict = true
	}
}

func flatten(fields ...interface{}) []interface{} {
	var out []interface{}
	for _, f := range fields {
		if f == nil {
			continue
		}

		switch f := f.(type) {
		case []interface{}:
			out = append(out, flatten(f...)...)
		case []*html.Node:
			for _, n := range f {
				out = append(out, n)
			}

		default:
			out = append(out, f)
		}
	}
	return out
}

func Concat(fields ...interface{}) string {
	return concat(fields...)
}

func concat(fields ...interface{}) string {
	var b strings.Builder
	for _, f := range flatten(fields...) {
		if f == nil {
			continue
		}

		switch f := f.(type) {
		case int:
			b.WriteString(strconv.Itoa(f))

		case string:
			b.WriteString(f)

		case []byte:
			b.Write(f)

		case *html.Node:
			var buf bytes.Buffer
			if err := html.Render(&buf, f); err != nil {
				panic(err)
			}
			b.Write(buf.Bytes())

		case debugRun:
			b.WriteString(concat(f.Value))

		case Attribute:
			b.WriteString(f.String())

		default:
			panic(errors.Errorf("concat: unsupported f type %T: %+v", f, f))
		}
	}
	return b.String()
}

func addChild(n *html.Node, children interface{}) bool {
	if children == nil {
		return false
	}

	switch children := children.(type) {
	case []interface{}:
		added := false
		for _, c := range children {
			if addChild(n, c) {
				added = true
			}
		}
		return added

	case *html.Node:
		n.AppendChild(children)
		return true

	case []byte:
		return addChild(n, string(children))

	case string:
		return addChild(n, &html.Node{
			Type: html.TextNode,
			Data: children,
		})

	default:
		log.Fatalf("unsupported children type %T: %#v", children, children)
		return false
	}
}

func inc(c *current, tag string) {
	v, _ := c.state[tag].(int)
	v++
	c.state[tag] = v
}

func dec(c *current, tag string) {
	v, ok := c.state[tag].(int)
	if ok {
		v--
		if v == 0 {
			delete(c.state, tag)
		} else {
			c.state[tag] = v
		}
	}
}

func count(c *current, tag string) int {
	v, _ := c.state[tag].(int)
	return v
}

type stack []interface{}

func (s stack) Clone() interface{} {
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
	return len(v) - 1
}

func pop(c *current, tag string) interface{} {
	v, _ := c.state[tag].(stack)
	if len(v) == 0 {
		return nil
	}
	val := v[len(v)-1]
	if len(v) == 1 {
		delete(c.state, tag)
	} else {
		c.state[tag] = v[:len(v)-1]
	}
	return val
}

func popTo(c *current, tag string, n int) {
	v, _ := c.state[tag].(stack)
	if len(v) > n {
		if n == 0 {
			delete(c.state, tag)
		} else {
			c.state[tag] = v[:n]
		}
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
	pos := c.pos.offset + len(c.text)
	//log.Printf("inlineBreaks %s, %q, pos %d", c.pos, c.text, pos)
	input := c.globalStore["text"].([]byte)
	if len(input) <= pos {
		log.Printf("inlinebreak false")
		return false, nil
	}
	ch := input[pos]
	if !inlineBreaksRegexp.Match([]byte{ch}) {
		//log.Printf("inlinebreak match fail: %s", []byte{ch})
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
		//log.Printf("inlineBreaks: } %q %q", preproc, input[pos:pos+2])
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
		//log.Printf("inlineBreaks extlink:%#v, preproc:%#v", extlink, preproc)
		return string(input[pos:pos+2]) == preproc, nil

	case '<':
		return (count(c, "noinclude") > 0 && string(input[pos:pos+12]) == "</noinclude>") ||
			(count(c, "includeonly") > 0 && string(input[pos:pos+14]) == "</includeonly>") ||
			(count(c, "onlyinclude") > 0 && string(input[pos:pos+14]) == "</onlyinclude>"), nil
	default:
		return false, errors.Errorf("Unhandled case!")
	}
}
