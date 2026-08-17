package stm

import (
	"errors"
	"testing"
	"time"

	qt "github.com/go-quicktest/qt"
)

var anError = errors.New("an error")

func checkNoPanic(t *testing.T, f func()) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("unexpected panic: %v", r)
		}
	}()
	f()
}

// The value is stored in the VarValue as an any, so a nil interface becomes a
// nil any, and the type assertions that get it back out again fail.
func TestNilInterfaceVarAtomicGet(t *testing.T) {
	v := NewVar[error](nil)
	checkNoPanic(t, func() {
		qt.Check(t, qt.IsNil(AtomicGet(v)))
	})
}

func TestNilInterfaceVarGet(t *testing.T) {
	v := NewVar[error](nil)
	checkNoPanic(t, func() {
		Atomically(VoidOperation(func(tx *Tx) {
			qt.Check(t, qt.IsNil(v.Get(tx)))
		}))
	})
}

// Reading back a nil written earlier in the same transaction reads it from the
// write log, which is also an any.
func TestNilInterfaceVarReadOwnWrite(t *testing.T) {
	v := NewVar[error](anError)
	checkNoPanic(t, func() {
		Atomically(VoidOperation(func(tx *Tx) {
			v.Set(tx, nil)
			qt.Check(t, qt.IsNil(v.Get(tx)))
		}))
	})
}

// changeValue asserts the new value's type while committing.
func TestNilInterfaceVarSet(t *testing.T) {
	v := NewVar[error](anError)
	checkNoPanic(t, func() {
		Atomically(VoidOperation(func(tx *Tx) {
			v.Set(tx, nil)
		}))
	})
}

// commit runs between lockAllVars and unlock, so the type assertion it panics
// on leaves every Var in the transaction locked for good.
func TestPanicInCommitDoesNotStrandVarLocks(t *testing.T) {
	v := NewVar[error](anError)
	other := NewVar(1)
	func() {
		defer func() { recover() }()
		Atomically(VoidOperation(func(tx *Tx) {
			other.Set(tx, other.Get(tx)+1)
			v.Set(tx, nil)
		}))
	}()
	done := make(chan struct{})
	go func() {
		AtomicSet(other, 99)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Var is still locked after a panic in commit")
	}
}
