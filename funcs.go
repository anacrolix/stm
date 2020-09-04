package stm

import (
	"math/rand"
	"runtime/pprof"
	"sync"
	"time"
)

var (
	txPool = sync.Pool{New: func() interface{} {
		expvars.Add("new txs", 1)
		tx := &Tx{
			reads:    make(map[*Var]VarValue),
			writes:   make(map[*Var]interface{}),
			watching: make(map[*Var]struct{}),
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
	return txPool.Get().(*Tx)
}

func WouldBlock(fn Operation) (block bool) {
	tx := newTx()
	tx.reset()
	_, block = catchRetry(fn, tx)
	return
}

// Atomically executes the atomic function fn.
func Atomically(op Operation) interface{} {
	expvars.Add("atomically", 1)
	// run the transaction
	tx := newTx()
	tx.tries = 0
retry:
	tx.tries++
	tx.reset()
	if sleepBetweenRetries {
		shift := int64(tx.tries - 1)
		const maxShift = 30
		if shift > maxShift {
			shift = maxShift
		}
		ns := int64(1) << shift
		ns = rand.Int63n(ns)
		if ns > 0 {
			tx.updateWatchers()
			time.Sleep(time.Duration(ns))
		}
	}
	ret, retry := catchRetry(op, tx)
	if retry {
		expvars.Add("retries", 1)
		// wait for one of the variables we read to change before retrying
		tx.wait()
		goto retry
	}
	// verify the read log
	tx.lockAllVars()
	if !tx.verify() {
		tx.unlock()
		expvars.Add("failed commits", 1)
		if profileFailedCommits {
			failedCommitsProfile.Add(new(int), 0)
		}
		goto retry
	}
	// commit the write log and broadcast that variables have changed
	tx.commit()
	tx.unlock()
	expvars.Add("commits", 1)
	tx.recycle()
	return ret
}

// AtomicGet is a helper function that atomically reads a value.
func AtomicGet(v *Var) interface{} {
	return v.value.Load().(VarValue).Get()
}

// AtomicSet is a helper function that atomically writes a value.
func AtomicSet(v *Var, val interface{}) {
	v.mu.Lock()
	v.changeValue(val)
	v.mu.Unlock()
}

// Compose is a helper function that composes multiple transactions into a
// single transaction.
func Compose(fns ...Operation) Operation {
	return func(tx *Tx) interface{} {
		for _, f := range fns {
			f(tx)
		}
		return nil
	}
}

// Select runs the supplied functions in order. Execution stops when a
// function succeeds without calling Retry. If no functions succeed, the
// entire selection will be retried.
func Select(fns ...Operation) Operation {
	return func(tx *Tx) interface{} {
		switch len(fns) {
		case 0:
			// empty Select blocks forever
			tx.Retry()
			panic("unreachable")
		case 1:
			return fns[0](tx)
		default:
			oldWrites := tx.writes
			tx.writes = make(map[*Var]interface{}, len(oldWrites))
			for k, v := range oldWrites {
				tx.writes[k] = v
			}
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

type Operation func(*Tx) interface{}

func VoidOperation(f func(*Tx)) Operation {
	return func(tx *Tx) interface{} {
		f(tx)
		return nil
	}
}
