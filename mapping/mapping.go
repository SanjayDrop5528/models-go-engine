package mapping

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"github.com/SanjayDrop5528/models-go-engine/model"
	"strconv"
	"strings"
	"time"
)

// GenerateUUID generates a compact 32-character hex RFC 4122 version 4 UUID string without hyphens.
func GenerateUUID() string {
	var uuid [16]byte
	_, _ = rand.Read(uuid[:])
	uuid[6] = (uuid[6] & 0x0f) | 0x40 // Version 4
	uuid[8] = (uuid[8] & 0x3f) | 0x80 // Variant 10
	return fmt.Sprintf("%08x%04x%04x%04x%012x",
		uuid[0:4],
		uuid[4:6],
		uuid[6:8],
		uuid[8:10],
		uuid[10:16])
}

// CoerceValue safely converts an arbitrary input value (e.g. from JSON payload) to the target model DataType.
func CoerceValue(val any, targetType model.DataType) (any, error) {
	if val == nil {
		return nil, nil
	}

	switch targetType {
	case model.TypeString, model.TypeText:
		switch v := val.(type) {
		case string:
			return v, nil
		case fmt.Stringer:
			return v.String(), nil
		default:
			return nil, fmt.Errorf("expects string value, got %T", val)
		}

	case model.TypeInt:
		switch v := val.(type) {
		case int:
			return v, nil
		case int64:
			return int(v), nil
		case float64:
			return int(v), nil
		case string:
			i, err := strconv.Atoi(v)
			if err != nil {
				return nil, fmt.Errorf("cannot parse '%s' as int: %w", v, err)
			}
			return i, nil
		default:
			return nil, fmt.Errorf("unsupported type %T for INT", val)
		}

	case model.TypeLong:
		switch v := val.(type) {
		case int64:
			return v, nil
		case int:
			return int64(v), nil
		case float64:
			return int64(v), nil
		case string:
			i, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("cannot parse '%s' as int64: %w", v, err)
			}
			return i, nil
		default:
			return nil, fmt.Errorf("unsupported type %T for LONG", val)
		}

	case model.TypeFloat, model.TypeDecimal:
		switch v := val.(type) {
		case float64:
			return v, nil
		case float32:
			return float64(v), nil
		case int:
			return float64(v), nil
		case int64:
			return float64(v), nil
		case string:
			f, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return nil, fmt.Errorf("cannot parse '%s' as float: %w", v, err)
			}
			return f, nil
		default:
			return nil, fmt.Errorf("unsupported type %T for FLOAT/DECIMAL", val)
		}

	case model.TypeBoolean:
		switch v := val.(type) {
		case bool:
			return v, nil
		case string:
			b, err := strconv.ParseBool(v)
			if err != nil {
				return nil, fmt.Errorf("cannot parse '%s' as boolean: %w", v, err)
			}
			return b, nil
		case int:
			return v != 0, nil
		case float64:
			return v != 0, nil
		default:
			return nil, fmt.Errorf("unsupported type %T for BOOLEAN", val)
		}

	case model.TypeDateTime:
		switch v := val.(type) {
		case time.Time:
			return v.UTC(), nil
		case string:
			// Try RFC3339, RFC3339Nano, standard ISO formats
			formats := []string{
				time.RFC3339Nano,
				time.RFC3339,
				"2006-01-02T15:04:05",
				"2006-01-02 15:04:05",
				"2006-01-02",
			}
			for _, fmtStr := range formats {
				if t, err := time.Parse(fmtStr, v); err == nil {
					return t.UTC(), nil
				}
			}
			return nil, fmt.Errorf("invalid datetime format '%s'", v)
		default:
			return nil, fmt.Errorf("unsupported type %T for DATETIME", val)
		}

	case model.TypeDate:
		switch v := val.(type) {
		case time.Time:
			return v.Format("2006-01-02"), nil
		case string:
			if _, err := time.Parse("2006-01-02", v); err == nil {
				return v, nil
			}
			return nil, fmt.Errorf("invalid date format '%s', expected YYYY-MM-DD", v)
		default:
			return nil, fmt.Errorf("unsupported type %T for DATE", val)
		}

	case model.TypeJSON, model.TypeArray:
		switch v := val.(type) {
		case string:
			// If already a valid json string or needs parsing
			var parsed any
			if err := json.Unmarshal([]byte(v), &parsed); err == nil {
				return parsed, nil
			}
			return v, nil
		default:
			return v, nil
		}

	case model.TypeUUID:
		return fmt.Sprintf("%v", val), nil

	default:
		return val, nil
	}
}

func isSequenceOrAuto(attr *model.Attribute) bool {
	if attr == nil {
		return false
	}
	if attr.AutoIncrement {
		return true
	}
	if attr.Type == model.TypeInt || attr.Type == model.TypeLong {
		return true
	}
	if attr.Default != nil {
		defStr := strings.ToLower(fmt.Sprintf("%v", attr.Default))
		if strings.Contains(defStr, "nextval") || strings.Contains(defStr, "sequence") || strings.Contains(defStr, "autoincrement") || strings.Contains(defStr, "identity") {
			return true
		}
	}
	return false
}

// SanitizeInput validates and coerces incoming record payload according to Model attribute definitions.
// If an 'id' attribute (or string/UUID primary key) is not provided or empty, a UUID string is generated automatically.
func SanitizeInput(m *model.Model, data map[string]any) (map[string]any, error) {
	sanitized := make(map[string]any)

	// Ensure data map exists
	if data == nil {
		data = make(map[string]any)
	}

	// If 'id' is not provided in data, check if we should auto-generate a UUID string
	rawID, hasID := data["id"]
	if !hasID || rawID == nil || fmt.Sprintf("%v", rawID) == "" {
		idAttr := m.GetAttribute("id")
		if idAttr != nil {
			if (idAttr.Type == model.TypeString || idAttr.Type == model.TypeUUID || idAttr.Type == model.TypeText) && !isSequenceOrAuto(idAttr) {
				data["id"] = GenerateUUID()
			}
		} else if m.StorageType == model.StorageDocument {
			data["id"] = GenerateUUID()
		}
	}

	for _, attr := range m.Attributes {
		rawVal, exists := data[attr.Name]
		if !exists {
			if attr.Default != nil {
				sanitized[attr.Name] = attr.Default
			} else if (attr.Name == "id" || m.IsPrimaryKey(attr.Name)) && (attr.Type == model.TypeString || attr.Type == model.TypeUUID || attr.Type == model.TypeText) && !isSequenceOrAuto(&attr) {
				sanitized[attr.Name] = GenerateUUID()
			} else if !attr.Nullable && !attr.AutoIncrement && !isSequenceOrAuto(&attr) && !m.IsPrimaryKey(attr.Name) {
				return nil, fmt.Errorf("missing required field '%s'", attr.Name)
			}
			continue
		}

		if rawVal == nil || (attr.Name == "id" && fmt.Sprintf("%v", rawVal) == "" && (attr.Type == model.TypeString || attr.Type == model.TypeUUID || attr.Type == model.TypeText)) {
			if (attr.Name == "id" || m.IsPrimaryKey(attr.Name)) && (attr.Type == model.TypeString || attr.Type == model.TypeUUID || attr.Type == model.TypeText) && !isSequenceOrAuto(&attr) {
				sanitized[attr.Name] = GenerateUUID()
				continue
			}
			if !attr.Nullable && !attr.AutoIncrement && !isSequenceOrAuto(&attr) && !m.IsPrimaryKey(attr.Name) {
				return nil, fmt.Errorf("field '%s' cannot be null", attr.Name)
			}
			sanitized[attr.Name] = nil
			continue
		}

		coerced, err := CoerceValue(rawVal, attr.Type)
		if err != nil {
			return nil, fmt.Errorf("field '%s': %w", attr.Name, err)
		}
		sanitized[attr.Name] = coerced
	}

	return sanitized, nil
}

// SanitizePartialInput coerces and sanitizes ONLY the keys present in data payload for update operations.
func SanitizePartialInput(m *model.Model, data map[string]any) (map[string]any, error) {
	if m == nil || data == nil {
		return data, nil
	}

	sanitized := make(map[string]any)
	for k, rawVal := range data {
		attr := m.GetAttribute(k)
		if attr == nil {
			sanitized[k] = rawVal
			continue
		}

		if rawVal == nil {
			if !attr.Nullable {
				return nil, fmt.Errorf("field '%s' cannot be null", attr.Name)
			}
			sanitized[k] = nil
			continue
		}

		coerced, err := CoerceValue(rawVal, attr.Type)
		if err != nil {
			return nil, fmt.Errorf("field '%s': %w", attr.Name, err)
		}
		sanitized[k] = coerced
	}

	return sanitized, nil
}
