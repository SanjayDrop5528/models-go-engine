// Package internal provides low-level string manipulation, case conversion,
// and zero-allocation byte slice conversions used by the RDBMS query engine.
//
// File: string.go
// Usage:
//   Internal string utility functions for casing conversions (Underscore, CamelCased,
//   ToExported) and zero-copy string/byte conversions.
package internal

import (
	"unsafe"
)

// IsUpper reports whether the byte is an ASCII uppercase letter.
//
// Purpose:
//   Checks if the given byte falls in ASCII range 'A' through 'Z'.
//
// Where it is used:
//   - In string case manipulation routines (ToLower, Underscore).
//
// When can it be used:
//   - When verifying ASCII character case.
func IsUpper(c byte) bool {
	return c >= 'A' && c <= 'Z'
}

// IsLower reports whether the byte is an ASCII lowercase letter.
//
// Purpose:
//   Checks if the given byte falls in ASCII range 'a' through 'z'.
//
// Where it is used:
//   - In string case manipulation routines (ToUpper, CamelCased, ToExported).
//
// When can it be used:
//   - When verifying ASCII character case.
func IsLower(c byte) bool {
	return c >= 'a' && c <= 'z'
}

// ToUpper converts an ASCII byte to uppercase.
//
// Purpose:
//   Converts ASCII lowercase byte to uppercase equivalent.
//
// Where it is used:
//   - In CamelCased and ToExported string converters.
//
// When can it be used:
//   - When capitalizing ASCII characters.
func ToUpper(c byte) byte {
	if IsLower(c) {
		return c - 32
	}
	return c
}

// ToLower converts an ASCII byte to lowercase.
//
// Purpose:
//   Converts ASCII uppercase byte to lowercase equivalent.
//
// Where it is used:
//   - In Underscore case converter.
//
// When can it be used:
//   - When lowercasing ASCII characters.
func ToLower(c byte) byte {
	if IsUpper(c) {
		return c + 32
	}
	return c
}

// Underscore converts "CamelCasedString" to "camel_cased_string".
//
// Purpose:
//   Converts camelCase or PascalCase identifiers to snake_case column and table names.
//
// Where it is used:
//   - In query builders and schema mapping when deriving SQL names from Go identifiers.
//
// When can it be used:
//   - When normalizing model struct field names to SQL database identifiers.
func Underscore(s string) string {
	r := make([]byte, 0, len(s)+5)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if IsUpper(c) {
			if i > 0 && i+1 < len(s) && (IsLower(s[i-1]) || IsLower(s[i+1])) {
				r = append(r, '_', ToLower(c))
			} else {
				r = append(r, ToLower(c))
			}
		} else {
			r = append(r, c)
		}
	}
	return string(r)
}

// CamelCased converts "snake_cased_string" to "SnakeCasedString".
//
// Purpose:
//   Converts snake_case database column names into PascalCase Go field identifiers.
//
// Where it is used:
//   - In introspection and model schema synthesis.
//
// When can it be used:
//   - When generating Go field names from SQL column names.
func CamelCased(s string) string {
	r := make([]byte, 0, len(s))
	upperNext := true
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '_' {
			upperNext = true
			continue
		}
		if upperNext {
			if IsLower(c) {
				c = ToUpper(c)
			}
			upperNext = false
		}
		r = append(r, c)
	}
	return string(r)
}

// ToExported capitalizes the first letter of a string if it is lowercase.
//
// Purpose:
//   Ensures the first character of an identifier is uppercase to produce an exported Go symbol.
//
// Where it is used:
//   - In code generators and reflection helpers.
//
// When can it be used:
//   - When exporting identifiers programmatically.
func ToExported(s string) string {
	if len(s) == 0 {
		return s
	}
	if c := s[0]; IsLower(c) {
		b := []byte(s)
		b[0] = ToUpper(c)
		return string(b)
	}
	return s
}

// String converts byte slice to string with zero allocation.
//
// Purpose:
//   Performs zero-copy conversion of byte slice to string using unsafe memory casting.
//
// Where it is used:
//   - In SQL query buffer serialization.
//
// When can it be used:
//   - In high-performance hot paths where buffer allocations should be avoided.
func String(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return unsafe.String(unsafe.SliceData(b), len(b))
}

// Bytes converts string to byte slice with zero allocation.
//
// Purpose:
//   Performs zero-copy conversion of string to byte slice using unsafe memory casting.
//
// Where it is used:
//   - In SQL query writing and byte buffer appending routines.
//
// When can it be used:
//   - In read-only operations consuming string buffers as byte slices.
func Bytes(s string) []byte {
	if len(s) == 0 {
		return nil
	}
	return unsafe.Slice(unsafe.StringData(s), len(s))
}
