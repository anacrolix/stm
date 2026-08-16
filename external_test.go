package stm_test

import (
	"sync"
	"testing"

	"github.com/anacrolix/stm"
	"github.com/anacrolix/stm/stmutil"
)

const maxTokens = 25

func BenchmarkThunderingHerdCondVar(b *testing.B) {
	for b.Loop() {
		var mu sync.Mutex
		consumer := sync.NewCond(&mu)
		generator := sync.NewCond(&mu)
		done := false
		tokens := 0
		var pending sync.WaitGroup
		for range 1000 {
			pending.Go(func() {
				mu.Lock()
				for {
					if tokens > 0 {
						tokens--
						generator.Signal()
						break
					}
					consumer.Wait()
				}
				mu.Unlock()
			})
		}
		go func() {
			mu.Lock()
			for !done {
				if tokens < maxTokens {
					tokens++
					consumer.Signal()
				} else {
					generator.Wait()
				}
			}
			mu.Unlock()
		}()
		pending.Wait()
		mu.Lock()
		done = true
		generator.Signal()
		mu.Unlock()
	}

}

func BenchmarkThunderingHerd(b *testing.B) {
	for b.Loop() {
		done := stm.NewBuiltinEqVar(false)
		tokens := stm.NewBuiltinEqVar(0)
		pending := stm.NewBuiltinEqVar(0)
		for range 1000 {
			stm.Atomically(stm.VoidOperation(func(tx *stm.Tx) {
				pending.Set(tx, pending.Get(tx)+1)
			}))
			go func() {
				stm.Atomically(stm.VoidOperation(func(tx *stm.Tx) {
					t := tokens.Get(tx)
					if t > 0 {
						tokens.Set(tx, t-1)
						pending.Set(tx, pending.Get(tx)-1)
					} else {
						tx.Retry()
					}
				}))
			}()
		}
		go func() {
			for stm.Atomically(func(tx *stm.Tx) bool {
				if done.Get(tx) {
					return false
				}
				tx.Assert(tokens.Get(tx) < maxTokens)
				tokens.Set(tx, tokens.Get(tx)+1)
				return true
			}) {
			}
		}()
		stm.Atomically(stm.VoidOperation(func(tx *stm.Tx) {
			tx.Assert(pending.Get(tx) == 0)
		}))
		stm.AtomicSet(done, true)
	}
}

func BenchmarkInvertedThunderingHerd(b *testing.B) {
	for b.Loop() {
		done := stm.NewBuiltinEqVar(false)
		tokens := stm.NewBuiltinEqVar(0)
		pending := stm.NewVar(stmutil.NewSet[*stm.Var[bool]]())
		for range 1000 {
			ready := stm.NewVar(false)
			stm.Atomically(stm.VoidOperation(func(tx *stm.Tx) {
				pending.Set(tx, pending.Get(tx).Add(ready))
			}))
			go func() {
				stm.Atomically(stm.VoidOperation(func(tx *stm.Tx) {
					tx.Assert(ready.Get(tx))
					set := pending.Get(tx)
					if !set.Contains(ready) {
						panic("couldn't find ourselves in pending")
					}
					pending.Set(tx, set.Delete(ready))
				}))
				//b.Log("waiter finished")
			}()
		}
		go func() {
			for stm.Atomically(func(tx *stm.Tx) bool {
				if done.Get(tx) {
					return false
				}
				tx.Assert(tokens.Get(tx) < maxTokens)
				tokens.Set(tx, tokens.Get(tx)+1)
				return true
			}) {
			}
		}()
		go func() {
			for stm.Atomically(func(tx *stm.Tx) bool {
				tx.Assert(tokens.Get(tx) > 0)
				tokens.Set(tx, tokens.Get(tx)-1)
				for ready := range pending.Get(tx).All() {
					if !ready.Get(tx) {
						ready.Set(tx, true)
						break
					}
				}
				return !done.Get(tx)
			}) {
			}
		}()
		stm.Atomically(stm.VoidOperation(func(tx *stm.Tx) {
			tx.Assert(pending.Get(tx).Len() == 0)
		}))
		stm.AtomicSet(done, true)
	}
}
