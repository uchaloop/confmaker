// Package filedecode contains decode hooks shared by confmaker's file loaders.
package filedecode

import (
	"reflect"

	"github.com/go-viper/mapstructure/v2"
	"github.com/uchaloop/secret/v2"
)

var secretValueType = reflect.TypeOf((*secret.Value)(nil)).Elem()

// ZeroSecrets clears supported exported configuration values marked as
// secret.Value before file decoding. Unexported fields and map keys are outside
// the configuration schema and are left untouched.
func ZeroSecrets(dst any) {
	zeroSecrets(reflect.ValueOf(dst), make(map[visit]bool))
}

// Hooks returns the decode hooks used for configuration files. Values targeting
// a secret.Value are discarded: secrets may be supplied by environment loading,
// but a file can never populate them.
func Hooks() mapstructure.DecodeHookFunc {
	return mapstructure.ComposeDecodeHookFunc(
		ignoreSecret(),
		mapstructure.StringToTimeDurationHookFunc(),
	)
}

func ignoreSecret() mapstructure.DecodeHookFuncType {
	return func(_ reflect.Type, to reflect.Type, data any) (any, error) {
		if isSecretType(to) {
			return reflect.Zero(to).Interface(), nil
		}

		return data, nil
	}
}

type visit struct {
	typ reflect.Type
	ptr uintptr
	len int
}

func zeroSecrets(value reflect.Value, seen map[visit]bool) {
	if !value.IsValid() {
		return
	}
	if isSecretType(value.Type()) {
		if value.CanSet() {
			value.SetZero()
		}

		return
	}

	switch value.Kind() {
	case reflect.Interface:
		if value.IsNil() {
			return
		}

		elem := reflect.New(value.Elem().Type()).Elem()
		elem.Set(value.Elem())
		zeroSecrets(elem, seen)
		if value.CanSet() {
			value.Set(elem)
		}
	case reflect.Pointer:
		if value.IsNil() {
			return
		}

		key := visit{typ: value.Type(), ptr: value.Pointer()}
		if seen[key] {
			return
		}
		seen[key] = true
		zeroSecrets(value.Elem(), seen)
	case reflect.Struct:
		for i := range value.NumField() {
			if !value.Type().Field(i).IsExported() {
				continue
			}
			zeroSecrets(value.Field(i), seen)
		}
	case reflect.Slice:
		if value.IsNil() {
			return
		}

		key := visit{typ: value.Type(), ptr: value.Pointer(), len: value.Len()}
		if seen[key] {
			return
		}
		seen[key] = true
		for i := range value.Len() {
			zeroSecrets(value.Index(i), seen)
		}
	case reflect.Array:
		for i := range value.Len() {
			zeroSecrets(value.Index(i), seen)
		}
	case reflect.Map:
		if value.IsNil() {
			return
		}

		key := visit{typ: value.Type(), ptr: value.Pointer()}
		if seen[key] {
			return
		}
		seen[key] = true
		iter := value.MapRange()
		for iter.Next() {
			item := reflect.New(value.Type().Elem()).Elem()
			item.Set(iter.Value())
			zeroSecrets(item, seen)
			value.SetMapIndex(iter.Key(), item)
		}
	}
}

func isSecretType(valueType reflect.Type) bool {
	return valueType.Kind() != reflect.Pointer && valueType.Implements(secretValueType)
}
