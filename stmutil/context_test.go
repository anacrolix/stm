package stmutil

import (
	"context"
	"testing"
	"time"

	"github.com/anacrolix/stm"
	qt "github.com/go-quicktest/qt"
)

func TestContextEquality(t *testing.T) {
	ctx := context.Background()
	qt.Check(t, qt.IsTrue(ctx == context.Background()))
	childCtx, cancel := context.WithCancel(ctx)
	qt.Check(t, qt.IsTrue(childCtx != ctx))
	qt.Check(t, qt.IsTrue(childCtx != ctx))
	qt.Check(t, qt.Equals(ctx, context.Background()))
	cancel()
	qt.Check(t, qt.Equals(ctx, context.Background()))
	qt.Check(t, qt.Not(qt.Equals(childCtx, ctx)))
}

// Blocks in a transaction until the var is set, the way a caller waiting on a
// Context does. Reports whether that happened before the timeout.
func awaitDoneVar(v *stm.Var[bool], timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		defer close(done)
		stm.Atomically(stm.VoidOperation(func(tx *stm.Tx) {
			tx.Assert(v.Get(tx))
		}))
	}()
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

func TestContextDoneVarSetWhenContextIsDone(t *testing.T) {
	ctx, cancelCtx := context.WithCancel(context.Background())
	v, cancel := ContextDoneVar(ctx)
	defer cancel()
	qt.Check(t, qt.IsFalse(stm.AtomicGet(v)))
	cancelCtx()
	qt.Check(t, qt.IsTrue(awaitDoneVar(v, 2*time.Second)))
}

func TestContextDoneVarForContextAlreadyDone(t *testing.T) {
	ctx, cancelCtx := context.WithCancel(context.Background())
	cancelCtx()
	v, cancel := ContextDoneVar(ctx)
	defer cancel()
	qt.Check(t, qt.IsTrue(stm.AtomicGet(v)))
}

// Cancelling unregisters from the Context, so completing it afterwards leaves
// the Var alone. A second Var on the same Context, which isn't cancelled, says
// when the Context has finished running what it did have registered.
func TestContextDoneVarCancelUnregisters(t *testing.T) {
	ctx, cancelCtx := context.WithCancel(context.Background())
	v, cancel := ContextDoneVar(ctx)
	canary, cancelCanary := ContextDoneVar(ctx)
	defer cancelCanary()
	cancel()
	cancelCtx()
	qt.Assert(t, qt.IsTrue(awaitDoneVar(canary, 2*time.Second)))
	// The two registrations run in separate goroutines, so give a cancelled one
	// that is going to run anyway the chance to.
	time.Sleep(100 * time.Millisecond)
	qt.Check(t, qt.IsFalse(stm.AtomicGet(v)))
}
