package crud

import (
	"context"
	"errors"
	"fmt"
	"github.com/SanjayDrop5528/models-go-engine/adapter"
	"github.com/SanjayDrop5528/models-go-engine/mapping"
	"github.com/SanjayDrop5528/models-go-engine/model"
	"github.com/SanjayDrop5528/models-go-engine/query"
	"github.com/SanjayDrop5528/models-go-engine/validation"
)

// OrbitalValidator provides orbital reference validation capabilities across models.
type OrbitalValidator interface {
	ValidateOrbitalReferences(ctx context.Context, modelID string, data map[string]any) error
}

// Engine orchestrates dynamic data operations through adapters with strict model validation.
type Engine struct {
	adapters         *adapter.Registry
	orbitalValidator OrbitalValidator
}

// NewEngine creates a new CRUD engine.
func NewEngine(adapters *adapter.Registry) *Engine {
	return &Engine{
		adapters: adapters,
	}
}

// SetOrbitalValidator sets the orbital reference validator instance.
func (e *Engine) SetOrbitalValidator(v OrbitalValidator) {
	e.orbitalValidator = v
}

// Create validates, coerces, and inserts a new dynamic record.
func (e *Engine) Create(ctx context.Context, m *model.Model, data map[string]any) (map[string]any, error) {
	if m == nil {
		return nil, errors.New("model cannot be nil")
	}

	// 1. Validate raw payload FIRST before coercion
	if err := validation.ValidateData(m, data); err != nil {
		return nil, err
	}

	if e.orbitalValidator != nil {
		if err := e.orbitalValidator.ValidateOrbitalReferences(ctx, m.Name, data); err != nil {
			return nil, err
		}
	}

	// 2. Sanitize and coerce input for adapter
	sanitized, err := mapping.SanitizeInput(m, data)
	if err != nil {
		return nil, fmt.Errorf("data sanitization failed: %w", err)
	}

	adp, err := e.adapters.Get(m.Database)
	if err != nil {
		return nil, err
	}

	return adp.Create(ctx, m.Ref(), sanitized)
}

// Find executes a query against the adapter.
func (e *Engine) Find(ctx context.Context, m *model.Model, q query.Query) ([]map[string]any, int64, error) {
	if m == nil {
		return nil, 0, errors.New("model cannot be nil")
	}

	adp, err := e.adapters.Get(m.Database)
	if err != nil {
		return nil, 0, err
	}

	return adp.Find(ctx, m.Ref(), q)
}

// FindOne gets a record by primary key identifier.
func (e *Engine) FindOne(ctx context.Context, m *model.Model, id any) (map[string]any, error) {
	if m == nil {
		return nil, errors.New("model cannot be nil")
	}

	adp, err := e.adapters.Get(m.Database)
	if err != nil {
		return nil, err
	}

	return adp.FindOne(ctx, m.Ref(), id)
}

// Update updates fields in an existing record by payload keys.
func (e *Engine) Update(ctx context.Context, m *model.Model, id any, data map[string]any) (map[string]any, error) {
	if m == nil {
		return nil, errors.New("model cannot be nil")
	}

	adp, err := e.adapters.Get(m.Database)
	if err != nil {
		return nil, err
	}

	// 1. Verify that the target record exists in the database
	_, err = adp.FindOne(ctx, m.Ref(), id)
	if err != nil {
		return nil, fmt.Errorf("record '%v' not found in model '%s'", id, m.Name)
	}

	// 2. Validate raw payload FIRST before coercion
	if err := validation.ValidatePartialData(m, data); err != nil {
		return nil, err
	}

	if e.orbitalValidator != nil {
		if err := e.orbitalValidator.ValidateOrbitalReferences(ctx, m.Name, data); err != nil {
			return nil, err
		}
	}

	// 3. Sanitize and coerce input for adapter
	sanitized, err := mapping.SanitizePartialInput(m, data)
	if err != nil {
		return nil, fmt.Errorf("data sanitization failed: %w", err)
	}

	// 4. Update adapter with payload given
	return adp.Update(ctx, m.Ref(), id, sanitized)
}

// Patch partially updates fields in an existing record by payload keys.
func (e *Engine) Patch(ctx context.Context, m *model.Model, id any, data map[string]any) (map[string]any, error) {
	return e.Update(ctx, m, id, data)
}

// Delete removes a record by primary key identifier.
func (e *Engine) Delete(ctx context.Context, m *model.Model, id any) error {
	if m == nil {
		return errors.New("model cannot be nil")
	}

	adp, err := e.adapters.Get(m.Database)
	if err != nil {
		return err
	}

	return adp.Delete(ctx, m.Ref(), id)
}
