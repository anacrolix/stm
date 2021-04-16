package stm

type VarValue interface {
	Set(interface{}) VarValue
	Get() interface{}
	Changed(VarValue) bool
}

type version uint64

type versionedValue struct {
	value   interface{}
	version version
}

func (me versionedValue) Set(newValue interface{}) VarValue {
	return versionedValue{
		value:   newValue,
		version: me.version + 1,
	}
}

func (me versionedValue) Get() interface{} {
	return me.value
}

func (me versionedValue) Changed(other VarValue) bool {
	return me.version != other.(versionedValue).version
}

type customVarValue[T any] struct {
	value   T
	changed func(T, T) bool
}

var _ VarValue = customVarValue[struct{}]{}

func (me customVarValue[T]) Changed(other VarValue) bool {
	return me.changed(me.value, other.(customVarValue[T]).value)
}

func (me customVarValue[T]) Set(newValue interface{}) VarValue {
	return customVarValue[T]{
		value:   newValue.(T),
		changed: me.changed,
	}
}

func (me customVarValue[T]) Get() interface{} {
	return me.value
}
