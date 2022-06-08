package stm_test

import (
	"github.com/anacrolix/stm"
)

func Example() {
	// create a shared variable
	n := stm.NewVar[int](3)

	// read a variable
	var v int
	stm.Atomically(stm.VoidOperation(func(tx *stm.Tx) {
		v = n.Get(tx)
	}))
	// or:
	v = stm.AtomicGet(n)
	_ = v

	// write to a variable
	stm.Atomically(stm.VoidOperation(func(tx *stm.Tx) {
		n.Set(tx, 12)
	}))
	// or:
	stm.AtomicSet(n, 12)

	// update a variable
	stm.Atomically(stm.VoidOperation(func(tx *stm.Tx) {
		cur := n.Get(tx)
		n.Set(tx, cur-1)
	}))

	// block until a condition is met
	stm.Atomically(stm.VoidOperation(func(tx *stm.Tx) {
		cur := n.Get(tx)
		if cur != 0 {
			tx.Retry()
		}
		n.Set(tx, 10)
	}))
	// or:
	stm.Atomically(stm.VoidOperation(func(tx *stm.Tx) {
		cur := n.Get(tx)
		tx.Assert(cur == 0)
		n.Set(tx, 10)
	}))

	// select among multiple (potentially blocking) transactions
	stm.Atomically(stm.Select(
		// this function blocks forever, so it will be skipped
		stm.VoidOperation(func(tx *stm.Tx) { tx.Retry() }),

		// this function will always succeed without blocking
		stm.VoidOperation(func(tx *stm.Tx) { n.Set(tx, 10) }),

		// this function will never run, because the previous
		// function succeeded
		stm.VoidOperation(func(tx *stm.Tx) { n.Set(tx, 11) }),
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
