package stm

type VarValue interface {
	Set(any) VarValue
	Get() any
	Changed(VarValue) bool
}

type version uint64

// Values are held in an any so that a Tx can log Vars of every type together. A T that is an
// interface type holding nil boxes into a nil any, which a plain type assertion rejects, so use the
// comma-ok form and let the zero value stand for it. A Var[T] is only ever given a T, so there is
// no other way for the assertion to fail.
func fromAny[T any](value any) T {
	t, _ := value.(T)
	return t
}

type versionedValue[T any] struct {
	value   T
	version version
}

func (me versionedValue[T]) Set(newValue any) VarValue {
	return versionedValue[T]{
		value:   fromAny[T](newValue),
		version: me.version + 1,
	}
}

func (me versionedValue[T]) Get() any {
	return me.value
}

func (me versionedValue[T]) Changed(other VarValue) bool {
	return me.version != other.(versionedValue[T]).version
}

type customVarValue[T any] struct {
	value   T
	changed func(T, T) bool
}

var _ VarValue = customVarValue[struct{}]{}

func (me customVarValue[T]) Changed(other VarValue) bool {
	return me.changed(me.value, other.(customVarValue[T]).value)
}

func (me customVarValue[T]) Set(newValue any) VarValue {
	return customVarValue[T]{
		value:   fromAny[T](newValue),
		changed: me.changed,
	}
}

func (me customVarValue[T]) Get() any {
	return me.value
}
