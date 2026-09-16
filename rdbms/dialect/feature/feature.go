// Package feature defines bitmask flags for capabilities supported across SQL dialects.
//
// File: feature.go
// Usage:
//   Declares SQL feature bitmasks (Returning, CTE, DistinctOn, IndexHints, TableLocking)
//   used by dialect implementations to declare SQL syntax capabilities.
package feature

// Feature represents a database capability bitmask or identifier.
type Feature uint32

const (
	SelectExists Feature = 1 << iota
	Returning
	CTE
	RecursiveCTE
	DistinctOn
	IndexHints
	TableLocking
)
