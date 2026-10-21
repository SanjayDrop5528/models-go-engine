// Package resolver defines the contracts and implementations for dynamic model,
// field, and database function lookup across all Dataset Studio operations.
//
// File: model_resolver.go
// Usage:
//
//	This file implements the DynamicModelResolver, which bridges the engine's core
//	model registry (ModelRegistry) with Dataset Studio's validation and planning pipelines.
//	It allows models and table definitions to be dynamically registered or looked up,
//	exposing primary keys, data types, nullability, and unique constraints.
package resolver

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/SanjayDrop5528/models-go-engine/dataset/domain"
	"github.com/SanjayDrop5528/models-go-engine/registry"
)

// DynamicModelResolver resolves model metadata from the engine's model registry or static dictionary.
type DynamicModelResolver struct {
	mu       sync.RWMutex
	registry *registry.ModelRegistry
	models   map[string]*ModelDefinition
}

// NewModelResolver creates a new model and field resolver.
//
// Purpose:
//
//	Initializes a thread-safe DynamicModelResolver backed optionally by a central ModelRegistry.
//
// Where it is used:
//   - Instantiated in server setups, dataset service initialization, and integration tests.
//
// When can it be used:
//   - Can be used whenever dataset operations require table/model metadata validation.
//
// Parameters:
//   - reg (*registry.ModelRegistry): Input value used by the operation.
//
// Returns:
//   - result1 (*DynamicModelResolver): Returned value.
//
// Business Rules:
//   - Follows the invariants documented by NewModelResolver and its owning package.
//
// Side Effects:
//   - Does not intentionally mutate caller-owned input; configured dependencies may still be consulted.
//
// Errors:
//   - This function does not return an error.
//
// Note:
//   - Callers should preserve the documented ownership and lifecycle of supplied values.
func NewModelResolver(reg *registry.ModelRegistry) *DynamicModelResolver {
	return &DynamicModelResolver{
		registry: reg,
		models:   make(map[string]*ModelDefinition),
	}
}

// RegisterModel registers a local model definition for schema validation.
//
// Purpose:
//
//	Stores a table/model specification in memory keyed by table name, schema.table, and reference name.
//
// Where it is used:
//   - Called during service bootstrapping, mock setup, or when schemas are discovered dynamically.
//
// When can it be used:
//   - Can be used to inject test fixtures or pre-register runtime tables into the resolver.
//
// Parameters:
//   - m (*ModelDefinition): Input value used by the operation.
//
// Returns:
//   - None.
//
// Business Rules:
//   - Follows the invariants documented by RegisterModel and its owning package.
//
// Side Effects:
//   - May update receiver state, persistent storage, or configured external dependencies.
//
// Errors:
//   - This function does not return an error.
//
// Note:
//   - Callers should preserve the documented ownership and lifecycle of supplied values.
func (r *DynamicModelResolver) RegisterModel(m *ModelDefinition) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := strings.ToLower(m.Table)
	if m.Schema != "" {
		key = strings.ToLower(fmt.Sprintf("%s.%s", m.Schema, m.Table))
	}
	r.models[key] = m
	r.models[strings.ToLower(m.Table)] = m
	if m.RefName != "" {
		r.models[strings.ToLower(m.RefName)] = m
	}
}

// ResolveModel looks up model metadata from registry.
//
// Purpose:
//
//	Finds a table or model definition by schema and collection name. If not present in local cache,
//	it consults the underlying ModelRegistry; if still absent, it creates a permissive virtual model.
//
// Where it is used:
//   - Called by DataSetValidator to check root and join collections.
//
// When can it be used:
//   - Can be used whenever inspecting entity attributes or verifying schema existence.
//
// Parameters:
//   - ctx (context.Context): Input value used by the operation.
//   - schema (string): Input value used by the operation.
//   - collection (string): Input value used by the operation.
//
// Returns:
//   - result1 (*ModelDefinition): Returned value.
//   - result2 (error): Returned value.
//
// Business Rules:
//   - Follows the invariants documented by ResolveModel and its owning package.
//
// Side Effects:
//   - Does not intentionally mutate caller-owned input; configured dependencies may still be consulted.
//
// Errors:
//   - Returns validation, configuration, context, or underlying dependency errors.
//
// Note:
//   - The context controls cancellation and request lifetime.
func (r *DynamicModelResolver) ResolveModel(ctx context.Context, schema, collection string) (*ModelDefinition, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := strings.ToLower(collection)
	if schema != "" {
		key = strings.ToLower(fmt.Sprintf("%s.%s", schema, collection))
	}

	if m, exists := r.models[key]; exists {
		return m, nil
	}

	// Fallback to core registry if present
	if r.registry != nil {
		lookup := collection
		if schema != "" {
			lookup = schema + "." + collection
		}
		act, err := r.registry.GetActive(lookup)
		if err != nil {
			act, err = r.registry.GetActive(collection)
		}
		if err == nil && act != nil && (schema == "" || act.Schema == "" || strings.EqualFold(schema, act.Schema)) {
			md := &ModelDefinition{
				ID:      act.ID,
				Schema:  schema,
				Table:   act.Table,
				RefName: act.Name,
				Fields:  make(map[string]*FieldDefinition),
			}
			if md.Schema == "" {
				md.Schema = act.Schema
			}
			for _, attr := range act.Attributes {
				dataType := string(attr.Type)
				if attr.Type == "ARRAY" && attr.Validation != nil && attr.Validation.Items != nil && attr.Validation.Items.Type != "" {
					dataType = string(attr.Validation.Items.Type) + "[]"
				}
				col := attr.ColumnName
				if col == "" {
					col = attr.Name
				}
				md.Fields[strings.ToLower(col)] = &FieldDefinition{
					Name:         attr.Name,
					ColumnName:   col,
					DataType:     dataType,
					IsPrimaryKey: attr.IsPrimaryKey,
					IsNullable:   attr.Nullable,
					IsUnique:     attr.Unique,
				}
				md.Fields[strings.ToLower(attr.Name)] = md.Fields[strings.ToLower(col)]
			}
			return md, nil
		}
	}

	// Create permissive virtual model definition if not registered
	return &ModelDefinition{
		Schema: schema,
		Table:  collection,
		Fields: make(map[string]*FieldDefinition),
	}, nil
}

// ResolveField verifies if a field exists on the model.
//
// Purpose:
//
//	Inspects the model's attribute dictionary for a column name. If found, returns the FieldDefinition;
//	otherwise, returns a permissive virtual FieldDefinition with ANY data type.
//
// Where it is used:
//   - Called by DataSetValidator when verifying fields referenced in joins, custom columns, and projections.
//
// When can it be used:
//   - When verifying field types, nullability, or checking whether an attribute is a primary key.
//
// Parameters:
//   - ctx (context.Context): Input value used by the operation.
//   - model (*ModelDefinition): Input value used by the operation.
//   - fieldName (string): Input value used by the operation.
//
// Returns:
//   - result1 (*FieldDefinition): Returned value.
//   - result2 (error): Returned value.
//
// Business Rules:
//   - Follows the invariants documented by ResolveField and its owning package.
//
// Side Effects:
//   - Does not intentionally mutate caller-owned input; configured dependencies may still be consulted.
//
// Errors:
//   - Returns validation, configuration, context, or underlying dependency errors.
//
// Note:
//   - The context controls cancellation and request lifetime.
func (r *DynamicModelResolver) ResolveField(ctx context.Context, model *ModelDefinition, fieldName string) (*FieldDefinition, error) {
	if model == nil {
		return nil, domain.NewError(domain.ErrModelNotFound, "cannot resolve field on nil model")
	}
	norm := strings.ToLower(strings.TrimSpace(fieldName))
	if f, exists := model.Fields[norm]; exists {
		return f, nil
	}
	// Return virtual field definition
	return &FieldDefinition{
		Name:       fieldName,
		ColumnName: fieldName,
		DataType:   "ANY",
		IsNullable: true,
	}, nil
}
