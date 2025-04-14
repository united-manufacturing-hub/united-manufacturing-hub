package deepcopy

import (
	"reflect"
)

// structFieldGetWithInit gets deep nested field with init value for pointer ones
func structFieldGetWithInit(field reflect.Value, index []int) reflect.Value {
	for _, idx := range index {
		if field.Kind() == reflect.Pointer {
			if field.IsNil() {
				field.Set(reflect.New(field.Type().Elem()))
			}
			field = field.Elem()
		}
		field = field.Field(idx)
	}
	return field
}

// structFieldSetZero sets zero to a deep nested field
func structFieldSetZero(field reflect.Value, index []int) {
	field, err := field.FieldByIndexErr(index)
	if err == nil && field.IsValid() {
		field.Set(reflect.Zero(field.Type())) // NOTE: Go1.18 has no SetZero
	}
}

// nillableValueSetNilOnZero sets value as `nil` when its inner value is zero.
// Only applies to `Pointer`, `Interface`, `Slice` and `Map` types.
func nillableValueSetNilOnZero(val reflect.Value) {
	innerVal := val
	for {
		switch innerVal.Kind() { //nolint:exhaustive
		case reflect.Pointer, reflect.Interface:
			innerVal = innerVal.Elem()
			if !innerVal.IsValid() || innerVal.IsZero() {
				val.Set(reflect.Zero(val.Type())) // NOTE: Go1.18 has no SetZero
				return
			}
		case reflect.Slice, reflect.Map:
			if innerVal.Len() == 0 {
				val.Set(reflect.Zero(val.Type())) // NOTE: Go1.18 has no SetZero
			}
			return // always return as we can't go deeper with a slice or map
		default:
			return
		}
	}
}
