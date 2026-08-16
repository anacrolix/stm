package stm

import "sync/atomic"

// A typed wrapper around sync/atomic.Value. This exists because the standard library still has no
// generic atomic.Value: sync/atomic gained typed Pointer/Int64/etc. in Go 1.19, but a generic Value
// for interface payloads is not among them. It previously came from github.com/alecthomas/atomic,
// which is a wrapper of the same shape.
//
// The same constraint as sync/atomic.Value applies: all values stored in one atomicValue must have
// the same concrete type.
type atomicValue[T any] struct {
	value atomic.Value
}

func (v *atomicValue[T]) Load() (out T) {
	value := v.value.Load()
	if value == nil {
		return
	}
	return value.(T)
}

func (v *atomicValue[T]) Store(value T) {
	v.value.Store(value)
}
