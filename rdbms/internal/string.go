package internal

import (
	"unsafe"
)

// IsUpper reports whether the byte is an ASCII uppercase letter.
func IsUpper(c byte) bool {
	return c >= 'A' && c <= 'Z'
}

// IsLower reports whether the byte is an ASCII lowercase letter.
func IsLower(c byte) bool {
	return c >= 'a' && c <= 'z'
}

// ToUpper converts an ASCII byte to uppercase.
func ToUpper(c byte) byte {
	if IsLower(c) {
		return c - 32
	}
	return c
}

// ToLower converts an ASCII byte to lowercase.
func ToLower(c byte) byte {
	if IsUpper(c) {
		return c + 32
	}
	return c
}

// Underscore converts "CamelCasedString" to "camel_cased_string".
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
func String(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return unsafe.String(unsafe.SliceData(b), len(b))
}

// Bytes converts string to byte slice with zero allocation.
func Bytes(s string) []byte {
	if len(s) == 0 {
		return nil
	}
	return unsafe.Slice(unsafe.StringData(s), len(s))
}
