package stm

import (
	"runtime"
	"testing"
	"time"
)

// Runs a transaction that installs a watcher on x by retrying, and then panics
// on its second attempt. Returns once the panic has escaped Atomically.
func panicAfterRetry(x *Var[int]) {
	panicked := make(chan struct{})
	go func() {
		defer func() {
			recover()
			close(panicked)
		}()
		Atomically(VoidOperation(func(tx *Tx) {
			if x.Get(tx) == 0 {
				tx.Retry()
			}
			panic("boom")
		}))
	}()
	// Give the transaction time to install its watcher and go to sleep.
	time.Sleep(100 * time.Millisecond)
	AtomicSet(x, 1)
	<-panicked
	time.Sleep(100 * time.Millisecond)
}

// Atomically locks tx.mu around the operation without deferring the unlock, so
// a panic that isn't the retry sentinel leaves the mutex locked, and the
// transaction registered in the watchers of every Var it read. Each subsequent
// write to such a Var strands a wakeWatchers goroutine on tx.mu forever.
func TestPanicDoesNotLeakWatcherGoroutines(t *testing.T) {
	x := NewVar(0)
	panicAfterRetry(x)
	before := runtime.NumGoroutine()
	const writes = 10
	for i := 0; i < writes; i++ {
		AtomicSet(x, 2+i)
	}
	time.Sleep(500 * time.Millisecond)
	after := runtime.NumGoroutine()
	if after > before+1 {
		t.Errorf("%v goroutines leaked by %v writes (%v before, %v after)",
			after-before, writes, before, after)
	}
}

// wakeWatchers blocks inside sync.Map.Range while holding up the iteration, so
// a transaction stranded by a panic can stop every watcher visited after it
// from ever being woken. Which watchers those are depends on map iteration
// order, hence the repetition.
func TestPanicDoesNotBlockOtherWaiters(t *testing.T) {
	for i := 0; i < 5; i++ {
		x := NewVar(0)
		panicAfterRetry(x)
		woke := make(chan struct{})
		go func() {
			Atomically(VoidOperation(func(tx *Tx) {
				tx.Assert(x.Get(tx) == 42)
			}))
			close(woke)
		}()
		// Let the waiter install its watcher and sleep.
		time.Sleep(200 * time.Millisecond)
		AtomicSet(x, 42)
		select {
		case <-woke:
		case <-time.After(2 * time.Second):
			t.Fatalf("waiter was never woken on attempt %v", i+1)
		}
	}
}
