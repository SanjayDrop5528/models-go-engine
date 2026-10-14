package project

import (
	"context"
	"fmt"
	"strings"

	"github.com/SanjayDrop5528/models-go-engine/model"
	"github.com/SanjayDrop5528/models-go-engine/query"
)

// Database is the metadata-enforcing database entry point. Model and Table
// both resolve an active engine model before any CRUD operation is allowed.
type Database struct {
	engine *Engine
}

// ModelDatabase scopes CRUD to an engine model identifier or reference name.
type ModelDatabase struct {
	engine  *Engine
	modelID string
	err     error
}

// DB returns the metadata-enforcing database interface for this engine.
//
// Parameters:
//   - None.
//
// Returns:
//   - result1 (*Database): Returned value.
//
// Business Rules:
//   - Follows the invariants documented by DB and its owning package.
//
// Side Effects:
//   - Does not intentionally mutate caller-owned input; configured dependencies may still be consulted.
//
// Errors:
//   - This function does not return an error.
//
// Note:
//   - Callers should preserve the documented ownership and lifecycle of supplied values.
func (e *Engine) DB() *Database {
	return &Database{engine: e}
}

// Model selects an active runtime model by ID, name, or reference name.
//
// Parameters:
//   - ctx (context.Context): Input value used by the operation.
//   - idOrName (string): Input value used by the operation.
//
// Returns:
//   - result1 (*ModelDatabase): Returned value.
//
// Business Rules:
//   - Follows the invariants documented by Model and its owning package.
//
// Side Effects:
//   - Does not intentionally mutate caller-owned input; configured dependencies may still be consulted.
//
// Errors:
//   - This function does not return an error.
//
// Note:
//   - The context controls cancellation and request lifetime.
func (d *Database) Model(ctx context.Context, idOrName string) *ModelDatabase {
	if d == nil || d.engine == nil {
		return &ModelDatabase{err: fmt.Errorf("engine database is not initialized")}
	}
	resolved, err := d.engine.resolveActiveModelID(ctx, idOrName, false)
	return &ModelDatabase{engine: d.engine, modelID: resolved, err: err}
}

// Table selects an active runtime model by its schema-qualified or plain table name.
//
// Parameters:
//   - ctx (context.Context): Input value used by the operation.
//   - table (string): Input value used by the operation.
//
// Returns:
//   - result1 (*ModelDatabase): Returned value.
//
// Business Rules:
//   - Follows the invariants documented by Table and its owning package.
//
// Side Effects:
//   - Does not intentionally mutate caller-owned input; configured dependencies may still be consulted.
//
// Errors:
//   - This function does not return an error.
//
// Note:
//   - The context controls cancellation and request lifetime.
func (d *Database) Table(ctx context.Context, table string) *ModelDatabase {
	if d == nil || d.engine == nil {
		return &ModelDatabase{err: fmt.Errorf("engine database is not initialized")}
	}
	resolved, err := d.engine.resolveActiveModelID(ctx, table, true)
	return &ModelDatabase{engine: d.engine, modelID: resolved, err: err}
}

// Parameters:
//   - None.
//
// Returns:
//   - result1 (error): Returned value.
//
// Business Rules:
//   - Follows the invariants documented by Err and its owning package.
//
// Side Effects:
//   - Does not intentionally mutate caller-owned input; configured dependencies may still be consulted.
//
// Errors:
//   - Returns validation, configuration, context, or underlying dependency errors.
//
// Note:
//   - Callers should preserve the documented ownership and lifecycle of supplied values.
func (d *ModelDatabase) Err() error {
	if d == nil {
		return fmt.Errorf("engine model database is nil")
	}
	return d.err
}

// ActiveModel returns the compiled active model selected by this scope.
//
// Parameters:
//   - None.
//
// Returns:
//   - result1 (*model.Model): Returned value.
//   - result2 (error): Returned value.
//
// Business Rules:
//   - Follows the invariants documented by ActiveModel and its owning package.
//
// Side Effects:
//   - Does not intentionally mutate caller-owned input; configured dependencies may still be consulted.
//
// Errors:
//   - Returns validation, configuration, context, or underlying dependency errors.
//
// Note:
//   - Callers should preserve the documented ownership and lifecycle of supplied values.
func (d *ModelDatabase) ActiveModel() (*model.Model, error) {
	if err := d.Err(); err != nil {
		return nil, err
	}
	return d.engine.getActiveModel(d.modelID)
}

// Parameters:
//   - ctx (context.Context): Input value used by the operation.
//   - data (map[string]any): Input value used by the operation.
//
// Returns:
//   - result1 (map[string]any): Returned value.
//   - result2 (error): Returned value.
//
// Business Rules:
//   - Follows the invariants documented by Create and its owning package.
//
// Side Effects:
//   - May update receiver state, persistent storage, or configured external dependencies.
//
// Errors:
//   - Returns validation, configuration, context, or underlying dependency errors.
//
// Note:
//   - The context controls cancellation and request lifetime.
func (d *ModelDatabase) Create(ctx context.Context, data map[string]any) (map[string]any, error) {
	if err := d.Err(); err != nil {
		return nil, err
	}
	return d.engine.Create(ctx, d.modelID, data)
}

// Parameters:
//   - ctx (context.Context): Input value used by the operation.
//   - q (query.Query): Input value used by the operation.
//
// Returns:
//   - result1 ([]map[string]any): Returned value.
//   - result2 (int64): Returned value.
//   - result3 (error): Returned value.
//
// Business Rules:
//   - Follows the invariants documented by Find and its owning package.
//
// Side Effects:
//   - Does not intentionally mutate caller-owned input; configured dependencies may still be consulted.
//
// Errors:
//   - Returns validation, configuration, context, or underlying dependency errors.
//
// Note:
//   - The context controls cancellation and request lifetime.
func (d *ModelDatabase) Find(ctx context.Context, q query.Query) ([]map[string]any, int64, error) {
	if err := d.Err(); err != nil {
		return nil, 0, err
	}
	return d.engine.Find(ctx, d.modelID, q)
}

// Parameters:
//   - ctx (context.Context): Input value used by the operation.
//   - id (any): Input value used by the operation.
//
// Returns:
//   - result1 (map[string]any): Returned value.
//   - result2 (error): Returned value.
//
// Business Rules:
//   - Follows the invariants documented by FindOne and its owning package.
//
// Side Effects:
//   - Does not intentionally mutate caller-owned input; configured dependencies may still be consulted.
//
// Errors:
//   - Returns validation, configuration, context, or underlying dependency errors.
//
// Note:
//   - The context controls cancellation and request lifetime.
func (d *ModelDatabase) FindOne(ctx context.Context, id any) (map[string]any, error) {
	if err := d.Err(); err != nil {
		return nil, err
	}
	return d.engine.FindOne(ctx, d.modelID, id)
}

// FindOneWithQuery retrieves one row while applying the same filters, relation
// loading, projection, and debug controls used by Find.
//
// Parameters:
//   - ctx (context.Context): Input value used by the operation.
//   - id (any): Input value used by the operation.
//   - q (query.Query): Input value used by the operation.
//
// Returns:
//   - result1 (map[string]any): Returned value.
//   - result2 (error): Returned value.
//
// Business Rules:
//   - Follows the invariants documented by FindOneWithQuery and its owning package.
//
// Side Effects:
//   - Does not intentionally mutate caller-owned input; configured dependencies may still be consulted.
//
// Errors:
//   - Returns validation, configuration, context, or underlying dependency errors.
//
// Note:
//   - The context controls cancellation and request lifetime.
func (d *ModelDatabase) FindOneWithQuery(ctx context.Context, id any, q query.Query) (map[string]any, error) {
	if err := d.Err(); err != nil {
		return nil, err
	}
	m, err := d.engine.getActiveModel(d.modelID)
	if err != nil {
		return nil, err
	}
	primaryKey := m.Ref().PrimaryKey
	if primaryKey == "" {
		primaryKey = "id"
	}
	q.Filters = append(q.Filters, query.Filter{Field: primaryKey, Op: query.OpEq, Value: id})
	q.Pagination.Limit = 1
	rows, _, err := d.engine.adapter.Find(ctx, m.Ref(), q)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("record %v not found in model %q", id, d.modelID)
	}
	return rows[0], nil
}

// Parameters:
//   - ctx (context.Context): Input value used by the operation.
//   - id (any): Input value used by the operation.
//   - data (map[string]any): Input value used by the operation.
//
// Returns:
//   - result1 (map[string]any): Returned value.
//   - result2 (error): Returned value.
//
// Business Rules:
//   - Follows the invariants documented by Update and its owning package.
//
// Side Effects:
//   - May update receiver state, persistent storage, or configured external dependencies.
//
// Errors:
//   - Returns validation, configuration, context, or underlying dependency errors.
//
// Note:
//   - The context controls cancellation and request lifetime.
func (d *ModelDatabase) Update(ctx context.Context, id any, data map[string]any) (map[string]any, error) {
	if err := d.Err(); err != nil {
		return nil, err
	}
	return d.engine.Update(ctx, d.modelID, id, data)
}

// Parameters:
//   - ctx (context.Context): Input value used by the operation.
//   - id (any): Input value used by the operation.
//   - data (map[string]any): Input value used by the operation.
//
// Returns:
//   - result1 (map[string]any): Returned value.
//   - result2 (error): Returned value.
//
// Business Rules:
//   - Follows the invariants documented by Patch and its owning package.
//
// Side Effects:
//   - Does not intentionally mutate caller-owned input; configured dependencies may still be consulted.
//
// Errors:
//   - Returns validation, configuration, context, or underlying dependency errors.
//
// Note:
//   - The context controls cancellation and request lifetime.
func (d *ModelDatabase) Patch(ctx context.Context, id any, data map[string]any) (map[string]any, error) {
	if err := d.Err(); err != nil {
		return nil, err
	}
	return d.engine.Patch(ctx, d.modelID, id, data)
}

// Parameters:
//   - ctx (context.Context): Input value used by the operation.
//   - id (any): Input value used by the operation.
//
// Returns:
//   - result1 (error): Returned value.
//
// Business Rules:
//   - Follows the invariants documented by Delete and its owning package.
//
// Side Effects:
//   - May update receiver state, persistent storage, or configured external dependencies.
//
// Errors:
//   - Returns validation, configuration, context, or underlying dependency errors.
//
// Note:
//   - The context controls cancellation and request lifetime.
func (d *ModelDatabase) Delete(ctx context.Context, id any) error {
	if err := d.Err(); err != nil {
		return err
	}
	return d.engine.Delete(ctx, d.modelID, id)
}

func (e *Engine) resolveActiveModelID(_ context.Context, idOrTable string, tableOnly bool) (string, error) {
	name := strings.TrimSpace(idOrTable)
	if name == "" {
		return "", fmt.Errorf("model or table name is required")
	}
	if !tableOnly {
		if active, err := e.registry.GetActive(name); err == nil && active != nil {
			return active.ID, nil
		}
	}
	for _, cfg := range e.registry.ListModelConfigs() {
		if cfg == nil || cfg.Status != model.ModelConfigStatusActive {
			continue
		}
		qualified := cfg.Table
		if cfg.Schema != "" && !strings.Contains(cfg.Table, ".") {
			qualified = cfg.Schema + "." + cfg.Table
		}
		matchesTable := strings.EqualFold(name, cfg.Table) || strings.EqualFold(name, qualified)
		matchesModel := strings.EqualFold(name, cfg.ID) || strings.EqualFold(name, cfg.Name) || strings.EqualFold(name, cfg.RefName)
		if matchesTable || (!tableOnly && matchesModel) {
			if active, err := e.registry.GetActive(cfg.ID); err == nil && active != nil {
				return active.ID, nil
			}
		}
	}
	return "", fmt.Errorf("active engine model %q is not registered", name)
}
