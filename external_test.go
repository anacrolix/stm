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
				tx.Set(pending, tx.Get(pending).(int)+1)
			}))
			go func() {
				stm.Atomically(stm.VoidOperation(func(tx *stm.Tx) {
					t := tx.Get(tokens).(int)
					if t > 0 {
						tx.Set(tokens, t-1)
						tx.Set(pending, tx.Get(pending).(int)-1)
					} else {
						tx.Retry()
					}
				}))
			}()
		}
		go func() {
			for stm.Atomically(func(tx *stm.Tx) interface{} {
				if tx.Get(done).(bool) {
					return false
				}
				tx.Assert(tx.Get(tokens).(int) < maxTokens)
				tx.Set(tokens, tx.Get(tokens).(int)+1)
				return true
			}).(bool) {
			}
		}()
		stm.Atomically(stm.VoidOperation(func(tx *stm.Tx) {
			tx.Assert(tx.Get(pending).(int) == 0)
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
				tx.Set(pending, tx.Get(pending).(stmutil.Settish).Add(ready))
			}))
			go func() {
				stm.Atomically(stm.VoidOperation(func(tx *stm.Tx) {
					tx.Assert(tx.Get(ready).(bool))
					set := tx.Get(pending).(stmutil.Settish)
					if !set.Contains(ready) {
						panic("couldn't find ourselves in pending")
					}
					tx.Set(pending, set.Delete(ready))
				}))
				//b.Log("waiter finished")
			}()
		}
		go func() {
			for stm.Atomically(func(tx *stm.Tx) interface{} {
				if tx.Get(done).(bool) {
					return false
				}
				tx.Assert(tx.Get(tokens).(int) < maxTokens)
				tx.Set(tokens, tx.Get(tokens).(int)+1)
				return true
			}).(bool) {
			}
		}()
		go func() {
			for stm.Atomically(func(tx *stm.Tx) interface{} {
				tx.Assert(tx.Get(tokens).(int) > 0)
				tx.Set(tokens, tx.Get(tokens).(int)-1)
				tx.Get(pending).(stmutil.Settish).Range(func(i interface{}) bool {
					ready := i.(*stm.Var)
					if !tx.Get(ready).(bool) {
						tx.Set(ready, true)
						return false
					}
					return true
				})
				return !tx.Get(done).(bool)
			}).(bool) {
			}
		}()
		stm.Atomically(stm.VoidOperation(func(tx *stm.Tx) {
			tx.Assert(tx.Get(pending).(stmutil.Lenner).Len() == 0)
		}))
		stm.AtomicSet(done, true)
	}
}
