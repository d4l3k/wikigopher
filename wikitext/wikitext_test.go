package wikitext

import (
	"log"
	"strings"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"golang.org/x/net/html"
)

func TestState(t *testing.T) {
	var p parser
	p.cur.state = make(storeDict)
	p.restoreState(p.cloneState())
	c := &p.cur

	backup := p.cloneState()

	c.state["foo"] = true

	p.restoreState(backup)
	if len(p.cur.state) > 0 {
		t.Fatalf("leaking state! %#v", p.cur.state)
	}
}

func TestConvert(t *testing.T) {
	log.SetFlags(log.Flags() | log.Lshortfile)

	cases := []struct {
		in   string
		want string
	}{
		{
			"Blah",
			"<p>Blah</p>",
		},
		{
			"== Test ==",
			"<h2> Test </h2>",
		},
		{
			"=Test=",
			"<h1>Test</h1>",
		},
		{
			"'''Test'''",
			"<b>Test</b>",
		},
		{
			"* foo\n* nah\n* woof",
			"<li> foo</li>\n<li> nah</li>\n<li> woof</li>",
		},
		{
			"----",
			"<hr/>",
		},
		{
			"{{reflink}}\n\nBlah",
			"<p></p><p>Blah</p>",
		},
		{
			"[[Jordanstown]]",
			`<p><a href="./Jordanstown">Jordanstown</a></p>`,
		},
		{
			"[[Jordanstown|Blah]]",
			`<p><a href="./Jordanstown">Blah</a></p>`,
		},
		{
			`{{Infobox basketball club
| name = Ulster Elks
| color1 = white
| color2 = blue
| logo =
| arena = [[Ulster University]] Sports Centre
}}`,
			"<p></p>",
		},
		{
			`<div class="bar">Test</div>`,
			`<p><div class="bar">Test</div></p>`,
		},
		{
			"<ref>Foo\n</ref>Bar",
			"<p><ref>Foo\n</ref>Bar</p>",
		},
		{
			"<ref>A</ref>B",
			"<p><ref>A</ref>B</p>",
		},
	}

	debugRules(true)

	for _, c := range cases {
		c := c
		t.Run(c.in, func(t *testing.T) {
			outBytes, err := Convert([]byte(c.in), strict())
			if err != nil {
				t.Fatal(err)
			}

			out := string(outBytes)
			if out != c.want {
				t.Errorf("Covert(%q) = %q; not %q", c.in, out, c.want)
			}
		})
	}
}

func TestSanitizationPolicy(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{
			"<div></div>",
			"<div></div>",
		},
		{
			"<div>A</div>",
			"<div>A</div>",
		},
		{
			"<ref></ref>",
			"<ref></ref>",
		},
	}

	p := wikitextPolicy()

	for _, c := range cases {
		c := c
		t.Run(c.in, func(t *testing.T) {
			doc, err := html.Parse(strings.NewReader(c.in))
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("Doc = %s", spew.Sdump(doc))

			out := p.Sanitize(c.in)
			if out != c.want {
				t.Errorf("Sanitize(%q) = %q; not %q", c.in, out, c.want)
			}
		})
	}
}
