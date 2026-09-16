// Package adapter defines the database-agnostic interface abstractions for database
// connections, transactions, schema operations, metadata import, and CRUD execution.
//
// File: adapter.go
// Usage:
//   This file provides the core Adapter and Transaction contracts implemented by database
//   drivers (PostgreSQL, MySQL, MongoDB, and In-Memory). It also defines the Adapter Registry
//   which catalogs active driver instances and allows unified multi-database access.
package adapter

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/SanjayDrop5528/models-go-engine/execution"
	"github.com/SanjayDrop5528/models-go-engine/model"
	"github.com/SanjayDrop5528/models-go-engine/plan"
	"github.com/SanjayDrop5528/models-go-engine/query"
	"github.com/SanjayDrop5528/models-go-engine/schema"
)

// ErrOperationNotSupported is returned when an adapter does not support a requested operation type.
var ErrOperationNotSupported = errors.New("OPERATION_NOT_SUPPORTED")

// Transaction defines atomic unit-of-work capabilities across adapters.
type Transaction interface {
	Create(ctx context.Context, model model.ModelRef, data map[string]any) (map[string]any, error)
	Find(ctx context.Context, model model.ModelRef, q query.Query) ([]map[string]any, int64, error)
	FindOne(ctx context.Context, model model.ModelRef, id any) (map[string]any, error)
	Update(ctx context.Context, model model.ModelRef, id any, data map[string]any) (map[string]any, error)
	Patch(ctx context.Context, model model.ModelRef, id any, data map[string]any) (map[string]any, error)
	Delete(ctx context.Context, model model.ModelRef, id any) error
	Execute(ctx context.Context, req execution.ExecutionRequest) (*execution.ExecutionResult, error)
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

// Adapter defines the uniform database interface for schema lifecycle, CRUD, operations, and transactions.
type Adapter interface {
	Name() string
	DatabaseName() string
	NativeClient() any
	Connect(ctx context.Context) error
	Ping(ctx context.Context) error
	// Metadata Table Auto-Provisioning & Live Import
	EnsureMetadataTables(ctx context.Context) error
	ImportLiveMetadata(ctx context.Context) ([]*model.ModelConfig, []*model.DataModel, error)

	// Schema operations
	GetSchema(ctx context.Context, model model.ModelRef) (*schema.Schema, error)
	ValidateSchemaPlan(ctx context.Context, plan *plan.SchemaPlan) error
	PreviewSchemaChange(ctx context.Context, plan *plan.SchemaPlan) (*plan.SchemaPreview, error)
	ApplySchemaChange(ctx context.Context, plan *plan.SchemaPlan) error

	// Data CRUD operations
	Create(ctx context.Context, model model.ModelRef, data map[string]any) (map[string]any, error)
	Find(ctx context.Context, model model.ModelRef, q query.Query) ([]map[string]any, int64, error)
	FindOne(ctx context.Context, model model.ModelRef, id any) (map[string]any, error)
	Update(ctx context.Context, model model.ModelRef, id any, data map[string]any) (map[string]any, error)
	Patch(ctx context.Context, model model.ModelRef, id any, data map[string]any) (map[string]any, error)
	Delete(ctx context.Context, model model.ModelRef, id any) error

	// Generic Operation Execution (Function, Procedure, Command, etc.)
	Execute(ctx context.Context, req execution.ExecutionRequest) (*execution.ExecutionResult, error)

	// Transaction support
	Begin(ctx context.Context) (Transaction, error)
}

// DataSetCompiler abstracts converting QueryAST into target database code.
type DataSetCompiler interface {
	Compile(ctx context.Context, ast any, ds any) (any, error)
}

// DataSetAdapter is an optional interface implemented by adapters that support native dataset compilation.
type DataSetAdapter interface {
	Adapter
	DataSetCompiler() DataSetCompiler
}

// Registry manages initialized database adapters.
type Registry struct {
	mu       sync.RWMutex
	adapters map[string]Adapter
}

// NewRegistry creates a new adapter registry.
//
// Purpose:
//   Initializes an empty thread-safe Registry for database adapters.
//
// Where it is used:
//   - Instantiated during application bootstrap to manage multi-database connections.
//
// When can it be used:
//   - Whenever the application coordinates access across multiple storage drivers.
func NewRegistry() *Registry {
	return &Registry{
		adapters: make(map[string]Adapter),
	}
}

// Register adds an adapter under a name.
//
// Purpose:
//   Registers a database adapter instance under a unique driver or tenant key.
//
// Where it is used:
//   - Called after establishing database connections in service or server initialization.
//
// When can it be used:
//   - When making a database adapter globally discoverable by name.
func (r *Registry) Register(name string, a Adapter) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.adapters[name] = a
}

// Get retrieves an adapter by name.
//
// Purpose:
//   Looks up a registered database adapter, returning an error if not found.
//
// Where it is used:
//   - Called by service routers, execution engines, and dataset services to obtain driver instances.
//
// When can it be used:
//   - When dispatching a query or dataset execution to a specific driver.
func (r *Registry) Get(name string) (Adapter, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.adapters[name]
	if !ok {
		return nil, fmt.Errorf("database adapter '%s' not registered", name)
	}
	return a, nil
}
