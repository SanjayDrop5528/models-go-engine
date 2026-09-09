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
