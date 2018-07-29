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
			`<a href="asdf">asdf</a>`,
		},
		{
			"wikilink_preproc",
			"[[a|b]]",
			`<a href="a">b</a>`,
		},
		{
			"template",
			"{{reflink}}",
			"",
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
