package stm

import (
	"sync"
	"testing"
	"time"

	_ "github.com/anacrolix/envpprof"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecrement(t *testing.T) {
	x := NewVar[int](1000)
	for i := 0; i < 500; i++ {
		go Atomically(VoidOperation(func(tx *Tx) {
			cur := x.Get(tx)
			x.Set(tx, cur-1)
		}))
	}
	done := make(chan struct{})
	go func() {
		Atomically(VoidOperation(func(tx *Tx) {
			tx.Assert(x.Get(tx) == 500)
		}))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("decrement did not complete in time")
	}
}

// read-only transaction aren't exempt from calling tx.inputsChanged
func TestReadVerify(t *testing.T) {
	read := make(chan struct{})
	x, y := NewVar[int](1), NewVar[int](2)

	// spawn a transaction that writes to x
	go func() {
		<-read
		AtomicSet(x, 3)
		read <- struct{}{}
		// other tx should retry, so we need to read/send again
		read <- <-read
	}()

	// spawn a transaction that reads x, then y. The other tx will modify x in
	// between the reads, causing this tx to retry.
	var x2, y2 int
	Atomically(VoidOperation(func(tx *Tx) {
		x2 = x.Get(tx)
		read <- struct{}{}
		<-read // wait for other tx to complete
		y2 = y.Get(tx)
	}))
	if x2 == 1 && y2 == 2 {
		t.Fatal("read was not verified")
	}
}

func TestRetry(t *testing.T) {
	x := NewVar[int](10)
	// spawn 10 transactions, one every 10 milliseconds. This will decrement x
	// to 0 over the course of 100 milliseconds.
	go func() {
		for i := 0; i < 10; i++ {
			time.Sleep(10 * time.Millisecond)
			Atomically(VoidOperation(func(tx *Tx) {
				cur := x.Get(tx)
				x.Set(tx, cur-1)
			}))
		}
	}()
	// Each time we read x before the above loop has finished, we need to
	// retry. This should result in no more than 1 retry per transaction.
	retry := 0
	Atomically(VoidOperation(func(tx *Tx) {
		cur := x.Get(tx)
		if cur != 0 {
			retry++
			tx.Retry()
		}
	}))
	if retry > 10 {
		t.Fatal("should have retried at most 10 times, got", retry)
	}
}

func TestVerify(t *testing.T) {
	// tx.inputsChanged should check more than pointer equality
	type foo struct {
		i int
	}
	x := NewVar[*foo](&foo{3})
	read := make(chan struct{})

	// spawn a transaction that modifies x
	go func() {
		Atomically(VoidOperation(func(tx *Tx) {
			<-read
			rx := x.Get(tx)
			rx.i = 7
			x.Set(tx, rx)
		}))
		read <- struct{}{}
		// other tx should retry, so we need to read/send again
		read <- <-read
	}()

	// spawn a transaction that reads x, then y. The other tx will modify x in
	// between the reads, causing this tx to retry.
	var i int
	Atomically(VoidOperation(func(tx *Tx) {
		f := x.Get(tx)
		i = f.i
		read <- struct{}{}
		<-read // wait for other tx to complete
	}))
	if i == 3 {
		t.Fatal("inputsChanged did not retry despite modified Var", i)
	}
}

func TestSelect(t *testing.T) {
	// empty Select should panic
	require.Panics(t, func() { Atomically(Select()) })

	// with one arg, Select adds no effect
	x := NewVar[int](2)
	Atomically(Select(VoidOperation(func(tx *Tx) {
		tx.Assert(x.Get(tx) == 2)
	})))

	picked := Atomically(Select(
		// always blocks; should never be selected
		VoidOperation(func(tx *Tx) {
			tx.Retry()
		}),
		// always succeeds; should always be selected
		func(tx *Tx) interface{} {
			return 2
		},
		// always succeeds; should never be selected
		func(tx *Tx) interface{} {
			return 3
		},
	))
	assert.EqualValues(t, 2, picked)
}

func TestCompose(t *testing.T) {
	nums := make([]int, 100)
	fns := make([]Operation, 100)
	for i := range fns {
		fns[i] = func(x int) Operation {
			return VoidOperation(func(*Tx) { nums[x] = x })
		}(i) // capture loop var
	}
	Atomically(Compose(fns...))
	for i := range nums {
		if nums[i] != i {
			t.Error("Compose failed:", nums[i], i)
		}
	}
}

func TestPanic(t *testing.T) {
	// normal panics should escape Atomically
	assert.PanicsWithValue(t, "foo", func() {
		Atomically(func(*Tx) interface{} {
			panic("foo")
		})
	})
}

func TestReadWritten(t *testing.T) {
	// reading a variable written in the same transaction should return the
	// previously written value
	x := NewVar[int](3)
	Atomically(VoidOperation(func(tx *Tx) {
		x.Set(tx, 5)
		tx.Assert(x.Get(tx) == 5)
	}))
}

func TestAtomicSetRetry(t *testing.T) {
	// AtomicSet should cause waiting transactions to retry
	x := NewVar[int](3)
	done := make(chan struct{})
	go func() {
		Atomically(VoidOperation(func(tx *Tx) {
			tx.Assert(x.Get(tx) == 5)
		}))
		done <- struct{}{}
	}()
	time.Sleep(10 * time.Millisecond)
	AtomicSet(x, 5)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("AtomicSet did not wake up a waiting transaction")
	}
}

func testPingPong(t testing.TB, n int, afterHit func(string)) {
	ball := NewBuiltinEqVar(false)
	doneVar := NewVar[bool](false)
	hits := NewVar[int](0)
	ready := NewVar[bool](true) // The ball is ready for hitting.
	var wg sync.WaitGroup
	bat := func(from, to bool, noise string) {
		defer wg.Done()
		for !Atomically(func(tx *Tx) interface{} {
			if doneVar.Get(tx) {
				return true
			}
			tx.Assert(ready.Get(tx))
			if ball.Get(tx) == from {
				ball.Set(tx, to)
				hits.Set(tx, hits.Get(tx)+1)
				ready.Set(tx, false)
				return false
			}
			return tx.Retry()
		}).(bool) {
			afterHit(noise)
			AtomicSet(ready, true)
		}
	}
	wg.Add(2)
	go bat(false, true, "ping!")
	go bat(true, false, "pong!")
	Atomically(VoidOperation(func(tx *Tx) {
		tx.Assert(hits.Get(tx) >= n)
		doneVar.Set(tx, true)
	}))
	wg.Wait()
}

func TestPingPong(t *testing.T) {
	testPingPong(t, 42, func(s string) { t.Log(s) })
}

func TestSleepingBeauty(t *testing.T) {
	require.Panics(t, func() {
		Atomically(func(tx *Tx) interface{} {
			tx.Assert(false)
			return nil
		})
	})
}

//func TestRetryStack(t *testing.T) {
//	v := NewVar[int](nil)
//	go func() {
//		i := 0
//		for {
//			AtomicSet(v, i)
//			i++
//		}
//	}()
//	Atomically(func(tx *Tx) interface{} {
//		debug.PrintStack()
//		ret := func() {
//			defer Atomically(nil)
//		}
//		v.Get(tx)
//		tx.Assert(false)
//		return ret
//	})
//}
