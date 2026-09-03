package repository

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/SanjayDrop5528/models-go-engine/dataset/domain"
)

// InMemDataSetRepository provides thread-safe in-memory storage of DataSet entities.
type InMemDataSetRepository struct {
	mu       sync.RWMutex
	datasets map[string]*domain.DataSet
	byRef    map[string]string // refName -> id
}

// NewDataSetRepository creates a new in-memory dataset repository.
func NewDataSetRepository() *InMemDataSetRepository {
	return &InMemDataSetRepository{
		datasets: make(map[string]*domain.DataSet),
		byRef:    make(map[string]string),
	}
}

// Save stores or updates a DataSet.
func (r *InMemDataSetRepository) Save(ctx context.Context, ds *domain.DataSet) error {
	if ds == nil || ds.ReferenceName == "" {
		return domain.NewError(domain.ErrDataSetNotFound, "invalid dataset: reference_name is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	if ds.ID == "" {
		ds.ID = "ds_" + strings.ToLower(ds.ReferenceName)
	}
	if ds.Status == "" {
		ds.Status = "ACTIVE"
	}
	if ds.CreatedAt.IsZero() {
		ds.CreatedAt = time.Now()
	}
	ds.UpdatedAt = time.Now()

	r.datasets[ds.ID] = ds
	r.byRef[strings.ToLower(ds.ReferenceName)] = ds.ID
	return nil
}

// FindByID retrieves a DataSet by its primary identifier.
func (r *InMemDataSetRepository) FindByID(ctx context.Context, id string) (*domain.DataSet, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ds, exists := r.datasets[id]
	if !exists {
		return nil, domain.NewErrorf(domain.ErrDataSetNotFound, "dataset with id '%s' not found", id)
	}
	return ds, nil
}

// FindByReferenceName retrieves a DataSet by reference name.
func (r *InMemDataSetRepository) FindByReferenceName(ctx context.Context, refName string) (*domain.DataSet, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	id, exists := r.byRef[strings.ToLower(refName)]
	if !exists {
		// check direct id fallback
		if direct, hasDirect := r.datasets[refName]; hasDirect {
			return direct, nil
		}
		return nil, domain.NewErrorf(domain.ErrDataSetNotFound, "dataset with reference_name '%s' not found", refName)
	}
	return r.datasets[id], nil
}

// List returns datasets matching the status filter.
func (r *InMemDataSetRepository) List(ctx context.Context, status string) ([]*domain.DataSet, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var list []*domain.DataSet
	for _, ds := range r.datasets {
		if status == "" || strings.EqualFold(ds.Status, status) {
			list = append(list, ds)
		}
	}
	return list, nil
}

// Delete removes a DataSet by ID.
func (r *InMemDataSetRepository) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if ds, exists := r.datasets[id]; exists {
		delete(r.byRef, strings.ToLower(ds.ReferenceName))
		delete(r.datasets, id)
	}
	return nil
}
