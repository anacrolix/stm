package stm_test

import (
	"github.com/anacrolix/stm"
)

func Example() {
	// create a shared variable
	n := stm.NewVar(3)

	// read a variable
	var v int
	stm.Atomically(stm.VoidOperation(func(tx *stm.Tx) {
		v = tx.Get(n).(int)
	}))
	// or:
	v = stm.AtomicGet(n).(int)
	_ = v

	// write to a variable
	stm.Atomically(stm.VoidOperation(func(tx *stm.Tx) {
		tx.Set(n, 12)
	}))
	// or:
	stm.AtomicSet(n, 12)

	// update a variable
	stm.Atomically(stm.VoidOperation(func(tx *stm.Tx) {
		cur := tx.Get(n).(int)
		tx.Set(n, cur-1)
	}))

	// block until a condition is met
	stm.Atomically(stm.VoidOperation(func(tx *stm.Tx) {
		cur := tx.Get(n).(int)
		if cur != 0 {
			tx.Retry()
		}
		tx.Set(n, 10)
	}))
	// or:
	stm.Atomically(stm.VoidOperation(func(tx *stm.Tx) {
		cur := tx.Get(n).(int)
		tx.Assert(cur == 0)
		tx.Set(n, 10)
	}))

	// select among multiple (potentially blocking) transactions
	stm.Atomically(stm.Select(
		// this function blocks forever, so it will be skipped
		stm.VoidOperation(func(tx *stm.Tx) { tx.Retry() }),

		// this function will always succeed without blocking
		stm.VoidOperation(func(tx *stm.Tx) { tx.Set(n, 10) }),

		// this function will never run, because the previous
		// function succeeded
		stm.VoidOperation(func(tx *stm.Tx) { tx.Set(n, 11) }),
	))

	// since Select is a normal transaction, if the entire select retries
	// (blocks), it will be retried as a whole:
	x := 0
	stm.Atomically(stm.Select(
		// this function will run twice, and succeed the second time
		stm.VoidOperation(func(tx *stm.Tx) { tx.Assert(x == 1) }),

		// this function will run once
		stm.VoidOperation(func(tx *stm.Tx) {
			x = 1
			tx.Retry()
		}),
	))
	// But wait! Transactions are only retried when one of the Vars they read is
	// updated. Since x isn't a stm Var, this code will actually block forever --
	// but you get the idea.
}
