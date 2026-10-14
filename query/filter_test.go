package query_test

import (
	"encoding/json"
	"testing"

	"github.com/SanjayDrop5528/models-go-engine/query"
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

func TestParsePaginationRequestSupportsDatasetOperators(t *testing.T) {
	q := query.ParsePaginationRequest(query.PaginationRequest{Filter: []query.FilterCondition{{Conditions: []query.ConditionGroup{
		{Column: "id", Operator: "=", Value: 10},
		{Column: "status", Operator: "not equals", Value: "deleted"},
		{Column: "kind", Operator: "IN", Value: []any{"a", "b"}},
		{Column: "kind", Operator: "NOT_IN", Value: []any{"x", "y"}},
	}}}})
	if len(q.Filters) != 4 {
		t.Fatalf("expected four filters, got %d", len(q.Filters))
	}
	if q.Filters[0].Op != query.OpEq || q.Filters[1].Op != query.OpNeq || q.Filters[2].Op != query.OpIn || q.Filters[3].Op != query.OpNin {
		t.Fatalf("unexpected operators: %+v", q.Filters)
	}
}

func TestPaginationRequestStandardJSONUnmarshal(t *testing.T) {
	rawJSON := `{
		"start": 10,
		"end": 50,
		"first_filter_append": true,
		"filterColumns": [
			{"fieldname": "tenant_id", "fieldvalue": "org-123", "defaultvalue": "default-org"}
		],
		"filter": [
			{
				"clause": "AND",
				"conditions": [
					{"operator": "EQUALS", "column": "status", "type": "string", "value": "active"}
				]
			}
		],
		"sort": [
			{"sort": "desc", "colId": "created_at"}
		],
		"filterParams": [
			{"ParamsName": "startDate", "parmsDataType": "date"}
		]
	}`

	var req query.PaginationRequest
	if err := json.Unmarshal([]byte(rawJSON), &req); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if req.Start != 10 || req.End != 50 || !req.FirstFilterAppend {
		t.Fatalf("basic fields mismatch: %+v", req)
	}
	if len(req.FilterColumns) != 1 || req.FilterColumns[0].FieldName != "tenant_id" || req.FilterColumns[0].FieldValue != "org-123" {
		t.Fatalf("filterColumns mismatch: %+v", req.FilterColumns)
	}
	if len(req.Filter) != 1 || len(req.Filter[0].Conditions) != 1 || req.Filter[0].Conditions[0].Column != "status" {
		t.Fatalf("filter mismatch: %+v", req.Filter)
	}
	if len(req.Sort) != 1 || req.Sort[0].ColID != "created_at" || req.Sort[0].Sort != "desc" {
		t.Fatalf("sort mismatch: %+v", req.Sort)
	}
	if len(req.FilterParam) != 1 || req.FilterParam[0].ParamsName != "startDate" {
		t.Fatalf("filterParam mismatch: %+v", req.FilterParam)
	}
}
