package adapter

import (
	"context"
	"errors"
	"fmt"
	"github.com/SanjayDrop5528/models-go-engine/execution"
	"github.com/SanjayDrop5528/models-go-engine/model"
	"github.com/SanjayDrop5528/models-go-engine/plan"
	"github.com/SanjayDrop5528/models-go-engine/query"
	"github.com/SanjayDrop5528/models-go-engine/schema"
	"sync"
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

// Registry manages initialized database adapters.
type Registry struct {
	mu       sync.RWMutex
	adapters map[string]Adapter
}

// NewRegistry creates a new adapter registry.
func NewRegistry() *Registry {
	return &Registry{
		adapters: make(map[string]Adapter),
	}
}

// Register adds an adapter under a name.
func (r *Registry) Register(name string, a Adapter) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.adapters[name] = a
}

// Get retrieves an adapter by name.
func (r *Registry) Get(name string) (Adapter, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.adapters[name]
	if !ok {
		return nil, fmt.Errorf("database adapter '%s' not registered", name)
	}
	return a, nil
}
