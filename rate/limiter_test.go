package rate

import (
	"os"
	"os/exec"
	"testing"
	"time"
)

// The token generator runs in its own goroutine, so a panic there takes the
// process down instead of failing a test. Run such cases in a subprocess.
func runIsolated(t *testing.T, f func(t *testing.T)) {
	name := t.Name()
	if os.Getenv("STM_ISOLATED_TEST") == name {
		f(t)
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^"+name+"$", "-test.v")
	cmd.Env = append(os.Environ(), "STM_ISOLATED_TEST="+name)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("isolated run of %v failed: %v\n%s", name, err, out)
	}
}

// tokenGenerator counts the tokens that have become available before it blocks
// on the bucket having room for one, so when a token is finally taken it
// commits a count worked out from a timestamp that has gone stale, and winds
// lastAdd back into the past. The round after that finds a pile of tokens
// available immediately, and hands them out regardless of the burst.
func TestBurstIsNotExceededAfterIdling(t *testing.T) {
	const interval = time.Second
	// Idle for a non-multiple of the interval, so that a token becoming
	// available on schedule can't land inside the window measured below.
	const idle = 2500 * time.Millisecond
	const window = 20 * time.Millisecond
	rl := NewLimiter(Every(interval), 1)
	time.Sleep(idle)
	granted := 0
	for deadline := time.Now().Add(window); time.Now().Before(deadline); {
		if rl.Allow() {
			granted++
		}
	}
	if granted > 1 {
		t.Errorf("granted %v tokens in %v after idling %v, want at most the burst of 1",
			granted, window, idle)
	}
}

// A Limit above one token per nanosecond has an interval that rounds down to
// zero, which the token generator then divides by.
func TestRateFasterThanOnePerNanosecond(t *testing.T) {
	runIsolated(t, func(t *testing.T) {
		rl := NewLimiter(Limit(2e9), 10)
		time.Sleep(100 * time.Millisecond)
		for range 20 {
			rl.Allow()
		}
		time.Sleep(100 * time.Millisecond)
		if !rl.Allow() {
			t.Fatal("a limiter this fast should always have a token")
		}
	})
}
