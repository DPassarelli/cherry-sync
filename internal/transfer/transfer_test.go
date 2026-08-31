package transfer

import "testing"

// TestStalled covers the decision that separates an rsync which gave up on a
// silent remote from every other way an rsync can fail. The decision has to be
// made from stderr alone: the two rsync implementations csync supports report a
// timeout with three different exit codes between them (GNU rsync 30, openrsync
// 1 from a dead peer and 20 from a self-inflicted one), so the code carries no
// portable signal, while both wordings do.
//
// The last case is the one that matters. A bare search for "timeout" passes
// every case above it and fails only here, where the word is part of a filename
// in an unrelated error — which is exactly how a user with a file called
// timeout.log would be told their healthy remote had died.
func TestStalled(t *testing.T) {
	cases := []struct {
		name   string
		stderr string
		want   bool
	}{
		{
			name:   "GNU rsync giving up on a silent peer",
			stderr: "[sender] io timeout after 6 seconds -- exiting\nrsync error: timeout in data send/receive (code 30) at io.c(201) [sender=3.4.1]\n",
			want:   true,
		},
		{
			name:   "openrsync giving up on a silent peer",
			stderr: "rsync(61508): error: poll: timeout\n",
			want:   true,
		},
		{
			name:   "a peer that closed the connection instead of going quiet",
			stderr: "rsync: connection unexpectedly closed (0 bytes received so far) [sender]\nrsync error: error in rsync protocol data stream (code 12) at io.c(232) [sender=3.4.1]\n",
			want:   false,
		},
		{
			name:   "a file that could not be transferred",
			stderr: "rsync: [sender] link_stat \"/project/nope\" failed: No such file or directory (2)\nrsync error: some files could not be transferred (code 23)\n",
			want:   false,
		},
		{
			name:   "an unrelated failure naming a file called timeout",
			stderr: "rsync: [sender] link_stat \"/project/timeout.log\" failed: No such file or directory (2)\nrsync error: some files could not be transferred (code 23)\n",
			want:   false,
		},
		{
			name:   "a failure that said nothing at all",
			stderr: "",
			want:   false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := stalled([]byte(tc.stderr))
			if got != tc.want {
				t.Errorf("stalled(%q) = %t, want %t", tc.stderr, got, tc.want)
			}
		})
	}
}
