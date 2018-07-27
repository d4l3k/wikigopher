package wikitext

import "testing"

func TestConvert(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{
			"\n\nBlah",
			"<p>Blah</p>",
		},
		{
			"\n\n== Test ==",
			"<h2>Test</h2>",
		},
		{
			"\n\n* foo\n* nah\n* woof",
			"<p>Blah</p>",
		},
		{
			"\n\n----",
			"\n\n<hr/>",
		},
	}

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
