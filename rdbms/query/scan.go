package query

import (
	"database/sql"
	"fmt"
	"reflect"
	"strings"
)

// scanRows maps database/sql rows into the common destination shapes accepted
// by the engine query API: a struct, a slice of structs or maps, one scalar, or
// one scalar destination per selected column.
func scanRows(rows *sql.Rows, destinations ...any) error {
	if len(destinations) == 0 {
		for rows.Next() {
		}
		return rows.Err()
	}
	if len(destinations) > 1 {
		if !rows.Next() {
			if err := rows.Err(); err != nil {
				return err
			}
			return sql.ErrNoRows
		}
		return rows.Scan(destinations...)
	}

	destination := reflect.ValueOf(destinations[0])
	if destination.Kind() != reflect.Pointer || destination.IsNil() {
		return fmt.Errorf("rdbms: scan destination must be a non-nil pointer")
	}
	value := destination.Elem()
	if value.Kind() == reflect.Slice {
		return scanSlice(rows, value)
	}
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return err
		}
		return sql.ErrNoRows
	}
	if value.Kind() != reflect.Struct && value.Kind() != reflect.Map {
		return rows.Scan(destinations[0])
	}
	return scanRecord(rows, value)
}

func scanSlice(rows *sql.Rows, destination reflect.Value) error {
	elementType := destination.Type().Elem()
	pointerElements := elementType.Kind() == reflect.Pointer
	if pointerElements {
		elementType = elementType.Elem()
	}
	for rows.Next() {
		element := reflect.New(elementType).Elem()
		if err := scanRecord(rows, element); err != nil {
			return err
		}
		if pointerElements {
			pointer := reflect.New(elementType)
			pointer.Elem().Set(element)
			destination.Set(reflect.Append(destination, pointer))
		} else {
			destination.Set(reflect.Append(destination, element))
		}
	}
	return rows.Err()
}

func scanRecord(rows *sql.Rows, destination reflect.Value) error {
	columns, err := rows.Columns()
	if err != nil {
		return err
	}
	if destination.Kind() == reflect.Map {
		if destination.IsNil() {
			destination.Set(reflect.MakeMap(destination.Type()))
		}
		values := make([]any, len(columns))
		targets := make([]any, len(columns))
		for i := range values {
			targets[i] = &values[i]
		}
		if err := rows.Scan(targets...); err != nil {
			return err
		}
		for i, column := range columns {
			key := reflect.ValueOf(column).Convert(destination.Type().Key())
			value := reflect.ValueOf(values[i])
			if !value.IsValid() {
				value = reflect.Zero(destination.Type().Elem())
			} else if !value.Type().AssignableTo(destination.Type().Elem()) {
				if destination.Type().Elem().Kind() == reflect.Interface {
					value = value.Convert(destination.Type().Elem())
				} else {
					return fmt.Errorf("rdbms: cannot assign %T to map value", values[i])
				}
			}
			destination.SetMapIndex(key, value)
		}
		return nil
	}
	if destination.Kind() != reflect.Struct {
		return fmt.Errorf("rdbms: unsupported record destination %s", destination.Kind())
	}
	fields := structFields(destination)
	targets := make([]any, len(columns))
	discard := make([]any, len(columns))
	for i, column := range columns {
		if field, ok := fields[normalizeColumn(column)]; ok && field.CanAddr() && field.CanSet() {
			targets[i] = field.Addr().Interface()
		} else {
			targets[i] = &discard[i]
		}
	}
	return rows.Scan(targets...)
}

func structFields(value reflect.Value) map[string]reflect.Value {
	result := make(map[string]reflect.Value)
	var visit func(reflect.Value)
	visit = func(current reflect.Value) {
		for current.Kind() == reflect.Pointer {
			if current.IsNil() {
				current.Set(reflect.New(current.Type().Elem()))
			}
			current = current.Elem()
		}
		if current.Kind() != reflect.Struct {
			return
		}
		typeOf := current.Type()
		for i := 0; i < current.NumField(); i++ {
			fieldInfo := typeOf.Field(i)
			field := current.Field(i)
			if fieldInfo.Anonymous {
				visit(field)
			}
			name := taggedColumn(fieldInfo)
			if name != "" && name != "-" {
				result[normalizeColumn(name)] = field
			}
			result[normalizeColumn(fieldInfo.Name)] = field
		}
	}
	visit(value)
	return result
}

func taggedColumn(field reflect.StructField) string {
	for _, key := range []string{"model", "json", "db"} {
		value := strings.Split(field.Tag.Get(key), ",")[0]
		if value != "" {
			return value
		}
	}
	return ""
}

func normalizeColumn(value string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), "_", ""))
}
