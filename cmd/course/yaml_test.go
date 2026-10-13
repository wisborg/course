package main

import (
	"reflect"
	"testing"
)

// Every field the commands write as JSON is named the same in YAML. The YAML
// library does not read a field's JSON name, and a field without a YAML name
// of its own was written as the Go name lowered -- from_m as "fromm", to_s as
// "tos" -- so the two formats named the same data differently. Walking every
// output type, nested ones included, catches a field added later with a JSON
// name and no YAML one.
func TestYAMLNamesAreJSONNames(t *testing.T) {
	seen := map[reflect.Type]bool{}
	var check func(reflect.Type)
	check = func(ty reflect.Type) {
		for ty.Kind() == reflect.Pointer || ty.Kind() == reflect.Slice || ty.Kind() == reflect.Array || ty.Kind() == reflect.Map {
			ty = ty.Elem()
		}
		if ty.Kind() != reflect.Struct || seen[ty] {
			return
		}
		seen[ty] = true
		for i := range ty.NumField() {
			f := ty.Field(i)
			if j, ok := f.Tag.Lookup("json"); ok {
				if y, _ := f.Tag.Lookup("yaml"); y != j {
					t.Errorf("%s.%s is %q in JSON and %q in YAML", ty.Name(), f.Name, j, y)
				}
			}
			check(f.Type)
		}
	}
	for _, v := range []any{jsonMatchReport{}, jsonComparisonDoc{}, jsonDoc{}} {
		check(reflect.TypeOf(v))
	}
	if len(seen) < 6 {
		t.Errorf("walked %d output types; the commands write more", len(seen))
	}
}
