package wikitext

import (
	"path"
	"testing"
)

func TestRules(t *testing.T) {
	cases := []struct {
		rule  string
		input string
		match bool
	}{
		{
			"wikilink_preprocessor_text",
			"asdf",
			true,
		},
		{
			"wikilink_preprocessor_text",
			"asdf|asdf",
			false,
		},
	}

	for _, c := range cases {
		c := c
		t.Run(path.Join(c.rule, c.input), func(t *testing.T) {
			_, err := Parse(
				"file",
				[]byte(c.input),
				GlobalStore("text", []byte(c.input)),
				GlobalStore("len", len(c.input)),
				Entrypoint(c.rule),
				Recover(false),
				Debug(true),
			)
			if c.match && err != nil {
				t.Error(err)
			} else if !c.match && err == nil {
				t.Error("expected error")
			}
		})
	}
}
