package frame

import "reflect"

func isNilPointer(message any) bool {
	value := reflect.ValueOf(message)
	return value.Kind() == reflect.Pointer && value.IsNil()
}
