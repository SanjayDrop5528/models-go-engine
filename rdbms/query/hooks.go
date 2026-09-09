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
