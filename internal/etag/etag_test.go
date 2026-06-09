package etag

import (
	"strings"
	"testing"
)

func TestWeak(t *testing.T) {
	a := Weak([]byte("hello"))
	b := Weak([]byte("hello"))
	c := Weak([]byte("world"))
	if a != b {
		t.Errorf("Weak must be deterministic: %q != %q", a, b)
	}
	if a == c {
		t.Errorf("Weak must differ for different bodies: %q == %q", a, c)
	}
	if !strings.HasPrefix(a, `W/"`) || !strings.HasSuffix(a, `"`) {
		t.Errorf("Weak must be a quoted weak validator, got %q", a)
	}
}

func TestMatches(t *testing.T) {
	const tag = `W/"abc123"`
	cases := []struct {
		ifNoneMatch string
		want        bool
	}{
		{"", false},
		{"*", true},
		{`W/"abc123"`, true},
		{`"abc123"`, true}, // strong form — If-None-Match uses weak comparison
		{`W/"other"`, false},
		{`W/"x", W/"abc123"`, true},    // comma list
		{`  W/"abc123" , W/"y"`, true}, // list with surrounding spaces
	}
	for _, c := range cases {
		if got := Matches(c.ifNoneMatch, tag); got != c.want {
			t.Errorf("Matches(%q, %q) = %v, want %v", c.ifNoneMatch, tag, got, c.want)
		}
	}
}

func TestMatches_StrongTag(t *testing.T) {
	// Static assets use a strong version tag; the client echoes it verbatim.
	const tag = `"v3.24.0"`
	if !Matches(`"v3.24.0"`, tag) {
		t.Error("exact strong-tag match must succeed")
	}
	if Matches(`"v3.23.0"`, tag) {
		t.Error("different version must not match")
	}
}
