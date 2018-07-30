package wikitext

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func TestProcessTokens(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{
			"", "",
		},
		{
			"<div></div>",
			"<div></div>",
		},
		{
			"<div _parsestart></div> <p _parsestart></p> Foo <p _parseend></p> <div _parseend></div>",
			`<div _parsestart=""> <p _parsestart=""> Foo </p> </div>`,
		},
		{
			`<div _parsestart></div>Foo<div _parseend></div> asdf <p>Blah</p> <div _parsestart></div>Bar<div _parseend></div>`,
			`<div _parsestart="">Foo</div> asdf <p>Blah</p> <div _parsestart="">Bar</div>`,
		},
	}

	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			doc, err := html.Parse(strings.NewReader(c.in))
			if err != nil {
				t.Fatal(err)
			}

			//t.Log(concat(doc))

			if remaining := processTokens(doc); len(remaining) > 0 {
				t.Errorf("got %d extra children", len(remaining))
			}
			var buf bytes.Buffer
			if err := html.Render(&buf, doc); err != nil {
				t.Fatal(err)
			}
			want := fmt.Sprintf("<html><head></head><body>%s</body></html>", c.want)
			out := buf.String()
			if out != want {
				t.Errorf("expected %q;\ngot %q", want, out)
			}
		})
	}
}
