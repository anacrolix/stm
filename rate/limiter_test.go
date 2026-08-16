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
