// Package query provides fluent SQL query builders, connection handling, statement
// compilation, and lifecycle hook definitions for the RDBMS abstraction layer.
//
// File: hooks.go
// Usage:
//   Defines BeforeSelectHook and AfterSelectHook interfaces allowing models or plugins
//   to intercept query execution before dispatch and after row scanning.
package query

import "context"

// BeforeSelectHook is executed before a SELECT query is run.
type BeforeSelectHook interface {
	BeforeSelect(ctx context.Context, query any) error
}

// AfterSelectHook is executed after SELECT query results are scanned.
type AfterSelectHook interface {
	AfterSelect(ctx context.Context, query any) error
}
