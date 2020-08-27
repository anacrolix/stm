package stm

import (
	"sync"
	"testing"

	"github.com/anacrolix/missinggo/iter"
)

func BenchmarkAtomicGet(b *testing.B) {
	x := NewVar(0)
	for i := 0; i < b.N; i++ {
		AtomicGet(x)
	}
}

func BenchmarkAtomicSet(b *testing.B) {
	x := NewVar(0)
	for i := 0; i < b.N; i++ {
		AtomicSet(x, 0)
	}
}

func BenchmarkIncrementSTM(b *testing.B) {
	for i := 0; i < b.N; i++ {
		// spawn 1000 goroutines that each increment x by 1
		x := NewVar(0)
		for i := 0; i < 1000; i++ {
			go Atomically(VoidOperation(func(tx *Tx) {
				cur := tx.Get(x).(int)
				tx.Set(x, cur+1)
			}))
		}
		// wait for x to reach 1000
		Atomically(VoidOperation(func(tx *Tx) {
			tx.Assert(tx.Get(x).(int) == 1000)
		}))
	}
}

func BenchmarkIncrementMutex(b *testing.B) {
	for i := 0; i < b.N; i++ {
		var mu sync.Mutex
		x := 0
		for i := 0; i < 1000; i++ {
			go func() {
				mu.Lock()
				x++
				mu.Unlock()
			}()
		}
		for {
			mu.Lock()
			read := x
			mu.Unlock()
			if read == 1000 {
				break
			}
		}
	}
}

func BenchmarkIncrementChannel(b *testing.B) {
	for i := 0; i < b.N; i++ {
		c := make(chan int, 1)
		c <- 0
		for i := 0; i < 1000; i++ {
			go func() {
				c <- 1 + <-c
			}()
		}
		for {
			read := <-c
			if read == 1000 {
				break
			}
			c <- read
		}
	}
}

func BenchmarkReadVarSTM(b *testing.B) {
	for i := 0; i < b.N; i++ {
		var wg sync.WaitGroup
		wg.Add(1000)
		x := NewVar(0)
		for i := 0; i < 1000; i++ {
			go func() {
				AtomicGet(x)
				wg.Done()
			}()
		}
		wg.Wait()
	}
}

func BenchmarkReadVarMutex(b *testing.B) {
	for i := 0; i < b.N; i++ {
		var mu sync.Mutex
		var wg sync.WaitGroup
		wg.Add(1000)
		x := 0
		for i := 0; i < 1000; i++ {
			go func() {
				mu.Lock()
				_ = x
				mu.Unlock()
				wg.Done()
			}()
		}
		wg.Wait()
	}
}

func BenchmarkReadVarChannel(b *testing.B) {
	for i := 0; i < b.N; i++ {
		var wg sync.WaitGroup
		wg.Add(1000)
		c := make(chan int)
		close(c)
		for i := 0; i < 1000; i++ {
			go func() {
				<-c
				wg.Done()
			}()
		}
		wg.Wait()
	}
}

func parallelPingPongs(b *testing.B, n int) {
	var wg sync.WaitGroup
	wg.Add(n)
	for range iter.N(n) {
		go func() {
			defer wg.Done()
			testPingPong(b, b.N, func(string) {})
		}()
	}
	wg.Wait()
}

func BenchmarkPingPong4(b *testing.B) {
	b.ReportAllocs()
	parallelPingPongs(b, 4)
}

func BenchmarkPingPong(b *testing.B) {
	b.ReportAllocs()
	parallelPingPongs(b, 1)
}

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
		done := NewVar(false)
		tokens := NewVar(0)
		pending := NewVar(0)
		for range iter.N(1000) {
			Atomically(VoidOperation(func(tx *Tx) {
				tx.Set(pending, tx.Get(pending).(int)+1)
			}))
			go func() {
				Atomically(VoidOperation(func(tx *Tx) {
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
			for Atomically(func(tx *Tx) interface{} {
				if tx.Get(done).(bool) {
					return false
				}
				tx.Assert(tx.Get(tokens).(int) < maxTokens)
				tx.Set(tokens, tx.Get(tokens).(int)+1)
				return true
			}).(bool) {
			}
		}()
		Atomically(VoidOperation(func(tx *Tx) {
			tx.Assert(tx.Get(pending).(int) == 0)
		}))
		AtomicSet(done, true)
	}
}
