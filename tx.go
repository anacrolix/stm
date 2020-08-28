package stm

import (
	"fmt"
	"sort"
	"sync"
	"unsafe"
)

// A Tx represents an atomic transaction.
type Tx struct {
	reads    map[*Var]uint64
	writes   map[*Var]interface{}
	watching map[*Var]struct{}
	locks    txLocks
	mu       sync.Mutex
	cond     sync.Cond
}

// Check that none of the logged values have changed since the transaction began.
func (tx *Tx) verify() bool {
	for v, version := range tx.reads {
		changed := v.loadState().version != version
		if changed {
			return false
		}
	}
	return true
}

// Writes the values in the transaction log to their respective Vars.
func (tx *Tx) commit() {
	for v, val := range tx.writes {
		v.changeValue(val)
	}
}

// wait blocks until another transaction modifies any of the Vars read by tx.
func (tx *Tx) wait() {
	if len(tx.reads) == 0 {
		panic("not waiting on anything")
	}
	for v := range tx.watching {
		if _, ok := tx.reads[v]; !ok {
			delete(tx.watching, v)
			v.watchers.Delete(tx)
		}
	}
	for v := range tx.reads {
		if _, ok := tx.watching[v]; !ok {
			v.watchers.Store(tx, nil)
			tx.watching[v] = struct{}{}
		}
	}
	tx.mu.Lock()
	firstWait := true
	for tx.verify() {
		if !firstWait {
			expvars.Add("wakes for unchanged versions", 1)
		}
		expvars.Add("waits", 1)
		tx.cond.Wait()
		firstWait = false
	}
	tx.mu.Unlock()
	//for v := range tx.reads {
	//	v.watchers.Delete(tx)
	//}
}

// Get returns the value of v as of the start of the transaction.
func (tx *Tx) Get(v *Var) interface{} {
	// If we previously wrote to v, it will be in the write log.
	if val, ok := tx.writes[v]; ok {
		return val
	}
	state := v.loadState()
	// If we haven't previously read v, record its version
	if _, ok := tx.reads[v]; !ok {
		tx.reads[v] = state.version
	}
	return state.val
}

// Set sets the value of a Var for the lifetime of the transaction.
func (tx *Tx) Set(v *Var, val interface{}) {
	if v == nil {
		panic("nil Var")
	}
	tx.writes[v] = val
}

// Retry aborts the transaction and retries it when a Var changes.
func (tx *Tx) Retry() {
	panic(Retry)
}

// Assert is a helper function that retries a transaction if the condition is
// not satisfied.
func (tx *Tx) Assert(p bool) {
	if !p {
		tx.Retry()
	}
}

func (tx *Tx) reset() {
	for k := range tx.reads {
		delete(tx.reads, k)
	}
	for k := range tx.writes {
		delete(tx.writes, k)
	}
	tx.resetLocks()
}

func (tx *Tx) recycle() {
	for v := range tx.watching {
		delete(tx.watching, v)
		v.watchers.Delete(tx)
	}
	txPool.Put(tx)
}

func (tx *Tx) lockAllVars() {
	tx.resetLocks()
	tx.collectAllLocks()
	tx.sortLocks()
	tx.lock()
}

func (tx *Tx) resetLocks() {
	tx.locks.clear()
}

func (tx *Tx) collectReadLocks() {
	for v := range tx.reads {
		tx.locks.append(&v.mu)
	}
}

func (tx *Tx) collectAllLocks() {
	tx.collectReadLocks()
	for v := range tx.writes {
		if _, ok := tx.reads[v]; !ok {
			tx.locks.append(&v.mu)
		}
	}
}

func (tx *Tx) sortLocks() {
	sort.Sort(&tx.locks)
}

func (tx *Tx) lock() {
	for _, l := range tx.locks.mus {
		l.Lock()
	}
}

func (tx *Tx) unlock() {
	for _, l := range tx.locks.mus {
		l.Unlock()
	}
}

func (tx *Tx) String() string {
	return fmt.Sprintf("%[1]T %[1]p", tx)
}

// Dedicated type avoids reflection in sort.Slice.
type txLocks struct {
	mus []*sync.Mutex
}

func (me txLocks) Len() int {
	return len(me.mus)
}

func (me txLocks) Less(i, j int) bool {
	return uintptr(unsafe.Pointer(me.mus[i])) < uintptr(unsafe.Pointer(me.mus[j]))
}

func (me txLocks) Swap(i, j int) {
	me.mus[i], me.mus[j] = me.mus[j], me.mus[i]
}

func (me *txLocks) clear() {
	me.mus = me.mus[:0]
}

func (me *txLocks) append(mu *sync.Mutex) {
	me.mus = append(me.mus, mu)
}
