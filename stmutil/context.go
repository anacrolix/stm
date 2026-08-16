package stmutil

import (
	"context"

	"github.com/anacrolix/stm"
)

// Returns an STM var that contains a bool equal to `ctx.Err != nil`, and a cancel function to be
// called when the user is no longer interested in the var.
func ContextDoneVar(ctx context.Context) (*stm.Var[bool], func()) {
	if ctx.Err() != nil {
		// TODO: What if we had read-only Vars? Then we could have a global one for this that we
		// just reuse.
		return stm.NewBuiltinEqVar(true), func() {}
	}
	v := stm.NewVar(false)
	// AfterFunc registers with the Context instead of parking a goroutine on <-ctx.Done(), and
	// returns the means to unregister again, which is all there is for cancel to do. Callers get a
	// Var each: sharing one per Context needs a cache, and a cache needs to know when the last
	// caller has gone, which costs more than the Var it saves. See the benchmarks.
	stop := context.AfterFunc(ctx, func() {
		stm.AtomicSet(v, true)
	})
	return v, func() { stop() }
}
