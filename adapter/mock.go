package adapter

import (
	"context"
	"fmt"
	"github.com/SanjayDrop5528/models-go-engine/diff"
	"github.com/SanjayDrop5528/models-go-engine/execution"
	"github.com/SanjayDrop5528/models-go-engine/model"
	"github.com/SanjayDrop5528/models-go-engine/plan"
	"github.com/SanjayDrop5528/models-go-engine/query"
	"github.com/SanjayDrop5528/models-go-engine/schema"
	"sync"
)

// MockAdapter is an in-memory mock implementation of Adapter for tests within engine.
type MockAdapter struct {
	mu      sync.RWMutex
	schemas map[string]*schema.Schema
	data    map[string][]map[string]any
	autoInc map[string]int64
}

// NewMockAdapter creates a new in-memory MockAdapter.
func NewMockAdapter() *MockAdapter {
	return &MockAdapter{
		schemas: make(map[string]*schema.Schema),
		data:    make(map[string][]map[string]any),
		autoInc: make(map[string]int64),
	}
}

func (a *MockAdapter) Name() string                     { return "mock" }
func (a *MockAdapter) DatabaseName() string             { return "mock" }
func (a *MockAdapter) NativeClient() any                { return a }
func (a *MockAdapter) Connect(ctx context.Context) error { return nil }
func (a *MockAdapter) Ping(ctx context.Context) error    { return nil }
func (a *MockAdapter) Close(ctx context.Context) error   { return nil }

func (a *MockAdapter) EnsureMetadataTables(ctx context.Context) error {
	return nil
}

func (a *MockAdapter) ImportLiveMetadata(ctx context.Context) ([]*model.ModelConfig, []*model.DataModel, error) {
	return nil, nil, nil
}

func (a *MockAdapter) GetSchema(ctx context.Context, m model.ModelRef) (*schema.Schema, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	s, ok := a.schemas[m.StorageName]
	if !ok {
		return nil, nil
	}
	return s, nil
}

func (a *MockAdapter) ValidateSchemaPlan(ctx context.Context, p *plan.SchemaPlan) error {
	return nil
}

func (a *MockAdapter) PreviewSchemaChange(ctx context.Context, p *plan.SchemaPlan) (*plan.SchemaPreview, error) {
	var actions []plan.NativeAction
	for _, op := range p.Operations {
		actions = append(actions, plan.NativeAction{
			Type:        "MOCK",
			Statement:   fmt.Sprintf("MOCK: %s on %s", op.Type, op.ObjectName),
			Destructive: op.Destructive,
			Description: op.Description,
		})
	}
	return &plan.SchemaPreview{
		ModelID:        p.ModelID,
		StorageName:    p.StorageName,
		Database:       p.Database,
		Changes:        p.Operations,
		NativeActions:  actions,
		HasDestructive: p.Destructive,
	}, nil
}

func (a *MockAdapter) ApplySchemaChange(ctx context.Context, p *plan.SchemaPlan) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	s, ok := a.schemas[p.StorageName]
	if !ok {
		s = &schema.Schema{
			Name:        p.StorageName,
			StorageType: model.StorageRelational,
			Attributes:  []schema.SchemaAttribute{},
		}
		a.schemas[p.StorageName] = s
	}

	for _, op := range p.Operations {
		switch op.Type {
		case diff.OpCreateTable:
			if sch, ok := op.After.(*schema.Schema); ok {
				a.schemas[p.StorageName] = sch
				s = sch
			}
		case diff.OpAddColumn:
			if attr, ok := op.After.(schema.SchemaAttribute); ok {
				s.Attributes = append(s.Attributes, attr)
			}
		case diff.OpRemoveColumn:
			newAttrs := make([]schema.SchemaAttribute, 0)
			for _, attr := range s.Attributes {
				if attr.Name != op.ObjectName {
					newAttrs = append(newAttrs, attr)
				}
			}
			s.Attributes = newAttrs
		}
	}
	return nil
}

func (a *MockAdapter) Create(ctx context.Context, m model.ModelRef, data map[string]any) (map[string]any, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	rec := make(map[string]any)
	for k, v := range data {
		rec[k] = v
	}

	a.autoInc[m.StorageName]++
	if rec["id"] == nil {
		rec["id"] = a.autoInc[m.StorageName]
	}

	a.data[m.StorageName] = append(a.data[m.StorageName], rec)
	return rec, nil
}

func (a *MockAdapter) Find(ctx context.Context, m model.ModelRef, q query.Query) ([]map[string]any, int64, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	records := a.data[m.StorageName]
	var res []map[string]any
	for _, r := range records {
		match := true
		for _, flt := range q.Filters {
			val, exists := r[flt.Field]
			if !exists || fmt.Sprintf("%v", val) != fmt.Sprintf("%v", flt.Value) {
				match = false
				break
			}
		}
		if match {
			cp := make(map[string]any)
			for k, v := range r {
				cp[k] = v
			}
			res = append(res, cp)
		}
	}
	return res, int64(len(res)), nil
}

func (a *MockAdapter) FindOne(ctx context.Context, m model.ModelRef, id any) (map[string]any, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	records := a.data[m.StorageName]
	for _, r := range records {
		if fmt.Sprintf("%v", r["id"]) == fmt.Sprintf("%v", id) {
			cp := make(map[string]any)
			for k, v := range r {
				cp[k] = v
			}
			return cp, nil
		}
	}
	return nil, fmt.Errorf("record not found")
}

func (a *MockAdapter) Update(ctx context.Context, m model.ModelRef, id any, data map[string]any) (map[string]any, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	records := a.data[m.StorageName]
	for i, r := range records {
		if fmt.Sprintf("%v", r["id"]) == fmt.Sprintf("%v", id) {
			data["id"] = r["id"]
			records[i] = data
			return data, nil
		}
	}
	return nil, fmt.Errorf("record not found")
}

func (a *MockAdapter) Patch(ctx context.Context, m model.ModelRef, id any, data map[string]any) (map[string]any, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	records := a.data[m.StorageName]
	for _, r := range records {
		if fmt.Sprintf("%v", r["id"]) == fmt.Sprintf("%v", id) {
			for k, v := range data {
				r[k] = v
			}
			return r, nil
		}
	}
	return nil, fmt.Errorf("record not found")
}

func (a *MockAdapter) Delete(ctx context.Context, m model.ModelRef, id any) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	records := a.data[m.StorageName]
	for i, r := range records {
		if fmt.Sprintf("%v", r["id"]) == fmt.Sprintf("%v", id) {
			a.data[m.StorageName] = append(records[:i], records[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("record not found")
}

func (a *MockAdapter) Execute(ctx context.Context, req execution.ExecutionRequest) (*execution.ExecutionResult, error) {
	return &execution.ExecutionResult{
		Data: map[string]any{
			"executed_operation": req.Operation,
			"target":             req.Target,
			"arguments":          req.Arguments,
			"result":             10000,
		},
		RowsAffected: 1,
		Status:       "SUCCESS",
	}, nil
}

func (a *MockAdapter) Begin(ctx context.Context) (Transaction, error) {
	return &MockTransaction{adapter: a}, nil
}

// MockTransaction implements Transaction for MockAdapter.
type MockTransaction struct {
	adapter   *MockAdapter
	committed bool
	rolledBack bool
}

func (tx *MockTransaction) Create(ctx context.Context, m model.ModelRef, data map[string]any) (map[string]any, error) {
	return tx.adapter.Create(ctx, m, data)
}

func (tx *MockTransaction) Find(ctx context.Context, m model.ModelRef, q query.Query) ([]map[string]any, int64, error) {
	return tx.adapter.Find(ctx, m, q)
}

func (tx *MockTransaction) FindOne(ctx context.Context, m model.ModelRef, id any) (map[string]any, error) {
	return tx.adapter.FindOne(ctx, m, id)
}

func (tx *MockTransaction) Update(ctx context.Context, m model.ModelRef, id any, data map[string]any) (map[string]any, error) {
	return tx.adapter.Update(ctx, m, id, data)
}

func (tx *MockTransaction) Patch(ctx context.Context, m model.ModelRef, id any, data map[string]any) (map[string]any, error) {
	return tx.adapter.Patch(ctx, m, id, data)
}

func (tx *MockTransaction) Delete(ctx context.Context, m model.ModelRef, id any) error {
	return tx.adapter.Delete(ctx, m, id)
}

func (tx *MockTransaction) Execute(ctx context.Context, req execution.ExecutionRequest) (*execution.ExecutionResult, error) {
	return tx.adapter.Execute(ctx, req)
}

func (tx *MockTransaction) Commit(ctx context.Context) error {
	tx.committed = true
	return nil
}

func (tx *MockTransaction) Rollback(ctx context.Context) error {
	tx.rolledBack = true
	return nil
}

