// Package gherkin provides helpers for godog step definitions.
package gherkin

import (
	"fmt"
	"reflect"
	"strconv"

	"github.com/cucumber/godog"
)

// DecodeTable fills dst from a two-column Gherkin table. Struct fields opt in
// with a `table:"key"` tag; slice fields collect repeated keys.
func DecodeTable(tbl *godog.Table, dst any) error {
	value := reflect.ValueOf(dst)
	if value.Kind() != reflect.Ptr || value.IsNil() {
		return fmt.Errorf("decode table: dst must be a non-nil pointer")
	}
	value = value.Elem()
	if value.Kind() != reflect.Struct {
		return fmt.Errorf("decode table: dst must point to a struct")
	}

	fields := make(map[string]reflect.Value, value.NumField())
	for i := 0; i < value.NumField(); i++ {
		key := value.Type().Field(i).Tag.Get("table")
		if key != "" {
			fields[key] = value.Field(i)
		}
	}

	seen := make(map[string]bool, len(fields))
	for _, row := range tbl.Rows {
		if len(row.Cells) != 2 {
			return fmt.Errorf("table row has %d cells, want 2", len(row.Cells))
		}
		key, raw := row.Cells[0].Value, row.Cells[1].Value
		field, ok := fields[key]
		if !ok {
			return fmt.Errorf("unknown table key %q", key)
		}
		if seen[key] && field.Kind() != reflect.Slice {
			return fmt.Errorf("duplicate table key %q", key)
		}
		seen[key] = true
		if err := setField(field, raw); err != nil {
			return fmt.Errorf("%s %q: %w", key, raw, err)
		}
	}
	return nil
}

func setField(field reflect.Value, raw string) error {
	switch field.Kind() {
	case reflect.String:
		field.SetString(raw)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		value, err := strconv.ParseInt(raw, 10, field.Type().Bits())
		if err != nil {
			return err
		}
		field.SetInt(value)
	case reflect.Bool:
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return err
		}
		field.SetBool(value)
	case reflect.Ptr:
		if field.IsNil() {
			field.Set(reflect.New(field.Type().Elem()))
		}
		return setField(field.Elem(), raw)
	case reflect.Slice:
		if field.Type().Elem().Kind() != reflect.String {
			return fmt.Errorf("unsupported slice element %s", field.Type().Elem())
		}
		field.Set(reflect.Append(field, reflect.ValueOf(raw).Convert(field.Type().Elem())))
	default:
		return fmt.Errorf("unsupported field type %s", field.Type())
	}
	return nil
}
