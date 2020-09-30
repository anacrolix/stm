package stm

import (
	"sync"
	"sync/atomic"
)

// Holds an STM variable.
type Var struct {
	value    atomic.Value
	watchers sync.Map
	mu       sync.Mutex
}

func (v *Var) changeValue(new interface{}) {
	old := v.value.Load().(VarValue)
	newVarValue := old.Set(new)
	v.value.Store(newVarValue)
	if old.Changed(newVarValue) {
		v.wakeWatchers()
	}
}

func (v *Var) wakeWatchers() {
	v.watchers.Range(func(k, _ interface{}) bool {
		tx := k.(*Tx)

		// We have to lock here to ensure that the Tx is waiting before we signal it. Otherwise we
		// could signal it before it goes to sleep and it will miss the notification.
		tx.mu.Lock()
		tx.cond.Broadcast()
		tx.mu.Unlock()
		return true
	})
}

type varSnapshot struct {
	val     interface{}
	version uint64
}

// Returns a new STM variable.
func NewVar(val interface{}) *Var {
	v := &Var{}
	v.value.Store(versionedValue{
		value: val,
	})
	return v
}

func NewCustomVar(val interface{}, changed func(interface{}, interface{}) bool) *Var {
	v := &Var{}
	v.value.Store(customVarValue{
		value:   val,
		changed: changed,
	})
	return v
}

func NewBuiltinEqVar(val interface{}) *Var {
	return NewCustomVar(val, func(a, b interface{}) bool {
		return a != b
	})
}
