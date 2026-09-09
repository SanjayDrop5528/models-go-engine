package ai

// AIQueryPlan represents the structured query plan produced by the LLM.
type AIQueryPlan struct {
	BaseModel    string        `json:"base_model"`              // Root table/collection name
	Schema       string        `json:"schema,omitempty"`        // Schema name (e.g. spares, trip, public)
	Select       []AISelect    `json:"select,omitempty"`        // Projected columns
	Joins        []AIJoin      `json:"joins,omitempty"`         // Multi-table relations
	Filters      []AIFilter    `json:"filters,omitempty"`       // WHERE filters
	Aggregations []AIAggregate `json:"aggregations,omitempty"`  // Aggregations (SUM, AVG, COUNT, MIN, MAX)
	GroupBy      []AIGroupBy   `json:"group_by,omitempty"`      // Categorical dimensions
	FilterParams []AIParam     `json:"filter_params,omitempty"` // Dynamic runtime arguments
	Driver       string        `json:"driver,omitempty"`        // postgres, mysql, mongodb, memory
	SaveMode     string        `json:"save_mode,omitempty"`     // PROCEDURE, FUNCTION, QUERY
	Explanation  string        `json:"explanation,omitempty"`   // Conversational explanation for user
}

// AISelect defines an individual projected column.
type AISelect struct {
	Table      string `json:"table,omitempty"`
	Field      string `json:"field"`
	HeaderName string `json:"header_name,omitempty"`
	DataType   string `json:"data_type,omitempty"`
}

// AIJoin defines a relational join link between tables.
type AIJoin struct {
	Schema    string `json:"schema,omitempty"`
	FromTable string `json:"from_table"`
	FromField string `json:"from_field"`
	ToTable   string `json:"to_table"`
	ToField   string `json:"to_field"`
	NamedAs   string `json:"named_as,omitempty"`
	JoinType  string `json:"join_type,omitempty"` // LEFT, INNER, RIGHT, FULL
}

// AIFilter defines a filter condition on a field.
type AIFilter struct {
	Table    string `json:"table,omitempty"`
	Field    string `json:"field"`
	Operator string `json:"operator"` // =, !=, >, >=, <, <=, like, in, between, is_null
	Value    any    `json:"value"`
}

// AIAggregate defines an aggregate metric calculation.
type AIAggregate struct {
	Name       string `json:"name"`                  // Output alias name (e.g. total_capacity)
	Function   string `json:"function"`              // SUM, AVG, COUNT, MIN, MAX
	Table      string `json:"table"`                 // Source table
	Field      string `json:"field"`                 // Target field
	GroupByKey string `json:"group_by_key,omitempty"` // Associated group by key
}

// AIGroupBy defines a categorical group by dimension.
type AIGroupBy struct {
	Table string `json:"table"`
	Field string `json:"field"`
	Name  string `json:"name,omitempty"`
}

// AIParam defines a parameterized query input argument.
type AIParam struct {
	Name     string `json:"name"`
	DataType string `json:"data_type"`
}
