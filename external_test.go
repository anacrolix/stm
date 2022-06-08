package stm_test

import (
	"sync"
	"testing"

	"github.com/anacrolix/missinggo/iter"
	"github.com/anacrolix/stm"
	"github.com/anacrolix/stm/stmutil"
)

const maxTokens = 25

func BenchmarkThunderingHerdCondVar(b *testing.B) {
	for i := 0; i < b.N; i++ {
		var mu sync.Mutex
		consumer := sync.NewCond(&mu)
		generator := sync.NewCond(&mu)
		done := false
		tokens := 0
		var pending sync.WaitGroup
		for range iter.N(1000) {
			pending.Add(1)
			go func() {
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
				pending.Done()
			}()
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
	for i := 0; i < b.N; i++ {
		done := stm.NewBuiltinEqVar(false)
		tokens := stm.NewBuiltinEqVar(0)
		pending := stm.NewBuiltinEqVar(0)
		for range iter.N(1000) {
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
	for i := 0; i < b.N; i++ {
		done := stm.NewBuiltinEqVar(false)
		tokens := stm.NewBuiltinEqVar(0)
		pending := stm.NewVar(stmutil.NewSet())
		for range iter.N(1000) {
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
				pending.Get(tx).Range(func(i any) bool {
					ready := i.(*stm.Var[bool])
					if !ready.Get(tx) {
						ready.Set(tx, true)
						return false
					}
					return true
				})
				return !done.Get(tx)
			}) {
			}
		}()
		stm.Atomically(stm.VoidOperation(func(tx *stm.Tx) {
			tx.Assert(pending.Get(tx).(stmutil.Lenner).Len() == 0)
		}))
		stm.AtomicSet(done, true)
	}
}
