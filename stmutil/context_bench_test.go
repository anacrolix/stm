package stmutil

import (
	"context"
	"sync"
	"testing"

	"github.com/anacrolix/stm"
)

// ContextDoneVar hands every caller its own Var and its own registration on
// the Context. These cover what that costs, and what caching a Var per Context
// instead would have to beat: a cache turns the first two into a map lookup
// under a process-wide mutex, wins on the third where callers have something
// to share, and loses on the fourth where they don't.

func BenchmarkContextDoneVarBackground(b *testing.B) {
	b.ReportAllocs()
	ctx := context.Background()
	for b.Loop() {
		_, cancel := ContextDoneVar(ctx)
		cancel()
	}
}

func BenchmarkContextDoneVarCancellable(b *testing.B) {
	b.ReportAllocs()
	ctx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()
	for b.Loop() {
		_, cancel := ContextDoneVar(ctx)
		cancel()
	}
}

// Every caller on one Context: registrations contend on that Context's lock.
func BenchmarkContextDoneVarParallelSharedContext(b *testing.B) {
	b.ReportAllocs()
	ctx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, cancel := ContextDoneVar(ctx)
			cancel()
		}
	})
}

// A Context per caller, as in one per request, which is the common shape.
func BenchmarkContextDoneVarParallelDistinctContexts(b *testing.B) {
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			ctx, cancelCtx := context.WithCancel(context.Background())
			_, cancel := ContextDoneVar(ctx)
			cancel()
			cancelCtx()
		}
	})
}

// Waking every transaction blocked on one Context at once.
func BenchmarkContextDoneVarWakeAll(b *testing.B) {
	const waiters = 200
	b.ReportAllocs()
	for b.Loop() {
		b.StopTimer()
		ctx, cancelCtx := context.WithCancel(context.Background())
		var wg sync.WaitGroup
		ready := make(chan struct{}, waiters)
		for range waiters {
			v, cancel := ContextDoneVar(ctx)
			wg.Go(func() {
				defer cancel()
				ready <- struct{}{}
				stm.Atomically(stm.VoidOperation(func(tx *stm.Tx) {
					tx.Assert(v.Get(tx))
				}))
			})
		}
		for range waiters {
			<-ready
		}
		b.StartTimer()
		cancelCtx()
		wg.Wait()
	}
}
