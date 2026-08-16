package stm

import (
	"sync"
	"testing"
)

func BenchmarkAtomicGet(b *testing.B) {
	x := NewVar(0)
	for b.Loop() {
		AtomicGet(x)
	}
}

func BenchmarkAtomicSet(b *testing.B) {
	x := NewVar(0)
	for b.Loop() {
		AtomicSet(x, 0)
	}
}

func BenchmarkIncrementSTM(b *testing.B) {
	for b.Loop() {
		// spawn 1000 goroutines that each increment x by 1
		x := NewVar(0)
		for range 1000 {
			go Atomically(VoidOperation(func(tx *Tx) {
				cur := x.Get(tx)
				x.Set(tx, cur+1)
			}))
		}
		// wait for x to reach 1000
		Atomically(VoidOperation(func(tx *Tx) {
			tx.Assert(x.Get(tx) == 1000)
		}))
	}
}

func BenchmarkIncrementMutex(b *testing.B) {
	for b.Loop() {
		var mu sync.Mutex
		x := 0
		for range 1000 {
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
	for b.Loop() {
		c := make(chan int, 1)
		c <- 0
		for range 1000 {
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
	for b.Loop() {
		var wg sync.WaitGroup
		x := NewVar(0)
		for range 1000 {
			wg.Go(func() {
				AtomicGet(x)
			})
		}
		wg.Wait()
	}
}

func BenchmarkReadVarMutex(b *testing.B) {
	for b.Loop() {
		var mu sync.Mutex
		var wg sync.WaitGroup
		x := 0
		for range 1000 {
			wg.Go(func() {
				mu.Lock()
				_ = x
				mu.Unlock()
			})
		}
		wg.Wait()
	}
}

func BenchmarkReadVarChannel(b *testing.B) {
	for b.Loop() {
		var wg sync.WaitGroup
		c := make(chan int)
		close(c)
		for range 1000 {
			wg.Go(func() {
				<-c
			})
		}
		wg.Wait()
	}
}

func parallelPingPongs(b *testing.B, n int) {
	var wg sync.WaitGroup
	for range n {
		wg.Go(func() {
			testPingPong(b, b.N, func(string) {})
		})
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
