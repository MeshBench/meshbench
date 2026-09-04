package firmwarelib

import "testing"

// v1.9.0 must sort below v1.17.0. As strings it does not, which is how
// asking for a board with no version came back with v1.14.1 while v1.17.1
// was published.
func TestNewerVersionCountsRatherThanCompares(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"v1.17.0", "v1.9.0", true},
		{"v1.9.0", "v1.17.0", false},
		{"v1.17.1", "v1.17.0", true},
		{"v1.17.0", "v1.17.1", false},
		{"v2.0.0", "v1.99.99", true},
		{"v1.17.0", "v1.17.0", false},
	}
	for _, c := range cases {
		if got := newerVersion(c.a, c.b); got != c.want {
			t.Errorf("newerVersion(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}
