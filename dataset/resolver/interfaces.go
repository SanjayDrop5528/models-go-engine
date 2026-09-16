// Package resolver defines the contracts and implementations for dynamic model,
// field, and database function lookup across all Dataset Studio operations.
//
// File: interfaces.go
// Usage:
//   This file defines the core resolver interfaces (ModelResolver, FieldResolver,
//   and FunctionResolver) and resolved entity models (ModelDefinition, FieldDefinition).
//   Resolvers provide an abstraction layer between Dataset Studio components and the
//   underlying data dictionaries, ensuring that tables, columns, and expressions exist
//   and are valid regardless of database vendor.
package resolver

import (
	"context"

	"github.com/SanjayDrop5528/models-go-engine/dataset/domain"
)

// ModelDefinition represents resolved metadata about a database entity / table.
type ModelDefinition struct {
	ID      string
	Schema  string
	Table   string
	RefName string
	Driver  string
	Fields  map[string]*FieldDefinition
}

// FieldDefinition represents resolved metadata about a single column / attribute.
type FieldDefinition struct {
	Name         string
	ColumnName   string
	DataType     string
	IsPrimaryKey bool
	IsNullable   bool
	IsUnique     bool
}

// ModelResolver abstracts loading model metadata.
//
// Purpose:
//   Resolves entity definitions, schema associations, and primary keys for collections/tables.
//
// Where it is used:
//   - Used by DataSetValidator to ensure collections exist before planning queries.
//   - Implemented by DynamicModelResolver.
//
// When can it be used:
//   - Can be used during dataset design and validation to verify base and join collections.
type ModelResolver interface {
	// ResolveModel looks up model metadata given an optional schema and collection/table name.
	ResolveModel(ctx context.Context, schema, collection string) (*ModelDefinition, error)
}

// FieldResolver abstracts resolving a specific field on a model.
//
// Purpose:
//   Verifies column existence, data type compatibility, and nullability on a specific model.
//
// Where it is used:
//   - Used by DataSetValidator to validate join keys and selected fields.
//   - Implemented by DynamicModelResolver.
//
// When can it be used:
//   - Can be used when verifying whether an attribute or column belongs to a table.
type FieldResolver interface {
	// ResolveField checks if a column/attribute exists on the given model and returns its specification.
	ResolveField(ctx context.Context, model *ModelDefinition, fieldName string) (*FieldDefinition, error)
}

// FunctionResolver abstracts looking up and validating functions in the dynamic DB registry.
//
// Purpose:
//   Provides a centralized catalog of allowed functions, operands, return types, and SQL/Mongo expressions.
//
// Where it is used:
//   - Used by DataSetValidator to validate custom column function names and operand bounds.
//   - Used by DataSetPlanner to determine if custom columns require aggregation semantics.
//   - Implemented by InMemFunctionRegistry.
//
// When can it be used:
//   - Can be used when listing available functions to the UI or resolving a function's dialect expression.
type FunctionResolver interface {
	// ResolveFunction looks up a function definition by name or reference name.
	ResolveFunction(ctx context.Context, name string) (*domain.FunctionDefinition, error)

	// RegisterFunction adds or updates a function definition in the registry.
	RegisterFunction(ctx context.Context, fn *domain.FunctionDefinition) error

	// ListFunctions lists all active functions filtered by category.
	ListFunctions(ctx context.Context, category domain.FunctionCategory) ([]*domain.FunctionDefinition, error)
}
