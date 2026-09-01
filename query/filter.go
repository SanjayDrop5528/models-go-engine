package query

import (
	"fmt"
	"strings"
)

// ConditionGroup defines a single column condition or nested condition group.
type ConditionGroup struct {
	Operator             string           `json:"operator" bson:"operator"`
	Column               string           `json:"column" bson:"column"`
	ParentCollectionName string           `json:"parentCollectionName,omitempty" bson:"parentCollectionName,omitempty"`
	ValueType            any              `json:"value_type,omitempty" bson:"value_type,omitempty"`
	Type                 string           `json:"type,omitempty" bson:"type,omitempty"`
	Value                any              `json:"value,omitempty" bson:"value,omitempty"`
	Clause               string           `json:"clause,omitempty" bson:"clause,omitempty"`
	Conditions           []ConditionGroup `json:"conditions,omitempty" bson:"conditions,omitempty"`
}

// FilterCondition defines a top-level group of filter conditions with a logical clause (AND/OR).
type FilterCondition struct {
	Clause     string           `json:"clause,omitempty" bson:"clause,omitempty"`
	Conditions []ConditionGroup `json:"conditions,omitempty" bson:"conditions,omitempty"`
}

// FilterParam defines custom parameter values for dynamic pipelines.
type FilterParam struct {
	ParamsName     string `json:"ParamsName"`
	ParamsDataType string `json:"parmsDataType"`
	DefaultValue   any    `json:"DefaultValue,omitempty"`
	ParamsValue    any    `json:"Paramsvalue,omitempty"`
}

// SortParam defines sorting criteria per column.
type SortParam struct {
	ColID string `json:"colId" bson:"colId"`
	Sort  string `json:"sort" bson:"sort"`
}

// PaginationRequest defines the structured POST filter payload compatible with AG-Grid and enterprise filter APIs.
type PaginationRequest struct {
	Start        int               `json:"start,omitempty"`
	End          int               `json:"end,omitempty"`
	Filter       []FilterCondition `json:"filter,omitempty"`
	FilterParam  []FilterParam     `json:"filterParam,omitempty"`
	Sort         []SortParam       `json:"sort,omitempty"`
	IncludeTotal *bool             `json:"includeTotal,omitempty"`
	Fields       []string          `json:"fields,omitempty"`
}

// WantsTotal returns true unless includeTotal is explicitly set to false.
func (r PaginationRequest) WantsTotal() bool {
	return r.IncludeTotal == nil || *r.IncludeTotal
}

// ParsePaginationRequest converts a PaginationRequest struct into a core Query object.
func ParsePaginationRequest(req PaginationRequest) Query {
	q := NewQuery()

	// 1. Fields / Projection
	if len(req.Fields) > 0 {
		q.Fields = req.Fields
	}

	// 2. Filters
	for _, fc := range req.Filter {
		clause := strings.ToUpper(fc.Clause)
		if clause == "OR" {
			q.LogicalOp = OpOr
		}
		for _, cond := range fc.Conditions {
			parseConditionGroup(&q, cond)
		}
	}

	// 3. Sorting
	for _, s := range req.Sort {
		if s.ColID == "" {
			continue
		}
		order := SortAsc
		if strings.EqualFold(s.Sort, "desc") {
			order = SortDesc
		}
		q.Sorts = append(q.Sorts, Sort{
			Field: s.ColID,
			Order: order,
		})
	}

	// 4. Pagination
	limit := 50
	offset := 0

	if req.Start >= 0 {
		offset = req.Start
	}
	if req.End > req.Start {
		limit = req.End - req.Start
	} else if req.End > 0 {
		limit = req.End
	}

	q.Pagination = Pagination{
		Limit:  limit,
		Offset: offset,
	}
	q.CountTotal = req.WantsTotal()

	return q
}

func parseConditionGroup(q *Query, cond ConditionGroup) {
	// Handle nested conditions recursively
	if len(cond.Conditions) > 0 {
		for _, subCond := range cond.Conditions {
			parseConditionGroup(q, subCond)
		}
	}

	if cond.Column == "" {
		return
	}

	col := cond.Column
	opStr := strings.ToUpper(cond.Operator)
	val := cond.Value

	switch opStr {
	case "EQUALS":
		if val == nil {
			q.Filters = append(q.Filters, Filter{Field: col, Op: OpIsNull})
		} else {
			q.Filters = append(q.Filters, Filter{Field: col, Op: OpEq, Value: val})
		}
	case "NOTEQUAL":
		if val == nil {
			q.Filters = append(q.Filters, Filter{Field: col, Op: OpIsNotNull})
		} else {
			q.Filters = append(q.Filters, Filter{Field: col, Op: OpNeq, Value: val})
		}
	case "CONTAINS":
		q.Filters = append(q.Filters, Filter{Field: col, Op: OpILike, Value: fmt.Sprintf("%%%v%%", val)})
	case "NOTCONTAINS":
		q.Filters = append(q.Filters, Filter{Field: col, Op: OpNotLike, Value: fmt.Sprintf("%%%v%%", val)})
	case "STARTSWITH":
		q.Filters = append(q.Filters, Filter{Field: col, Op: OpILike, Value: fmt.Sprintf("%v%%", val)})
	case "ENDSWITH":
		q.Filters = append(q.Filters, Filter{Field: col, Op: OpILike, Value: fmt.Sprintf("%%%v", val)})
	case "LESSTHAN":
		q.Filters = append(q.Filters, Filter{Field: col, Op: OpLt, Value: val})
	case "GREATERTHAN":
		q.Filters = append(q.Filters, Filter{Field: col, Op: OpGt, Value: val})
	case "LESSTHANOREQUAL":
		q.Filters = append(q.Filters, Filter{Field: col, Op: OpLte, Value: val})
	case "GREATERTHANOREQUAL":
		q.Filters = append(q.Filters, Filter{Field: col, Op: OpGte, Value: val})
	case "INRANGE", "IN_BETWEEN":
		if slice, ok := val.([]any); ok && len(slice) >= 2 {
			q.Filters = append(q.Filters, Filter{Field: col, Op: OpBetween, Value: slice[0], ValueTo: slice[1]})
		}
	case "BLANK":
		q.Filters = append(q.Filters, Filter{Field: col, Op: OpIsNull})
	case "NOTBLANK":
		q.Filters = append(q.Filters, Filter{Field: col, Op: OpIsNotNull})
	case "IN":
		q.Filters = append(q.Filters, Filter{Field: col, Op: OpIn, Value: val})
	case "NOTIN":
		q.Filters = append(q.Filters, Filter{Field: col, Op: OpNin, Value: val})
	default:
		if cond.Operator != "" {
			q.Filters = append(q.Filters, Filter{Field: col, Op: OpEq, Value: val})
		}
	}
}
