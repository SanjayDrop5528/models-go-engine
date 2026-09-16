// Package adapter provides mock implementations of database adapters and transactions for unit testing.
//
// File: mock.go
// Usage:
//   Provides MockAdapter and MockTransaction which simulate schema alterations, CRUD queries,
//   and transactions purely in memory without requiring external database instances.
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
//
// Purpose:
//   Instantiates an in-memory test double of the Adapter interface.
//
// Where it is used:
//   - Used by unit test suites in models-go-engine packages.
//
// When can it be used:
//   - Call when testing engine workflows without spinning up live database instances.
func NewMockAdapter() *MockAdapter {
	return &MockAdapter{
		schemas: make(map[string]*schema.Schema),
		data:    make(map[string][]map[string]any),
		autoInc: make(map[string]int64),
	}
}

// Name returns the identifier of the mock adapter.
//
// Purpose:
//   Identifies the mock adapter name.
//
// Where it is used:
//   - Used during adapter registration and logging.
//
// When can it be used:
//   - Call when checking adapter identity.
func (a *MockAdapter) Name() string { return "mock" }

// DatabaseName returns the logical mock database name.
//
// Purpose:
//   Identifies the mock database name.
//
// Where it is used:
//   - Used in database routing.
//
// When can it be used:
//   - Call when inspecting the active mock database name.
func (a *MockAdapter) DatabaseName() string { return "mock" }

// NativeClient returns the underlying mock instance.
//
// Purpose:
//   Exposes the underlying test double instance.
//
// Where it is used:
//   - Used in test assertions.
//
// When can it be used:
//   - Call when inspecting mock state directly.
func (a *MockAdapter) NativeClient() any { return a }

// Connect simulates database connection initialization.
//
// Purpose:
//   Simulates establishing a database connection.
//
// Where it is used:
//   - Called during adapter bootstrap.
//
// When can it be used:
//   - Call during adapter lifecycle startup.
func (a *MockAdapter) Connect(ctx context.Context) error { return nil }

// Ping simulates checking database connectivity.
//
// Purpose:
//   Simulates health check ping.
//
// Where it is used:
//   - Called during health checks.
//
// When can it be used:
//   - Call to check connection vitality.
func (a *MockAdapter) Ping(ctx context.Context) error { return nil }

// Close simulates closing the mock connection.
//
// Purpose:
//   Simulates teardown of database resources.
//
// Where it is used:
//   - Called during test cleanup and server shutdown.
//
// When can it be used:
//   - Call during shutdown.
func (a *MockAdapter) Close(ctx context.Context) error { return nil }

// EnsureMetadataTables simulates metadata schema initialization.
//
// Purpose:
//   Satisfies metadata persistence interface for mock adapter.
//
// Where it is used:
//   - Called during project metadata initialization.
//
// When can it be used:
//   - Call to bootstrap metadata tables.
func (a *MockAdapter) EnsureMetadataTables(ctx context.Context) error {
	return nil
}

// ImportLiveMetadata simulates reverse introspection of metadata.
//
// Purpose:
//   Simulates loading model configurations from live database.
//
// Where it is used:
//   - Called during project import routines.
//
// When can it be used:
//   - Call when synchronizing models from live storage.
func (a *MockAdapter) ImportLiveMetadata(ctx context.Context) ([]*model.ModelConfig, []*model.DataModel, error) {
	return nil, nil, nil
}

// GetSchema returns the in-memory schema definition for the requested model.
//
// Purpose:
//   Retrieves stored mock schema definition for a table or collection.
//
// Where it is used:
//   - Called by SchemaService during diffing and testing.
//
// When can it be used:
//   - Call when inspecting schema in mock tests.
func (a *MockAdapter) GetSchema(ctx context.Context, m model.ModelRef) (*schema.Schema, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	s, ok := a.schemas[m.StorageName]
	if !ok {
		return nil, nil
	}
	return s, nil
}

// ValidateSchemaPlan simulates schema plan validation.
//
// Purpose:
//   Validates the given schema plan in the mock adapter.
//
// Where it is used:
//   - Called prior to executing mock schema changes.
//
// When can it be used:
//   - Call to check plan validity.
func (a *MockAdapter) ValidateSchemaPlan(ctx context.Context, p *plan.SchemaPlan) error {
	return nil
}

// PreviewSchemaChange simulates generating native preview actions for mock execution.
//
// Purpose:
//   Produces mock preview action statements for the operations in the schema plan.
//
// Where it is used:
//   - Called by SchemaService.Preview in tests.
//
// When can it be used:
//   - Call when testing migration preview output.
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

// ApplySchemaChange applies the schema modifications to the in-memory mock schema table.
//
// Purpose:
//   Mutates the in-memory Schema structure with added/removed columns and created tables.
//
// Where it is used:
//   - Called by SchemaService.Apply in tests.
//
// When can it be used:
//   - Call when executing schema migration tests.
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

// Create inserts a record into the in-memory table.
//
// Purpose:
//   Stores a record in memory and assigns an auto-incrementing ID if none provided.
//
// Where it is used:
//   - Called by CRUD service tests.
//
// When can it be used:
//   - Call when inserting dynamic records in mock mode.
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

// Find retrieves records matching filter conditions from memory.
//
// Purpose:
//   Scans in-memory records and returns those matching filter values.
//
// Where it is used:
//   - Called by CRUD service Find tests.
//
// When can it be used:
//   - Call when querying mock records.
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

// FindOne retrieves a single record by primary key identifier from memory.
//
// Purpose:
//   Finds the specific record whose id matches the provided parameter.
//
// Where it is used:
//   - Called by CRUD service FindOne tests.
//
// When can it be used:
//   - Call when fetching an individual mock record.
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

// Update replaces an existing record in memory by ID.
//
// Purpose:
//   Updates matching record fields with the provided payload.
//
// Where it is used:
//   - Called by CRUD service Update tests.
//
// When can it be used:
//   - Call when modifying existing records in mock storage.
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

// Patch updates specific fields on an existing record in memory.
//
// Purpose:
//   Merges payload keys into an existing mock record.
//
// Where it is used:
//   - Called by CRUD service Patch tests.
//
// When can it be used:
//   - Call when patching attributes in mock storage.
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

// Delete removes a record by primary key identifier from memory.
//
// Purpose:
//   Removes the matching record slice element from in-memory storage.
//
// Where it is used:
//   - Called by CRUD service Delete tests.
//
// When can it be used:
//   - Call when deleting a mock record.
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

// Execute returns a synthetic execution response for custom commands.
//
// Purpose:
//   Emulates operation execution (function, procedure, batch) in memory.
//
// Where it is used:
//   - Called by operation execution tests.
//
// When can it be used:
//   - Call when testing dynamic command execution.
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

// Begin starts a mock transaction.
//
// Purpose:
//   Returns a MockTransaction wrapper around the mock adapter.
//
// Where it is used:
//   - Called by transactional tests.
//
// When can it be used:
//   - Call when simulating transactional boundaries.
func (a *MockAdapter) Begin(ctx context.Context) (Transaction, error) {
	return &MockTransaction{adapter: a}, nil
}

// MockTransaction implements Transaction for MockAdapter.
type MockTransaction struct {
	adapter    *MockAdapter
	committed  bool
	rolledBack bool
}

// Create inserts a record in the mock transaction.
//
// Purpose:
//   Delegates Create to the underlying mock adapter.
//
// Where it is used:
//   - Called during transactional tests.
//
// When can it be used:
//   - Call within a mock transaction.
func (tx *MockTransaction) Create(ctx context.Context, m model.ModelRef, data map[string]any) (map[string]any, error) {
	return tx.adapter.Create(ctx, m, data)
}

// Find queries records in the mock transaction.
//
// Purpose:
//   Delegates Find to the underlying mock adapter.
//
// Where it is used:
//   - Called during transactional tests.
//
// When can it be used:
//   - Call within a mock transaction.
func (tx *MockTransaction) Find(ctx context.Context, m model.ModelRef, q query.Query) ([]map[string]any, int64, error) {
	return tx.adapter.Find(ctx, m, q)
}

// FindOne queries a single record in the mock transaction.
//
// Purpose:
//   Delegates FindOne to the underlying mock adapter.
//
// Where it is used:
//   - Called during transactional tests.
//
// When can it be used:
//   - Call within a mock transaction.
func (tx *MockTransaction) FindOne(ctx context.Context, m model.ModelRef, id any) (map[string]any, error) {
	return tx.adapter.FindOne(ctx, m, id)
}

// Update modifies a record in the mock transaction.
//
// Purpose:
//   Delegates Update to the underlying mock adapter.
//
// Where it is used:
//   - Called during transactional tests.
//
// When can it be used:
//   - Call within a mock transaction.
func (tx *MockTransaction) Update(ctx context.Context, m model.ModelRef, id any, data map[string]any) (map[string]any, error) {
	return tx.adapter.Update(ctx, m, id, data)
}

// Patch partially updates a record in the mock transaction.
//
// Purpose:
//   Delegates Patch to the underlying mock adapter.
//
// Where it is used:
//   - Called during transactional tests.
//
// When can it be used:
//   - Call within a mock transaction.
func (tx *MockTransaction) Patch(ctx context.Context, m model.ModelRef, id any, data map[string]any) (map[string]any, error) {
	return tx.adapter.Patch(ctx, m, id, data)
}

// Delete removes a record in the mock transaction.
//
// Purpose:
//   Delegates Delete to the underlying mock adapter.
//
// Where it is used:
//   - Called during transactional tests.
//
// When can it be used:
//   - Call within a mock transaction.
func (tx *MockTransaction) Delete(ctx context.Context, m model.ModelRef, id any) error {
	return tx.adapter.Delete(ctx, m, id)
}

// Execute dispatches an execution request in the mock transaction.
//
// Purpose:
//   Delegates Execute to the underlying mock adapter.
//
// Where it is used:
//   - Called during transactional tests.
//
// When can it be used:
//   - Call within a mock transaction.
func (tx *MockTransaction) Execute(ctx context.Context, req execution.ExecutionRequest) (*execution.ExecutionResult, error) {
	return tx.adapter.Execute(ctx, req)
}

// Commit records that the transaction committed.
//
// Purpose:
//   Marks the mock transaction as committed.
//
// Where it is used:
//   - Called at the end of successful transactional blocks.
//
// When can it be used:
//   - Call to finalize a transaction in tests.
func (tx *MockTransaction) Commit(ctx context.Context) error {
	tx.committed = true
	return nil
}

// Rollback records that the transaction rolled back.
//
// Purpose:
//   Marks the mock transaction as rolled back.
//
// Where it is used:
//   - Called upon error in transactional blocks.
//
// When can it be used:
//   - Call to abort a transaction in tests.
func (tx *MockTransaction) Rollback(ctx context.Context) error {
	tx.rolledBack = true
	return nil
}

