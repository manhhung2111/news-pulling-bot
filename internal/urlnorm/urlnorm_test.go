package urlnorm

import "testing"

func TestNormalize(t *testing.T) {
	same := [][2]string{
		{"https://www.example.com/post/", "http://example.com/post"},
		{"https://example.com/a?utm_source=x&utm_medium=y&id=3", "https://example.com/a?id=3"},
		{"https://example.com/a?b=2&a=1", "https://example.com/a?a=1&b=2"},
		{"https://example.com/a#section", "https://example.com/a"},
		{"https://example.com/a?ref=hn&fbclid=1&gclid=2", "https://example.com/a"},
	}
	for _, p := range same {
		a, b := Normalize(p[0]), Normalize(p[1])
		if a == "" || a != b {
			t.Errorf("Normalize(%q)=%q, Normalize(%q)=%q; want equal non-empty", p[0], a, p[1], b)
		}
	}
	exact := map[string]string{
		"https://EXAMPLE.com/A":                  "https://example.com/A",
		"https://news.ycombinator.com/item?id=1": "https://news.ycombinator.com/item?id=1",
		"https://example.com/":                   "https://example.com/",
		"mailto:x@y.z":                           "",
		"":                                       "",
		"::not a url":                            "",
	}
	for in, want := range exact {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}
