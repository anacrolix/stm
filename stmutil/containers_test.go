package stmutil

import (
	"os"
	"os/exec"
	"testing"
)

// The hashed containers can fault the process rather than fail a test, so run
// each case in a subprocess and report its output.
func runIsolated(t *testing.T, f func(t *testing.T)) {
	name := t.Name()
	if os.Getenv("STM_ISOLATED_TEST") == name {
		f(t)
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^"+name+"$", "-test.v")
	cmd.Env = append(os.Environ(), "STM_ISOLATED_TEST="+name)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("isolated run of %v failed: %v\n%s", name, err, out)
	}
}

func TestMapIntKeys(t *testing.T) {
	runIsolated(t, func(t *testing.T) {
		m := NewMap[int, string]()
		for i := 0; i < 1000; i++ {
			m = m.Set(i, "value")
		}
		if m.Len() != 1000 {
			t.Fatalf("got %v keys, want 1000", m.Len())
		}
		for i := 0; i < 1000; i++ {
			if v, ok := m.Get(i); !ok || v != "value" {
				t.Fatalf("Get(%v) = %q, %v", i, v, ok)
			}
		}
		if _, ok := m.Get(1000); ok {
			t.Fatal("Get returned a key that was never set")
		}
		m = m.Delete(500)
		if _, ok := m.Get(500); ok {
			t.Fatal("deleted key is still present")
		}
	})
}

func TestMapStringKeys(t *testing.T) {
	runIsolated(t, func(t *testing.T) {
		m := NewMap[string, int]()
		m = m.Set("a", 1)
		m = m.Set("b", 2)
		if v, ok := m.Get("b"); !ok || v != 2 {
			t.Fatalf(`Get("b") = %v, %v`, v, ok)
		}
	})
}

func TestMapPointerKeys(t *testing.T) {
	runIsolated(t, func(t *testing.T) {
		type foo struct{ i int }
		m := NewMap[*foo, int]()
		keys := make([]*foo, 0, 100)
		for i := 0; i < 100; i++ {
			k := &foo{i}
			keys = append(keys, k)
			m = m.Set(k, i)
		}
		for i, k := range keys {
			if v, ok := m.Get(k); !ok || v != i {
				t.Fatalf("Get(keys[%v]) = %v, %v", i, v, ok)
			}
		}
		if _, ok := m.Get(&foo{0}); ok {
			t.Fatal("a distinct pointer with an equal pointee matched")
		}
	})
}

// Set is a mapToSet, so it hashes its keys the same way Map does. This mirrors
// the usage in BenchmarkInvertedThunderingHerd.
func TestSetPointerKeys(t *testing.T) {
	runIsolated(t, func(t *testing.T) {
		type foo struct{ i int }
		s := NewSet[*foo]()
		a, b := &foo{1}, &foo{2}
		s = s.Add(a)
		if !s.Contains(a) {
			t.Fatal("set does not contain the value just added")
		}
		if s.Contains(b) {
			t.Fatal("set contains a value that was never added")
		}
		if s.Len() != 1 {
			t.Fatalf("got len %v, want 1", s.Len())
		}
		if s = s.Delete(a); s.Contains(a) {
			t.Fatal("deleted value is still present")
		}
	})
}

// SortedMap orders keys with a comparer instead of hashing them, so it is
// unaffected by the hashing of the unsorted containers.
func TestSortedMapIntKeys(t *testing.T) {
	runIsolated(t, func(t *testing.T) {
		m := NewSortedMap[int, string](func(l, r int) bool { return l < r })
		for i := 0; i < 100; i++ {
			m = m.Set(i, "value")
		}
		if v, ok := m.Get(42); !ok || v != "value" {
			t.Fatalf("Get(42) = %q, %v", v, ok)
		}
	})
}
