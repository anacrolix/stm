package stm

import "sync"

// A Var holds an STM variable.
type Var struct {
	val      interface{}
	version  uint64
	mu       sync.Mutex
	watchers map[*Tx]struct{}
}

// NewVar returns a new STM variable.
func NewVar(val interface{}) *Var {
	return &Var{
		val:      val,
		watchers: make(map[*Tx]struct{}),
	}
}
