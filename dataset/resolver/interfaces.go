package resolver

import (
	"context"
	"github.com/SanjayDrop5528/models-go-engine/dataset/domain"
)

// ModelDefinition represents resolved metadata about a database entity / table.
type ModelDefinition struct {
	ID        string
	Schema    string
	Table     string
	RefName   string
	Driver    string
	Fields    map[string]*FieldDefinition
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
type ModelResolver interface {
	ResolveModel(ctx context.Context, schema, collection string) (*ModelDefinition, error)
}

// FieldResolver abstracts resolving a specific field on a model.
type FieldResolver interface {
	ResolveField(ctx context.Context, model *ModelDefinition, fieldName string) (*FieldDefinition, error)
}

// FunctionResolver abstracts looking up and validating functions in the dynamic DB registry.
type FunctionResolver interface {
	ResolveFunction(ctx context.Context, name string) (*domain.FunctionDefinition, error)
	RegisterFunction(ctx context.Context, fn *domain.FunctionDefinition) error
	ListFunctions(ctx context.Context, category domain.FunctionCategory) ([]*domain.FunctionDefinition, error)
}
