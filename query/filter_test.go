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

func TestParsePaginationRequestIncludesRelations(t *testing.T) {
	q := query.ParsePaginationRequest(query.PaginationRequest{
		Relations:            []string{"Customer", "OrderProducts"},
		RelationSpecs:        []query.RelationSpec{{Name: "Lines", Fields: []string{"id", "quantity"}, Order: []query.Sort{{Field: "position", Order: query.SortAsc}}}},
		Debug:                true,
		DebugIncludeArgs:     true,
		SlowQueryThresholdMS: 250,
	})

	if len(q.Relations) != 2 || q.Relations[0] != "Customer" || q.Relations[1] != "OrderProducts" {
		t.Fatalf("expected requested relations to be preserved, got %v", q.Relations)
	}
	if !q.Debug {
		t.Fatal("expected debug flag to be preserved")
	}
	if !q.DebugIncludeArgs || q.SlowQueryThresholdMS != 250 {
		t.Fatalf("expected debug controls to be preserved, got include_args=%v slow_ms=%d", q.DebugIncludeArgs, q.SlowQueryThresholdMS)
	}
	if len(q.RelationSpecs) != 1 || q.RelationSpecs[0].Name != "Lines" || len(q.RelationSpecs[0].Fields) != 2 || len(q.RelationSpecs[0].Order) != 1 {
		t.Fatalf("expected structured relation options to be preserved, got %+v", q.RelationSpecs)
	}
}
