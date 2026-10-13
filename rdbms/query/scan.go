package query

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/google/uuid"
)

// scanRows maps database/sql rows into the common destination shapes accepted
// by the engine query API: a struct, a slice of structs or maps, one scalar, or
// one scalar destination per selected column.
func scanRows(rows *sql.Rows, destinations ...any) error {
	return scanRowsWithContext(context.Background(), rows, destinations...)
}

func scanRowsWithContext(ctx context.Context, rows *sql.Rows, destinations ...any) error {
	if rows == nil {
		return nil
	}
	defer rows.Close()
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
		return scanSlice(ctx, rows, value)
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
	return scanRecord(ctx, rows, value)
}

func scanSlice(ctx context.Context, rows *sql.Rows, destination reflect.Value) error {
	elementType := destination.Type().Elem()
	pointerElements := elementType.Kind() == reflect.Pointer
	if pointerElements {
		elementType = elementType.Elem()
	}
	for rows.Next() {
		element := reflect.New(elementType).Elem()
		if err := scanRecord(ctx, rows, element); err != nil {
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

func scanRecord(ctx context.Context, rows *sql.Rows, destination reflect.Value) error {
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
	if err := rows.Scan(targets...); err != nil {
		return err
	}

	cleanEmptyRelations(destination)

	// Call AfterScanRow hook if implemented
	if destination.CanAddr() {
		if hook, ok := destination.Addr().Interface().(interface{ AfterScanRow(context.Context) error }); ok {
			_ = hook.AfterScanRow(ctx)
		}
	} else if hook, ok := destination.Interface().(interface{ AfterScanRow(context.Context) error }); ok {
		_ = hook.AfterScanRow(ctx)
	}

	return nil
}

func structFields(value reflect.Value) map[string]reflect.Value {
	result := make(map[string]reflect.Value)
	var visit func(current reflect.Value, prefix string)
	visit = func(current reflect.Value, prefix string) {
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
				visit(field, prefix)
				continue
			}

			// Check if field is a relation struct (pointer or struct, but not time/uuid/slice)
			fType := fieldInfo.Type
			for fType.Kind() == reflect.Pointer {
				fType = fType.Elem()
			}
			isRelStruct := fType.Kind() == reflect.Struct &&
				fType != reflect.TypeOf(time.Time{}) &&
				fType != reflect.TypeOf(uuid.UUID{}) &&
				fieldInfo.Type.Kind() != reflect.Slice

			colName := taggedColumn(fieldInfo)
			names := []string{fieldInfo.Name}
			if colName != "" && colName != "-" {
				names = append(names, colName)
			}

			for _, name := range names {
				fullKey := name
				if prefix != "" {
					fullKey = prefix + "__" + name
				}
				result[normalizeColumn(fullKey)] = field
			}

			if isRelStruct {
				relPrefix := fieldInfo.Name
				if prefix != "" {
					relPrefix = prefix + "__" + fieldInfo.Name
				}
				visit(field, relPrefix)
			}
		}
	}
	visit(value, "")
	return result
}

func cleanEmptyRelations(value reflect.Value) {
	for value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return
	}
	t := value.Type()
	for i := 0; i < value.NumField(); i++ {
		f := value.Field(i)
		fInfo := t.Field(i)
		if fInfo.Anonymous {
			cleanEmptyRelations(f)
			continue
		}
		if f.Kind() == reflect.Pointer && !f.IsNil() && f.Elem().Kind() == reflect.Struct &&
			f.Type().Elem() != reflect.TypeOf(time.Time{}) &&
			f.Type().Elem() != reflect.TypeOf(uuid.UUID{}) {
			cleanEmptyRelations(f.Elem())
			if isStructAllZero(f.Elem()) {
				f.Set(reflect.Zero(f.Type()))
			}
		}
	}
}

func isStructAllZero(v reflect.Value) bool {
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		if !field.IsZero() {
			return false
		}
	}
	return true
}

func taggedColumn(field reflect.StructField) string {
	for _, key := range []string{"bun", "model", "db"} {
		tag := field.Tag.Get(key)
		if tag == "-" {
			return "-"
		}
		parts := strings.Split(tag, ",")
		if parts[0] != "" {
			return parts[0]
		}
	}
	if jsonTag := field.Tag.Get("json"); jsonTag != "" {
		parts := strings.Split(jsonTag, ",")
		if parts[0] != "" {
			return parts[0]
		}
	}
	return ""
}

func normalizeColumn(value string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), "_", ""))
}
