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
func (e *Engine) DB() *Database {
	return &Database{engine: e}
}

// Model selects an active runtime model by ID, name, or reference name.
func (d *Database) Model(ctx context.Context, idOrName string) *ModelDatabase {
	if d == nil || d.engine == nil {
		return &ModelDatabase{err: fmt.Errorf("engine database is not initialized")}
	}
	resolved, err := d.engine.resolveActiveModelID(ctx, idOrName, false)
	return &ModelDatabase{engine: d.engine, modelID: resolved, err: err}
}

// Table selects an active runtime model by its schema-qualified or plain table name.
func (d *Database) Table(ctx context.Context, table string) *ModelDatabase {
	if d == nil || d.engine == nil {
		return &ModelDatabase{err: fmt.Errorf("engine database is not initialized")}
	}
	resolved, err := d.engine.resolveActiveModelID(ctx, table, true)
	return &ModelDatabase{engine: d.engine, modelID: resolved, err: err}
}

func (d *ModelDatabase) Err() error {
	if d == nil {
		return fmt.Errorf("engine model database is nil")
	}
	return d.err
}

// ActiveModel returns the compiled active model selected by this scope.
func (d *ModelDatabase) ActiveModel() (*model.Model, error) {
	if err := d.Err(); err != nil {
		return nil, err
	}
	return d.engine.getActiveModel(d.modelID)
}

func (d *ModelDatabase) Create(ctx context.Context, data map[string]any) (map[string]any, error) {
	if err := d.Err(); err != nil {
		return nil, err
	}
	return d.engine.Create(ctx, d.modelID, data)
}

func (d *ModelDatabase) Find(ctx context.Context, q query.Query) ([]map[string]any, int64, error) {
	if err := d.Err(); err != nil {
		return nil, 0, err
	}
	return d.engine.Find(ctx, d.modelID, q)
}

func (d *ModelDatabase) FindOne(ctx context.Context, id any) (map[string]any, error) {
	if err := d.Err(); err != nil {
		return nil, err
	}
	return d.engine.FindOne(ctx, d.modelID, id)
}

// FindOneWithQuery retrieves one row while applying the same filters, relation
// loading, projection, and debug controls used by Find.
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

func (d *ModelDatabase) Update(ctx context.Context, id any, data map[string]any) (map[string]any, error) {
	if err := d.Err(); err != nil {
		return nil, err
	}
	return d.engine.Update(ctx, d.modelID, id, data)
}

func (d *ModelDatabase) Patch(ctx context.Context, id any, data map[string]any) (map[string]any, error) {
	if err := d.Err(); err != nil {
		return nil, err
	}
	return d.engine.Patch(ctx, d.modelID, id, data)
}

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
