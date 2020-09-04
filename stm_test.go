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
	x := NewVar(1000)
	for i := 0; i < 500; i++ {
		go Atomically(VoidOperation(func(tx *Tx) {
			cur := tx.Get(x).(int)
			tx.Set(x, cur-1)
		}))
	}
	done := make(chan struct{})
	go func() {
		Atomically(VoidOperation(func(tx *Tx) {
			tx.Assert(tx.Get(x) == 500)
		}))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("decrement did not complete in time")
	}
}

// read-only transaction aren't exempt from calling tx.verify
func TestReadVerify(t *testing.T) {
	read := make(chan struct{})
	x, y := NewVar(1), NewVar(2)

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
		x2 = tx.Get(x).(int)
		read <- struct{}{}
		<-read // wait for other tx to complete
		y2 = tx.Get(y).(int)
	}))
	if x2 == 1 && y2 == 2 {
		t.Fatal("read was not verified")
	}
}

func TestRetry(t *testing.T) {
	x := NewVar(10)
	// spawn 10 transactions, one every 10 milliseconds. This will decrement x
	// to 0 over the course of 100 milliseconds.
	go func() {
		for i := 0; i < 10; i++ {
			time.Sleep(10 * time.Millisecond)
			Atomically(VoidOperation(func(tx *Tx) {
				cur := tx.Get(x).(int)
				tx.Set(x, cur-1)
			}))
		}
	}()
	// Each time we read x before the above loop has finished, we need to
	// retry. This should result in no more than 1 retry per transaction.
	retry := 0
	Atomically(VoidOperation(func(tx *Tx) {
		cur := tx.Get(x).(int)
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
	// tx.verify should check more than pointer equality
	type foo struct {
		i int
	}
	x := NewVar(&foo{3})
	read := make(chan struct{})

	// spawn a transaction that modifies x
	go func() {
		Atomically(VoidOperation(func(tx *Tx) {
			<-read
			rx := tx.Get(x).(*foo)
			rx.i = 7
			tx.Set(x, rx)
		}))
		read <- struct{}{}
		// other tx should retry, so we need to read/send again
		read <- <-read
	}()

	// spawn a transaction that reads x, then y. The other tx will modify x in
	// between the reads, causing this tx to retry.
	var i int
	Atomically(VoidOperation(func(tx *Tx) {
		f := tx.Get(x).(*foo)
		i = f.i
		read <- struct{}{}
		<-read // wait for other tx to complete
	}))
	if i == 3 {
		t.Fatal("verify did not retry despite modified Var", i)
	}
}

func TestSelect(t *testing.T) {
	// empty Select should panic
	require.Panics(t, func() { Atomically(Select()) })

	// with one arg, Select adds no effect
	x := NewVar(2)
	Atomically(Select(VoidOperation(func(tx *Tx) {
		tx.Assert(tx.Get(x).(int) == 2)
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
	)).(int)
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
	x := NewVar(3)
	Atomically(VoidOperation(func(tx *Tx) {
		tx.Set(x, 5)
		tx.Assert(tx.Get(x).(int) == 5)
	}))
}

func TestAtomicSetRetry(t *testing.T) {
	// AtomicSet should cause waiting transactions to retry
	x := NewVar(3)
	done := make(chan struct{})
	go func() {
		Atomically(VoidOperation(func(tx *Tx) {
			tx.Assert(tx.Get(x).(int) == 5)
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
	doneVar := NewVar(false)
	hits := NewVar(0)
	ready := NewVar(true) // The ball is ready for hitting.
	var wg sync.WaitGroup
	bat := func(from, to interface{}, noise string) {
		defer wg.Done()
		for !Atomically(func(tx *Tx) interface{} {
			if tx.Get(doneVar).(bool) {
				return true
			}
			tx.Assert(tx.Get(ready).(bool))
			if tx.Get(ball) == from {
				tx.Set(ball, to)
				tx.Set(hits, tx.Get(hits).(int)+1)
				tx.Set(ready, false)
				return false
			}
			panic(Retry)
		}).(bool) {
			afterHit(noise)
			AtomicSet(ready, true)
		}
	}
	wg.Add(2)
	go bat(false, true, "ping!")
	go bat(true, false, "pong!")
	Atomically(VoidOperation(func(tx *Tx) {
		tx.Assert(tx.Get(hits).(int) >= n)
		tx.Set(doneVar, true)
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
//	v := NewVar(nil)
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
//		tx.Get(v)
//		tx.Assert(false)
//		return ret
//	})
//}
