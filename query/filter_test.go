package query_test

import (
	"github.com/SanjayDrop5528/models-go-engine/query"
	"testing"
)

func TestParsePaginationRequest(t *testing.T) {
	req := query.PaginationRequest{
		Start: 0,
		End:   20,
		Filter: []query.FilterCondition{
			{
				Clause: "AND",
				Conditions: []query.ConditionGroup{
					{
						Column:   "status",
						Operator: "EQUALS",
						Value:    "active",
					},
					{
						Column:   "created_at",
						Operator: "GREATERTHAN",
						Value:    "2026-01-01",
					},
					{
						Column:   "name",
						Operator: "CONTAINS",
						Value:    "john",
					},
				},
			},
		},
		Sort: []query.SortParam{
			{ColID: "created_at", Sort: "desc"},
		},
	}

	q := query.ParsePaginationRequest(req)

	if q.Pagination.Limit != 20 {
		t.Fatalf("expected limit 20, got: %d", q.Pagination.Limit)
	}
	if q.Pagination.Offset != 0 {
		t.Fatalf("expected offset 0, got: %d", q.Pagination.Offset)
	}
	if len(q.Filters) != 3 {
		t.Fatalf("expected 3 filters, got: %d", len(q.Filters))
	}
	if len(q.Sorts) != 1 || q.Sorts[0].Order != query.SortDesc {
		t.Fatalf("expected 1 DESC sort, got: %+v", q.Sorts)
	}
}
