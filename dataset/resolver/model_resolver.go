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
func NewModelResolver(reg *registry.ModelRegistry) *DynamicModelResolver {
	return &DynamicModelResolver{
		registry: reg,
		models:   make(map[string]*ModelDefinition),
	}
}

// RegisterModel registers a local model definition for schema validation.
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
		if act, err := r.registry.GetActive(collection); err == nil && act != nil {
			md := &ModelDefinition{
				ID:      act.ID,
				Schema:  schema,
				Table:   act.Table,
				RefName: act.Name,
				Fields:  make(map[string]*FieldDefinition),
			}
			for _, attr := range act.Attributes {
				col := attr.ColumnName
				if col == "" {
					col = attr.Name
				}
				md.Fields[strings.ToLower(col)] = &FieldDefinition{
					Name:         attr.Name,
					ColumnName:   col,
					DataType:     string(attr.Type),
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
