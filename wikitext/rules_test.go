package wikitext

import (
	"path"
	"testing"
)

func TestRules(t *testing.T) {
	cases := []struct {
		rule  string
		input string
		match string
	}{
		{
			"wikilink_preprocessor_text",
			"asdf",
			"asdf",
		},
		{
			"wikilink_preprocessor_text",
			"asdf|asdf",
			"asdf",
		},
		{
			"wikilink_preproc",
			"[[asdf]]",
			`<a href="./asdf">asdf</a>`,
		},
		{
			"wikilink_preproc",
			"[[a|b]]",
			`<a href="./a">b</a>`,
		},
		{
			"template",
			"{{reflink}}",
			"",
		},
		{
			"block_lines",
			"* foo",
			"<li> foo</li>",
		},
		{
			"heading",
			"== Foos ==",
			"<h2> Foos </h2>",
		},
		{
			"inlineline",
			"Foo's",
			"Foo's",
		},
		{
			"heading",
			"== Foo's ==",
			"<h2> Foo&#39;s </h2>",
		},
		{
			"extlink",
			"[http://example.com/ Yes Foo Bar]",
			`<a href="http://example.com/" class="external" rel="nofollow">Yes Foo Bar</a>`,
		},
		{
			"xmlish_tag",
			"<div>foo</div>",
			`<div _parsestart=""></div>`,
		},
		{
			"xmlish_tag",
			"</div>",
			`<div _parseend=""></div>`,
		},
		{
			"xmlish_tag",
			"<div/>",
			"<div></div>",
		},
		{
			"xmlish_tag",
			`<div foo="bar" />`,
			`<div foo="bar"></div>`,
		},
	}

	for _, c := range cases {
		c := c
		t.Run(path.Join(c.rule, c.input), func(t *testing.T) {
			val, err := Parse(
				"file",
				[]byte(c.input),
				GlobalStore("text", []byte(c.input)),
				GlobalStore("len", len(c.input)),
				Entrypoint(c.rule),
				Recover(false),
			)
			if err != nil {
				t.Error(err)
			}
			text := concat(val)
			if c.match != text {
				t.Errorf("got %q; expected %q", text, c.match)
			}
		})
	}
}
