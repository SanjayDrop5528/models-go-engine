package domain

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// StructToMap converts any struct, map, or pointer into a map[string]any.
func StructToMap(obj any) (map[string]any, error) {
	if obj == nil {
		return nil, nil
	}
	if m, ok := obj.(map[string]any); ok {
		return m, nil
	}
	if m, ok := obj.(map[string]string); ok {
		res := make(map[string]any, len(m))
		for k, v := range m {
			res[k] = v
		}
		return res, nil
	}
	b, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	var res map[string]any
	err = json.Unmarshal(b, &res)
	return res, err
}

// CreateFilterParams resolves and replaces filter parameter placeholders in a pipeline/query string.
// It is common for all adapters (PostgreSQL, MySQL, MongoDB, Memory).
// Placeholders supported:
//   - `{"paramName":"...","paramDataType":"..."}`
//   - `{"ParamsName":"...","parmsDataType":"..."}`
//   - Escaped variations: `{\"paramName\":\"...\",\"paramDataType\":\"...\"}`
func CreateFilterParams(filterParams []FilterParam, pipeline string, userToken any) string {
	filterPipeline := pipeline
	var user map[string]any
	if userToken != nil {
		user, _ = StructToMap(userToken)
	}

	for _, filter := range filterParams {
		patterns := []string{
			fmt.Sprintf(`{"paramName":"%s","paramDataType":"%s"}`, filter.ParamName, filter.ParamDataType),
			fmt.Sprintf(`{"paramName":"%s", "paramDataType":"%s"}`, filter.ParamName, filter.ParamDataType),
			fmt.Sprintf(`{"ParamsName":"%s","parmsDataType":"%s"}`, filter.ParamName, filter.ParamDataType),
			fmt.Sprintf(`{"ParamsName":"%s", "parmsDataType":"%s"}`, filter.ParamName, filter.ParamDataType),
			fmt.Sprintf(`{\"paramName\":\"%s\",\"paramDataType\":\"%s\"}`, filter.ParamName, filter.ParamDataType),
			fmt.Sprintf(`{\"ParamsName\":\"%s\",\"parmsDataType\":\"%s\"}`, filter.ParamName, filter.ParamDataType),
		}

		targetVal := filter.Paramvalue
		if targetVal == nil || targetVal == "" {
			targetVal = filter.DefaultValue
		}

		if targetVal != nil {
			c := reflect.TypeOf(targetVal).String()
			a := ConvertValueToDataType(c, targetVal, user)

			lowerType := strings.ToLower(filter.ParamDataType)
			if lowerType == "string" || lowerType == "time.time" || lowerType == "date" || lowerType == "datetime" || lowerType == "timestamp" {
				if !strings.HasPrefix(a, `"`) && !strings.HasSuffix(a, `"`) {
					a = `"` + a + `"`
				}
			}

			replaceVal := a
			if a == `"unsupported_data_type"` || a == "unsupported_data_type" {
				queryString, err := json.Marshal(targetVal)
				if err == nil {
					replaceVal = string(queryString)
				}
			}

			for _, pattern := range patterns {
				filterPipeline = strings.ReplaceAll(filterPipeline, pattern, replaceVal)
			}
		}
	}

	return filterPipeline
}

// ConvertValueToDataType converts a value according to its data type, resolving
// dynamic user token fields (KTON|<key>) and current date expressions (CD|<offset>|<mode>).
func ConvertValueToDataType(datatype string, defaultValue any, user any) string {
	var replaceValue string

	switch datatype {
	case "int":
		if intValue, ok := defaultValue.(int); ok {
			replaceValue = strconv.Itoa(intValue)
		}
	case "int8":
		if intValue, ok := defaultValue.(int8); ok {
			replaceValue = strconv.FormatInt(int64(intValue), 10)
		}
	case "int16":
		if intValue, ok := defaultValue.(int16); ok {
			replaceValue = strconv.FormatInt(int64(intValue), 10)
		}
	case "int32":
		if intValue, ok := defaultValue.(int32); ok {
			replaceValue = strconv.FormatInt(int64(intValue), 10)
		}
	case "int64":
		if intValue, ok := defaultValue.(int64); ok {
			replaceValue = strconv.FormatInt(intValue, 10)
		}
	case "uint":
		if uintValue, ok := defaultValue.(uint); ok {
			replaceValue = strconv.FormatUint(uint64(uintValue), 10)
		}
	case "uint8":
		if uintValue, ok := defaultValue.(uint8); ok {
			replaceValue = strconv.FormatUint(uint64(uintValue), 10)
		}
	case "uint16":
		if uintValue, ok := defaultValue.(uint16); ok {
			replaceValue = strconv.FormatUint(uint64(uintValue), 10)
		}
	case "uint32":
		if uintValue, ok := defaultValue.(uint32); ok {
			replaceValue = strconv.FormatUint(uint64(uintValue), 10)
		}
	case "uint64":
		if uintValue, ok := defaultValue.(uint64); ok {
			replaceValue = strconv.FormatUint(uintValue, 10)
		}
	case "bool":
		if boolValue, ok := defaultValue.(bool); ok {
			replaceValue = strconv.FormatBool(boolValue)
		}
	case "string":
		if stringValue, ok := defaultValue.(string); ok {
			replaceValue = stringValue
		}
		if user != nil {
			if valStr, ok := defaultValue.(string); ok {
				parts := strings.Split(valStr, "|")

				// Expecting format: KTON|key
				if len(parts) == 2 && parts[0] == "KTON" {
					key := parts[1]
					if tokenMap, ok := user.(map[string]any); ok {
						if val, exists := tokenMap[key]; exists {
							replaceValue = fmt.Sprintf("%v", val)
						}
					}
				} else if len(parts) >= 1 && parts[0] == "CD" {
					// Load timezone (fallback to UTC if invalid)
					var tz string
					if tokenMap, ok := user.(map[string]any); ok {
						if tVal, ok := tokenMap["timezone"].(string); ok {
							tz = tVal
						}
					}
					loc, err := time.LoadLocation(tz)
					if err != nil || loc == nil {
						loc = time.UTC
					}

					now := time.Now().In(loc)
					offset := 0
					mode := ""

					// Parse offset (e.g., +2, -1)
					if len(parts) >= 2 {
						offsetStr := parts[1]
						if strings.HasPrefix(offsetStr, "+") || strings.HasPrefix(offsetStr, "-") {
							if val, err := strconv.Atoi(offsetStr); err == nil {
								offset = val
							}
						}
					}

					// Parse mode (ST / ED)
					if len(parts) == 3 {
						mode = parts[2]
					}

					// Apply day offset
					result := now.AddDate(0, 0, offset)

					// Apply Start / End of Day in user's timezone
					switch mode {
					case "ST":
						replaceValue = time.Date(result.Year(), result.Month(), result.Day(), 0, 0, 0, 0, loc).Format(time.RFC3339)
					case "ED":
						replaceValue = time.Date(result.Year(), result.Month(), result.Day(), 23, 59, 59, 0, loc).Format(time.RFC3339)
					default:
						replaceValue = result.Format(time.RFC3339)
					}
				}
			}
		}
	case "float32":
		if floatValue, ok := defaultValue.(float32); ok {
			replaceValue = strconv.FormatFloat(float64(floatValue), 'f', -1, 32)
		}
	case "float64":
		if floatValue, ok := defaultValue.(float64); ok {
			replaceValue = strconv.FormatFloat(floatValue, 'f', -1, 64)
		}
	case "json.Number":
		if num, ok := defaultValue.(json.Number); ok {
			replaceValue = num.String()
		}
	default:
		// Also try type assertion for common primitives if reflect type was an alias
		if intVal, ok := defaultValue.(int); ok {
			replaceValue = strconv.Itoa(intVal)
		} else if floatVal, ok := defaultValue.(float64); ok {
			replaceValue = strconv.FormatFloat(floatVal, 'f', -1, 64)
		} else if boolVal, ok := defaultValue.(bool); ok {
			replaceValue = strconv.FormatBool(boolVal)
		} else if strVal, ok := defaultValue.(string); ok {
			replaceValue = strVal
		} else {
			replaceValue = "unsupported_data_type"
		}
	}

	return replaceValue
}
