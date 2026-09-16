// Package domain defines the core dataset models, schemas, and parameter parsing logic.
//
// File: filter_params.go
// Usage:
//   This file provides centralized parameter parsing, placeholder replacement, and dynamic
//   token/date expression resolution for datasets across all database adapters (PostgreSQL,
//   MySQL, MongoDB, and in-memory). It ensures that parameter values supplied in runtime payloads
//   take precedence over default values, and that dynamic user tokens (KTON|key) and date macros
//   (CD|+offset|mode) are uniformly resolved across all query execution and compilation modes.
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
//
// Purpose:
//   Normalizes diverse input representations (Go structs, JSON objects, maps) into a
//   standard string-keyed map for safe lookup and parameter extraction.
//
// Where it is used:
//   - Called by CreateFilterParams and ConvertValueToDataType to inspect userToken objects.
//   - Called by DataSetService and API handlers when processing authentication/session tokens.
//
// When can it be used:
//   - Can be used whenever an arbitrary object needs to be accessed dynamically by string key,
//     such as extracting session variables ("org_id", "timezone", "user_id").
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
//
// Purpose:
//   Replaces parameter definition placeholders (`{"paramName":"...","paramDataType":"..."}`) within
//   raw pipeline or SQL query templates with actual concrete values. If a runtime value (Paramvalue)
//   is provided, it is used; otherwise, it falls back to the defined DefaultValue.
//
// Where it is used:
//   - Used by DataSetService.Execute / ExecuteWithUserToken for direct query execution (SaveModeQuery).
//   - Used by MongoDB dataset compiler and adapter execution when compiling native aggregation pipelines.
//   - Used by preview handlers to substitute test parameter values into reference queries.
//
// When can it be used:
//   - When executing a dataset query directly without database stored routines.
//   - When compiling a parameterized reference pipeline into an executable pipeline.
//   - Whenever dynamic runtime values, user token claims (KTON), or current date macros (CD) need
//     to be stamped into a query string.
func CreateFilterParams(filterParams []FilterParam, pipeline string, userToken any) string {
	filterPipeline := pipeline
	var user map[string]any
	if userToken != nil {
		user, _ = StructToMap(userToken)
	}

	for _, filter := range filterParams {
		findString := fmt.Sprintf(`{"paramName":"%s","paramDataType":"%s"}`, filter.ParamName, filter.ParamDataType)

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

			filterPipeline = strings.ReplaceAll(filterPipeline, findString, replaceVal)
		}
	}

	return filterPipeline
}

// ConvertValueToDataType converts a value according to its data type, resolving
// dynamic user token fields (KTON|<key>) and current date expressions (CD|<offset>|<mode>).
//
// Purpose:
//   Formats a given Go value or macro expression into a string suitable for query injection,
//   performing dynamic token extraction and timezone-aware date calculations where necessary.
//
// Where it is used:
//   - Called by CreateFilterParams for each matched parameter.
//   - Called by DataSetService.ExecuteWithUserToken to resolve dynamic expressions before type coercion.
//
// When can it be used:
//   - When resolving string macros:
//       "KTON|org_id"    -> Resolves to userToken["org_id"]
//       "CD"             -> Resolves to current timestamp in user timezone
//       "CD|+1"          -> Resolves to tomorrow (+1 day)
//       "CD|-1"          -> Resolves to yesterday (-1 day)
//       "CD|+0|ST"       -> Resolves to Start of Day (00:00:00) in user timezone
//       "CD|+0|ED"       -> Resolves to End of Day (23:59:59) in user timezone
//   - When converting numeric, boolean, or temporal primitive types into string representations.
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
