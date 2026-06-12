package tui

import "testing"

// pastTense translates each verb csync emits into the past-tense word the
// post-sync summary shows. A wrong mapping misinforms the user about what
// happened to a file (created vs. deleted), so unlike the color mapping it is
// content worth pinning. Each case is one verb — the function's only input.
func TestPastTense(t *testing.T) {
	cases := []struct {
		verb string
		want string
	}{
		{"create", "created"},
		{"update", "updated"},
		{"delete", "deleted"},
		// An unrecognized verb passes through unchanged rather than vanishing, so a
		// future verb still names itself in the summary instead of rendering blank.
		{"frobnicate", "frobnicate"},
	}

	for _, tc := range cases {
		got := pastTense(tc.verb)
		if got != tc.want {
			t.Errorf("pastTense(%q) = %q, want %q", tc.verb, got, tc.want)
		}
	}
}
