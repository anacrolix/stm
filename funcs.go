package stm

import (
	"maps"
	"math/rand/v2"
	"runtime/pprof"
	"sync"
	"time"
)

var (
	txPool = sync.Pool{New: func() any {
		expvars.Add("new txs", 1)
		tx := &Tx{
			reads:    make(map[txVar]VarValue),
			writes:   make(map[txVar]any),
			watching: make(map[txVar]struct{}),
		}
		tx.cond.L = &tx.mu
		return tx
	}}
	failedCommitsProfile *pprof.Profile
)

const (
	profileFailedCommits = false
	sleepBetweenRetries  = false
)

func init() {
	if profileFailedCommits {
		failedCommitsProfile = pprof.NewProfile("stmFailedCommits")
	}
}

func newTx() *Tx {
	tx := txPool.Get().(*Tx)
	tx.tries = 0
	tx.completed = false
	return tx
}

func WouldBlock[R any](fn Operation[R]) (block bool) {
	tx := newTx()
	tx.reset()
	_, block = catchRetry(fn, tx)
	if len(tx.watching) != 0 {
		panic("shouldn't have installed any watchers")
	}
	tx.recycle()
	return
}

// Atomically executes the atomic function fn.
func Atomically[R any](op Operation[R]) R {
	expvars.Add("atomically", 1)
	// run the transaction
	tx := newTx()
	// A panic that isn't the retry sentinel leaves through here, and the transaction has to stop
	// watching the Vars it read on the way out. A transaction left in a Var's watchers is never
	// going to wait or complete, so every later write to that Var strands a wakeWatchers goroutine
	// on it, and that goroutine blocks the rest of the watchers from being woken at all.
	defer tx.recycle()
retry:
	tx.tries++
	tx.reset()
	if sleepBetweenRetries {
		const maxShift = 30
		shift := min(int64(tx.tries-1), maxShift)
		ns := int64(1) << shift
		d := time.Duration(rand.Int64N(ns))
		if d > 100*time.Microsecond {
			tx.updateWatchers()
			time.Sleep(time.Duration(ns))
		}
	}
	ret, retry := func() (R, bool) {
		// Deferred, so that a panic escaping the operation doesn't leave the transaction locked
		// against the wakeWatchers that want to look at its read log.
		tx.mu.Lock()
		defer tx.mu.Unlock()
		return catchRetry(op, tx)
	}()
	if retry {
		expvars.Add("retries", 1)
		// wait for one of the variables we read to change before retrying
		tx.wait()
		goto retry
	}
	committed := func() bool {
		// verify the read log
		tx.lockAllVars()
		// Deferred, because everything below holds the lock on every Var the transaction touched,
		// and includes calls out to the comparisons a custom Var was made with. A panic in there
		// would otherwise leave those Vars locked against every future transaction.
		defer tx.unlock()
		if tx.inputsChanged() {
			expvars.Add("failed commits", 1)
			if profileFailedCommits {
				failedCommitsProfile.Add(new(int), 0)
			}
			return false
		}
		// commit the write log and broadcast that variables have changed
		tx.commit()
		tx.mu.Lock()
		tx.completed = true
		tx.cond.Broadcast()
		tx.mu.Unlock()
		return true
	}()
	if !committed {
		goto retry
	}
	expvars.Add("commits", 1)
	return ret
}

// AtomicGet is a helper function that atomically reads a value.
func AtomicGet[T any](v *Var[T]) T {
	return fromAny[T](v.value.Load().Get())
}

// AtomicSet is a helper function that atomically writes a value.
func AtomicSet[T any](v *Var[T], val T) {
	v.mu.Lock()
	v.changeValue(val)
	v.mu.Unlock()
}

// Compose is a helper function that composes multiple transactions into a
// single transaction.
func Compose[R any](fns ...Operation[R]) Operation[struct{}] {
	return VoidOperation(func(tx *Tx) {
		for _, f := range fns {
			f(tx)
		}
	})
}

// Select runs the supplied functions in order. Execution stops when a
// function succeeds without calling Retry. If no functions succeed, the
// entire selection will be retried.
func Select[R any](fns ...Operation[R]) Operation[R] {
	return func(tx *Tx) R {
		switch len(fns) {
		case 0:
			// empty Select blocks forever
			tx.Retry()
			panic("unreachable")
		case 1:
			return fns[0](tx)
		default:
			oldWrites := tx.writes
			tx.writes = maps.Clone(oldWrites)
			ret, retry := catchRetry(fns[0], tx)
			if retry {
				tx.writes = oldWrites
				return Select(fns[1:]...)(tx)
			} else {
				return ret
			}
		}
	}
}

type Operation[R any] func(*Tx) R

func VoidOperation(f func(*Tx)) Operation[struct{}] {
	return func(tx *Tx) struct{} {
		f(tx)
		return struct{}{}
	}
}

func AtomicModify[T any](v *Var[T], f func(T) T) {
	Atomically(VoidOperation(func(tx *Tx) {
		v.Set(tx, f(v.Get(tx)))
	}))
}
