package wikitext

import (
	"log"
	"testing"
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
			`<p><a href="Jordanstown">Jordanstown</a></p>`,
		},
		{
			"[[Jordanstown|Blah]]",
			`<p><a href="Jordanstown">Blah</a></p>`,
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
	}

	debugRules(true)

	for _, c := range cases {
		c := c
		t.Run(c.in, func(t *testing.T) {
			outBytes, err := Convert([]byte(c.in))
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
