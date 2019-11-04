package stm

import (
	"sync"
	"sync/atomic"
	"unsafe"
)

// Holds an STM variable.
type Var struct {
	state    *varSnapshot
	watchers sync.Map
	mu       sync.Mutex
}

func (v *Var) addr() *unsafe.Pointer {
	return (*unsafe.Pointer)(unsafe.Pointer(&v.state))
}

func (v *Var) loadState() *varSnapshot {
	return (*varSnapshot)(atomic.LoadPointer(v.addr()))
}

func (v *Var) changeValue(new interface{}) {
	version := v.loadState().version
	atomic.StorePointer(v.addr(), unsafe.Pointer(&varSnapshot{version: version + 1, val: new}))
}

type varSnapshot struct {
	val     interface{}
	version uint64
}

// Returns a new STM variable.
func NewVar(val interface{}) *Var {
	return &Var{
		state: &varSnapshot{
			val: val,
		},
	}
}
