// Package validation provides domain validation engines, model schema constraint checks,
// record validation, orbital reference resolution, and schema migration plan safety verification.
//
// File: engine.go
// Usage:
//   This file defines the standalone Validation Engine (validation.Engine).
//   It can be used in two primary modes:
//     1. Pure Standalone Mode (Zero DB dependencies): Validates Go structs, maps, schema models,
//        data types, required fields, formats (email, uuid, regex), and schema change plans in-memory.
//     2. Combined Mode (With Database Adapter & Model Resolver): Additionally performs live referential
//        integrity lookups and soft-active record status checks on orbital references.
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

// NewValidationEngine creates a new dedicated validation engine instance.
//
// Purpose:
//   Initializes the validation engine in standalone mode without any database or resolver requirements.
//
// Where it is used:
//   - Used directly in microservices or modules wanting only validation logic.
//   - Attached to engine projects (`project.Engine.Validation()`).
//   - Used in API validation endpoints (`/api/validation/*`).
//
// When can it be used:
//   - Can be instantiated anytime for in-memory schema, data, query, or migration validation.
func NewValidationEngine() *Engine {
	return &Engine{}
}

// WithModelResolver configures model resolution for reference validation.
//
// Purpose:
//   Attaches a model resolver so foreign keys and orbital reference targets can be dynamically looked up.
//
// Where it is used:
//   - Called during engine initialization or when configuring cross-model validation.
//
// When can it be used:
//   - When relational validation across multiple models is required.
func (e *Engine) WithModelResolver(r ModelResolver) *Engine {
	e.resolver = r
	return e
}

// WithAdapter configures database adapter for live constraint/reference lookups.
//
// Purpose:
//   Attaches a live database adapter to allow the validation engine to verify that referenced
//   foreign keys exist and meet active status constraints in the database.
//
// Where it is used:
//   - Combined architecture where the validation engine performs live referential checks before writes.
//
// When can it be used:
//   - When live database validation of orbital references is needed.
func (e *Engine) WithAdapter(a adapter.Adapter) *Engine {
	e.adapter = a
	return e
}

// ValidateModel validates a model's schema definitions and constraints.
//
// Purpose:
//   Ensures a model's name, primary key, attribute names, data types, and constraints are valid.
//
// Where it is used:
//   - Called before registering models into runtime catalogs or executing DDL migrations.
//   - Called by the HTTP endpoint `POST /api/validation/model`.
//
// When can it be used:
//   - During schema design, model creation, or dynamic ModelConfig initialization.
func (e *Engine) ValidateModel(m *model.Model) error {
	return ValidateModel(m)
}

// ValidateData validates a record creation payload against model attributes.
//
// Purpose:
//   Performs full record validation for CREATE operations: verifies required fields, checks types,
//   validates regex/email/uuid formats, and ensures no unknown fields exist if strict mode is on.
//
// Where it is used:
//   - Called before database inserts in CRUD pipelines.
//   - Called by the HTTP endpoint `POST /api/validation/data/:model`.
//
// When can it be used:
//   - Whenever a client submits a new record payload to be inserted.
func (e *Engine) ValidateData(m *model.Model, data map[string]any) error {
	return ValidateData(m, data)
}

// ValidateUpdateData validates an update/patch payload against model attributes.
//
// Purpose:
//   Performs partial record validation for UPDATE/PATCH operations: checks data types and formats
//   only for the fields included in the update payload without requiring all mandatory creation fields.
//
// Where it is used:
//   - Called before database updates or patches in CRUD pipelines.
//   - Called by the HTTP endpoint `POST /api/validation/partial-data/:model`.
//
// When can it be used:
//   - Whenever a client submits a partial record payload (PATCH).
func (e *Engine) ValidateUpdateData(m *model.Model, data map[string]any) error {
	return ValidatePartialData(m, data)
}

// ValidateSchemaPlan checks a schema change plan for safety and completeness.
//
// Purpose:
//   Analyzes schema migration operations (add column, drop table, alter type) to detect destructive
//   changes (dropping columns, data truncating type changes) and blocks them unless explicitly permitted.
//
// Where it is used:
//   - Called during DDL migration planning and before applying schema diffs to databases.
//   - Called by the HTTP endpoint `POST /api/validation/schema-plan-safety`.
//
// When can it be used:
//   - In CI/CD pipelines, runtime schema migrations, or admin panels executing database changes.
func (e *Engine) ValidateSchemaPlan(p *plan.SchemaPlan, allowDestructive bool) error {
	return ValidatePlan(p, allowDestructive)
}

// ValidateQuery validates query filters and sorts against the model schema.
//
// Purpose:
//   Inspects query fields, filter conditions, and sort clauses to ensure they correspond to real,
//   valid attributes defined in the target model schema.
//
// Where it is used:
//   - Called by query execution engines before compiling SQL or MongoDB queries.
//   - Guards against SQL injection or referencing nonexistent columns.
//
// When can it be used:
//   - Whenever dynamic queries are constructed from client requests.
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
//
// Purpose:
//   Verifies referential integrity by performing live database lookups via the adapter
//   to confirm that target records exist and satisfy active status rules (e.g. status='active').
//
// Where it is used:
//   - Called during record insertion or update when orbital reference validation is enabled.
//   - Called by the HTTP endpoint `POST /api/validation/orbital-reference/:model`.
//
// When can it be used:
//   - When database adapter is connected and referential integrity needs enforcement.
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
