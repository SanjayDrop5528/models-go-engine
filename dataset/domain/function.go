package domain

import (
	"time"
)

// FunctionCategory classifies dataset functions.
type FunctionCategory string

const (
	CategoryNumeric              FunctionCategory = "Numeric"
	CategoryString               FunctionCategory = "String"
	CategoryDateTime             FunctionCategory = "Date/Time"
	CategoryArray                FunctionCategory = "Array"
	CategoryJSON                 FunctionCategory = "JSON"
	CategoryTypeConversion       FunctionCategory = "Type Conversion"
	CategoryAggregate            FunctionCategory = "Aggregate"
	CategoryConditionalAggregate FunctionCategory = "Conditional Aggregate"
	CategoryComparison           FunctionCategory = "Comparison"
	CategoryBoolean              FunctionCategory = "Boolean"
	CategoryWindowRanking        FunctionCategory = "Window/Ranking"
	CategoryFinancial            FunctionCategory = "Financial"
	CategoryGeospatial           FunctionCategory = "Geospatial"
	CategoryUtility              FunctionCategory = "Utility"
)

// FunctionDefinition represents a database-driven function registry entry.
type FunctionDefinition struct {
	ID                 string           `json:"id"`
	Name               string           `json:"name"`
	ReferenceName      string           `json:"reference_name"`
	Category           FunctionCategory `json:"category"`
	Status             string           `json:"status"` // "ACTIVE", "INACTIVE"
	InputType          string           `json:"input_type"`
	OutputType         string           `json:"output_type"`
	MinOperands        int              `json:"min_operands"`
	MaxOperands        int              `json:"max_operands"` // -1 for unlimited/variadic
	IsAggregate        bool             `json:"is_aggregate"`
	IsDeterministic    bool             `json:"is_deterministic"`
	PostgresExpression string           `json:"postgres_expression"`
	MySQLExpression    string           `json:"mysql_expression"`
	MongoDBExpression  string           `json:"mongodb_expression"`
	Description        string           `json:"description"`
	CreatedAt          time.Time        `json:"created_at"`
	UpdatedAt          time.Time        `json:"updated_at"`
}
