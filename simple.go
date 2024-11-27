// Package simple allows for structured, but schema-less values to be
// represented while constraining the possible types to a limited, knowable set.
package simple // import "code.nkcmr.net/simple"

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// Value is a way of having structured data with no specific schema. It mirrors
// JSON's limited type set. So, Value can only be one of the following:
// [Struct], [Array], [Number], [String], [Bool], or [Null].
type Value interface {
	xIsValue()
	String() string
}

// FromJSON will instantiate a Value based on JSON. The only possible failure is
// JSON syntax errors.
func FromJSON(jb json.RawMessage) (Value, error) {
	var anyv any
	if err := json.Unmarshal(jb, &anyv); err != nil {
		return nil, err
	}
	return fastFromValue(anyv), nil
}

// fastFromValue converts untyped data to simple values with assumptions that
// these values came straight from a json unmarshal
func fastFromValue(v any) Value {
	switch rv := v.(type) {
	case map[string]any:
		out := make(Struct, len(rv))
		for k, v := range rv {
			out[k] = fastFromValue(v)
		}
		return out
	case []any:
		out := make(Array, 0, len(rv))
		for _, v := range rv {
			out = append(out, fastFromValue(v))
		}
		return out
	case float64:
		return Number(rv)
	case bool:
		return Bool(rv)
	case string:
		return String(rv)
	case nil:
		return Null{}
	}
	panic(fmt.Sprintf("fastFromValue: unexpected type %T", v))
}

// FromValue allows any scalar or composite value to be simplified to a [Value].
//
// Things like channels, functions and interfaces do not represent transmittable
// values and therefore cannot be simplified.
//
// Any value that implements `SimpleValue() (Value, error)` or
// `SimpleValue() Value` can override some logic and handle value simplification
// on their own.
func FromValue(v any) (Value, error) {
	return fromReflectValue(reflect.ValueOf(v), []string{})
}

var builtinString = reflect.TypeFor[string]()
var builtinInt64 = reflect.TypeFor[int64]()
var builtinUint64 = reflect.TypeFor[uint64]()
var builtinFloat64 = reflect.TypeFor[float64]()
var builtinBool = reflect.TypeFor[bool]()
var structReflectType = reflect.TypeFor[Struct]()

func stringify(rt reflect.Type) func(reflect.Value) string {
	switch rt.Kind() {
	case reflect.String:
		return func(v reflect.Value) string {
			return v.Convert(builtinString).Interface().(string)
		}
	case reflect.Bool:
		return func(v reflect.Value) string {
			switch v.Interface() {
			case true:
				return "true"
			case false:
				return "false"
			}
			panic("impossible: stringify bool was not true or false")
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return func(v reflect.Value) string {
			realValue := v.Convert(builtinInt64).Interface().(int64)
			return strconv.FormatInt(realValue, 10)
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return func(v reflect.Value) string {
			realValue := v.Convert(builtinUint64).Interface().(uint64)
			return strconv.FormatUint(realValue, 10)
		}
	case reflect.Float32, reflect.Float64:
		return func(v reflect.Value) string {
			return fmt.Sprintf("%v", v.Interface())
		}
	}
	return nil
}

type fromValueError struct {
	path    []string
	problem string
}

func (f fromValueError) Error() string {
	return fmt.Sprintf("cannot convert value at %s: %s", strings.Join(f.path, ""), f.problem)
}

type fromValueWrappedError struct {
	error
	path []string
}

func (f fromValueWrappedError) Unwrap() error { return f.error }
func (f fromValueWrappedError) Error() string {
	return fmt.Sprintf("cannot convert value at %s: %s", strings.Join(f.path, ""), f.error.Error())
}

func fromReflectValue(anyv any, path []string) (Value, error) {
	switch sv := anyv.(type) {
	case interface{ SimpleValue() Value }:
		return sv.SimpleValue(), nil
	case interface{ SimpleValue() (Value, error) }:
		v, err := sv.SimpleValue()
		if err != nil {
			return nil, fromValueWrappedError{
				error: err,
				path:  path,
			}
		}
		return v, nil
	}

	if len(path) >= 1000 {
		panic("fromReflectValue: value too deep")
	}
	rv := reflect.ValueOf(anyv)
	rt := rv.Type()
	switch rt.Kind() {

	// composite types
	case reflect.Pointer:
		if rv.IsNil() {
			return Null{}, nil
		}
		return fromReflectValue(rv.Elem(), path)
	case reflect.Struct:
		outstruct := make(Struct, rt.NumField())
		for i := 0; i < rv.NumField(); i++ {
			key := rt.Field(i).Name
			value, err := fromReflectValue(rv.Field(i), append(path, ".", key))
			if err != nil {
				return nil, err
			}
			outstruct[key] = value
		}
		return outstruct, nil
	case reflect.Map:
		keytostr := stringify(rt.Key())
		if keytostr == nil {
			return nil, fromValueError{path: path, problem: fmt.Sprintf("map key with type %q cannot be stringified", rt.Key().String())}
		}
		outstruct := make(Struct, rv.Len())
		mapiter := rv.MapRange()
		for mapiter.Next() {
			key := mapiter.Key()
			keystr := keytostr(key)
			value := mapiter.Value()
			goodValue, err := fromReflectValue(value, append(path, ".", keystr))
			if err != nil {
				return nil, err
			}
			outstruct[keystr] = goodValue
		}
		return outstruct, nil
	case reflect.Array, reflect.Slice:
		outarray := make(Array, 0, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			v, err := fromReflectValue(rv.Index(i), append(path, fmt.Sprintf("[%d]", i)))
			if err != nil {
				return nil, err
			}
			outarray = append(outarray, v)
		}
		return outarray, nil

	// scalar types
	case reflect.String:
		fv := rv
		if rt != builtinString {
			fv = fv.Convert(builtinString)
		}
		return String(fv.Interface().(string)), nil

		// numbers
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64:
		fv := rv
		if rt != builtinFloat64 {
			fv = fv.Convert(builtinFloat64)
		}
		return Number(fv.Interface().(float64)), nil

	case reflect.Bool:
		if rt != builtinBool {
			return Bool(rv.Convert(builtinBool).Interface().(bool)), nil
		}
		return Bool(rv.Interface().(bool)), nil

	default:
		return nil, fromValueError{
			path:    path,
			problem: fmt.Sprintf("cannot convert value of kind %s to simple value", rv.Kind()),
		}
	}
}

func mustJSONEncodeValue(v Value) string {
	jb, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("mustJSONEncodeValue: json encode failed: %s", err.Error()))
	}
	return string(jb)
}

// Struct is a key value structure where keys are strings the are mapped to a
// [Value]
type Struct map[string]Value

func (Struct) xIsValue() {}

// String implements [Value]
func (s Struct) String() string {
	return mustJSONEncodeValue(s)
}

// Array is an ordered set of [Value] values
type Array []Value

func (Array) xIsValue() {}

// String implements [Value]
func (a Array) String() string {
	return mustJSONEncodeValue(a)
}

// Number is some numeric value. IEEE754 floating point number.
type Number float64

func (Number) xIsValue() {}

// String implements [Value]
func (n Number) String() string {
	return mustJSONEncodeValue(n)
}

// Bool is true of false
type Bool bool

func (Bool) xIsValue() {}

// String implements [Value]
func (b Bool) String() string {
	return mustJSONEncodeValue(b)
}

// Null is an intentionally missing value
type Null struct{}

func (Null) xIsValue() {}

func (Null) MarshalJSON() ([]byte, error) {
	return []byte("null"), nil
}

// String implements [Value]
func (n Null) String() string {
	return "null"
}

// String is an ordered set of UTF-8 characters.
type String string

func (String) xIsValue() {}

// String implements [Value]
func (s String) String() string {
	return mustJSONEncodeValue(s)
}
