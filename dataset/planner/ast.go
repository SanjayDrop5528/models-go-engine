package planner

import (
	"github.com/SanjayDrop5528/models-go-engine/dataset/domain"
)

// QueryAST represents the complete, database-independent query execution plan.
type QueryAST struct {
	BaseTable     ASTBaseTable
	Joins         []ASTJoin
	Projections   []ASTProjection
	CustomColumns []ASTCustomColumn
	WhereFilters  []ASTCondition
	GroupBy       []ASTGroupBy
	HavingFilters []ASTCondition
	OrderBy       []ASTOrderBy
	Limit         int
	Offset        int
	Distinct      bool
	Parameters    []domain.FilterParam
}

// ASTBaseTable represents the root FROM source.
type ASTBaseTable struct {
	Schema string
	Table  string
	Alias  string
	Filter map[string]any
}

// ASTJoin represents a relational JOIN with its specific ON join filter.
type ASTJoin struct {
	Schema        string
	Type          domain.JoinType
	FromTable     string
	FromField     string
	ToTable       string
	ToField       string
	Alias         string
	JoinFilter    map[string]any
	ConvertString bool
	CastMode      string
}

// ASTProjection represents a selected result column.
type ASTProjection struct {
	SourceTable string
	SourceField string
	Alias       string
	DataType    string
}

// ASTCustomColumn represents a calculated or aggregate expression.
type ASTCustomColumn struct {
	Alias       string
	Label       string
	Function    *domain.FunctionDefinition
	Expression  string
	Operands    []ASTOperand
	IsAggregate bool
	DataType    string
}

// ASTOperand represents an argument passed to a function or calculation.
type ASTOperand struct {
	SourceTable string
	SourceField string
	LiteralVal  any
	IsLiteral   bool
}

// ASTGroupBy represents grouping column criteria.
type ASTGroupBy struct {
	Table string
	Field string
}

// ASTCondition represents a structured filter comparison.
type ASTCondition struct {
	Table         string
	Column        string
	Operator      string
	Value         any
	IsParamRef    bool
	ParamName     string
	ParamDataType string
	Clause        string // "AND" or "OR"
	Children      []ASTCondition
}

// ASTOrderBy represents sort direction.
type ASTOrderBy struct {
	Table string
	Field string
	Desc  bool
}
