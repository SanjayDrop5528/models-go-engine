package validation

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/SanjayDrop5528/models-go-engine/adapter"
	"github.com/SanjayDrop5528/models-go-engine/model"
	"github.com/SanjayDrop5528/models-go-engine/plan"
	"github.com/SanjayDrop5528/models-go-engine/query"
)

// ModelResolver abstracts looking up models by name or identifier.
type ModelResolver interface {
	GetModel(ctx context.Context, nameOrID string) (*model.Model, error)
}

// Engine performs comprehensive, pure validation across models, records, queries, and migrations
// without performing side-effects or persistent database mutations.
type Engine struct {
	resolver ModelResolver
	adapter  adapter.Adapter
}

// NewValidationEngine creates a new dedicated validation engine.
func NewValidationEngine() *Engine {
	return &Engine{}
}

// WithModelResolver configures model resolution for reference validation.
func (e *Engine) WithModelResolver(r ModelResolver) *Engine {
	e.resolver = r
	return e
}

// WithAdapter configures database adapter for live constraint/reference lookups.
func (e *Engine) WithAdapter(a adapter.Adapter) *Engine {
	e.adapter = a
	return e
}

// ValidateModel validates a model's schema definitions and constraints.
func (e *Engine) ValidateModel(m *model.Model) error {
	return ValidateModel(m)
}

// ValidateData validates a record creation payload against model attributes.
func (e *Engine) ValidateData(m *model.Model, data map[string]any) error {
	return ValidateData(m, data)
}

// ValidateUpdateData validates an update/patch payload against model attributes.
func (e *Engine) ValidateUpdateData(m *model.Model, data map[string]any) error {
	return ValidatePartialData(m, data)
}

// ValidateSchemaPlan checks a schema change plan for safety and completeness.
func (e *Engine) ValidateSchemaPlan(p *plan.SchemaPlan, allowDestructive bool) error {
	return ValidatePlan(p, allowDestructive)
}

// ValidateQuery validates query filters and sorts against the model schema.
func (e *Engine) ValidateQuery(m *model.Model, q query.Query) error {
	if m == nil {
		return errors.New("cannot validate query against nil model")
	}

	attrMap := make(map[string]bool)
	for _, attr := range m.Attributes {
		attrMap[attr.Name] = true
	}

	var errs []*ValidationError

	// Validate fields
	for _, f := range q.Fields {
		if !attrMap[f] && f != "*" && f != "count(*)" && f != "id" {
			errs = append(errs, NewValidationError(f, fmt.Sprintf("queried field '%s' does not exist in model '%s'", f, m.Name)))
		}
	}

	// Validate filters
	for _, f := range q.Filters {
		if !attrMap[f.Field] && f.Field != "id" && f.Field != "created_at" && f.Field != "updated_at" && f.Field != "deleted_at" {
			errs = append(errs, NewValidationError(f.Field, fmt.Sprintf("filter field '%s' does not exist in model '%s'", f.Field, m.Name)))
		}
	}

	// Validate sorts
	for _, s := range q.Sorts {
		if !attrMap[s.Field] && s.Field != "id" && s.Field != "created_at" && s.Field != "updated_at" {
			errs = append(errs, NewValidationError(s.Field, fmt.Sprintf("sort field '%s' does not exist in model '%s'", s.Field, m.Name)))
		}
	}

	return NewMultiValidationError(errs)
}

// ValidateOrbitalReferences checks foreign keys and orbital references against target models.
func (e *Engine) ValidateOrbitalReferences(ctx context.Context, m *model.Model, data map[string]any) error {
	if m == nil || e.adapter == nil {
		return nil
	}

	var orbitalErrs []*ValidationError

	for _, attr := range m.Attributes {
		if attr.Reference == nil || attr.Reference.Model == "" {
			continue
		}

		fieldKey := attr.Name
		val, exists := data[fieldKey]
		if !exists || val == nil {
			continue
		}

		targetModelID := attr.Reference.Model
		targetCol := "id"
		if attr.Reference.Attribute != "" {
			targetCol = attr.Reference.Attribute
		}

		targetRef := model.ModelRef{
			Name:        targetModelID,
			StorageName: targetModelID,
		}

		q := query.NewQuery().Where(targetCol, query.OpEq, val).LimitOffset(1, 0)
		results, _, err := e.adapter.Find(ctx, targetRef, q)
		if err != nil || len(results) == 0 {
			orbitalErrs = append(orbitalErrs, NewReferenceNotFoundError(fieldKey, targetModelID, val))
		} else {
			record := results[0]
			if statusVal, ok := record["status"]; ok && statusVal != nil {
				if !strings.EqualFold(fmt.Sprintf("%v", statusVal), "active") {
					orbitalErrs = append(orbitalErrs, NewValidationError(fieldKey, fmt.Sprintf("orbital reference validation failed: referenced record in '%s' is not active (status='%v')", targetModelID, statusVal)))
				}
			}
		}
	}

	return NewMultiValidationError(orbitalErrs)
}
