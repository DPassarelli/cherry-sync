package runlog

import (
	"testing"
	"time"
)

// TestRoundUpMillis pins the rounding decision the acceptance suite cannot reach: it
// runs real rsync, which always takes more than a millisecond, so it can prove the
// value is decimal-free and positive but not that the rounding is *up* rather than to
// nearest. The sub-millisecond cases below are where the two differ — rounding to
// nearest would floor a fast call to 0ms, which is exactly what rounding up prevents.
func TestRoundUpMillis(t *testing.T) {
	cases := []struct {
		name string
		in   time.Duration
		want string
	}{
		{"a single nanosecond of work still rounds up to one millisecond", 1 * time.Nanosecond, "1ms"},
		{"a sub-millisecond call rounds up, never down to zero", 500 * time.Microsecond, "1ms"},
		{"an exact millisecond is left as it is", 1 * time.Millisecond, "1ms"},
		{"a fractional millisecond rounds up", 43900 * time.Microsecond, "44ms"},
		{"a whole-millisecond count is left as it is", 43 * time.Millisecond, "43ms"},
		{"over a second stays whole milliseconds, with no decimal seconds", 1500 * time.Millisecond, "1500ms"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := roundUpMillis(c.in)
			if got != c.want {
				t.Errorf("roundUpMillis(%v) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
