package tool

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
)

// schemaFor derives JSON Schema from a Go type.
// @param t input Go type.
// @return JSON Schema or unsupported-type error.
func schemaFor(t reflect.Type) (map[string]any, error) {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.Struct:
		properties := make(map[string]any)
		required := make([]string, 0)
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			if !field.IsExported() {
				continue
			}
			parts := strings.Split(field.Tag.Get("json"), ",")
			if parts[0] == "-" {
				continue
			}
			name := parts[0]
			if name == "" {
				name = field.Name
			}
			child, err := schemaFor(field.Type)
			if err != nil {
				return nil, fmt.Errorf("field %s: %w", field.Name, err)
			}
			if description := field.Tag.Get("description"); description != "" {
				child["description"] = description
			}
			properties[name] = child
			if !contains(parts[1:], "omitempty") {
				required = append(required, name)
			}
		}
		schema := map[string]any{"type": "object", "properties": properties, "additionalProperties": false}
		if len(required) > 0 {
			schema["required"] = required
		}
		return schema, nil
	case reflect.String:
		return map[string]any{"type": "string"}, nil
	case reflect.Bool:
		return map[string]any{"type": "boolean"}, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return map[string]any{"type": "integer"}, nil
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}, nil
	case reflect.Slice, reflect.Array:
		items, err := schemaFor(t.Elem())
		if err != nil {
			return nil, err
		}
		return map[string]any{"type": "array", "items": items}, nil
	case reflect.Map:
		if t.Key().Kind() != reflect.String {
			return nil, errors.New("map keys must be strings")
		}
		value, err := schemaFor(t.Elem())
		if err != nil {
			return nil, err
		}
		return map[string]any{"type": "object", "additionalProperties": value}, nil
	case reflect.Interface:
		return map[string]any{}, nil
	default:
		return nil, fmt.Errorf("unsupported Go type %s", t)
	}
}

// contains reports whether a string slice contains a value.
// @param values strings to inspect.
// @param want target value.
// @return true when found.
func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
