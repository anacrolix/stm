package stm

import (
	"runtime/pprof"
	"sync"
)

var (
	txPool = sync.Pool{New: func() interface{} {
		expvars.Add("new txs", 1)
		tx := &Tx{
			reads:  make(map[*Var]uint64),
			writes: make(map[*Var]interface{}),
		}
		tx.cond.L = &tx.mu
		return tx
	}}
	failedCommitsProfile *pprof.Profile
)

const profileFailedCommits = false

func init() {
	if profileFailedCommits {
		failedCommitsProfile = pprof.NewProfile("stmFailedCommits")
	}
}

func newTx() *Tx {
	return txPool.Get().(*Tx)
}

func WouldBlock(fn func(*Tx)) (block bool) {
	tx := newTx()
	tx.reset()
	defer func() {
		if r := recover(); r == Retry {
			block = true
		} else if _, ok := r.(_return); ok {
		} else if r != nil {
			panic(r)
		}
	}()
	return catchRetry(fn, tx)
}

// Atomically executes the atomic function fn.
func Atomically(fn func(*Tx)) interface{} {
	expvars.Add("atomically", 1)
	// run the transaction
	tx := newTx()
retry:
	tx.reset()
	var ret interface{}
	if func() (retry bool) {
		defer func() {
			r := recover()
			if r == nil {
				return
			}
			if _ret, ok := r.(_return); ok {
				expvars.Add("explicit returns", 1)
				ret = _ret.value
			} else if r == Retry {
				expvars.Add("retries", 1)
				// wait for one of the variables we read to change before retrying
				tx.wait()
				retry = true
			} else {
				panic(r)
			}
		}()
		fn(tx)
		return false
	}() {
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
	return v.loadState().val
}

// AtomicSet is a helper function that atomically writes a value.
func AtomicSet(v *Var, val interface{}) {
	v.mu.Lock()
	v.changeValue(val)
	v.mu.Unlock()
}

// Compose is a helper function that composes multiple transactions into a
// single transaction.
func Compose(fns ...func(*Tx)) func(*Tx) {
	return func(tx *Tx) {
		for _, f := range fns {
			f(tx)
		}
	}
}

// Select runs the supplied functions in order. Execution stops when a
// function succeeds without calling Retry. If no functions succeed, the
// entire selection will be retried.
func Select(fns ...func(*Tx)) func(*Tx) {
	return func(tx *Tx) {
		switch len(fns) {
		case 0:
			// empty Select blocks forever
			tx.Retry()
		case 1:
			fns[0](tx)
		default:
			if catchRetry(fns[0], tx) {
				Select(fns[1:]...)(tx)
			}
		}
	}
}
