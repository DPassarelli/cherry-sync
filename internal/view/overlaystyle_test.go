package view

import "testing"

// A file's full path is its operand and its relative path joined by one slash, with
// a root operand contributing no slash of its own.
func TestEndpointPath(t *testing.T) {
	cases := []struct{ operand, rel, want string }{
		{"user@host:/project", "src/main.go", "user@host:/project/src/main.go"},
		{"./project", "README.md", "./project/README.md"},
		{"host:.", "README.md", "host:./README.md"},
		{"/", "README.md", "/README.md"},
	}
	for _, c := range cases {
		got := endpointPath(c.operand, c.rel)
		if got != c.want {
			t.Errorf("endpointPath(%q, %q) = %q, want %q", c.operand, c.rel, got, c.want)
		}
	}
}
