// Package repository provides persistence abstractions and implementations for DataSet models.
//
// File: inmemory.go
// Usage:
//   This file implements the InMemDataSetRepository, providing an in-memory, thread-safe
//   store for DataSet configurations. It supports secondary lookup by reference name, status
//   filtering, and full CRUD operations without requiring database connectivity.
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
//
// Purpose:
//   Initializes an empty, thread-safe in-memory repository for dataset definitions.
//
// Where it is used:
//   - Used by DataSetService as a fallback when database adapter repository is not available.
//   - Used as an internal memory cache inside AdapterDataSetRepository.
//   - Used in unit and mock tests.
//
// When can it be used:
//   - Can be used whenever datasets need to be stored transiently without external database dependencies.
func NewDataSetRepository() *InMemDataSetRepository {
	return &InMemDataSetRepository{
		datasets: make(map[string]*domain.DataSet),
		byRef:    make(map[string]string),
	}
}

// Save stores or updates a DataSet in the in-memory map.
//
// Purpose:
//   Persists or updates a dataset, assigning a default ID, status, and timestamps if omitted.
//
// Where it is used:
//   - Called by DataSetService.Save when running in in-memory mode or caching adapter datasets.
//
// When can it be used:
//   - When storing newly designed or updated dataset configurations in memory.
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
//
// Purpose:
//   Looks up a dataset by its unique UUID or primary ID.
//
// Where it is used:
//   - Called by DataSetService.Get / Execute when retrieving a dataset by ID.
//
// When can it be used:
//   - When an API caller provides the dataset's unique primary identifier.
func (r *InMemDataSetRepository) FindByID(ctx context.Context, id string) (*domain.DataSet, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ds, exists := r.datasets[id]
	if !exists {
		return nil, domain.NewErrorf(domain.ErrDataSetNotFound, "dataset with id '%s' not found", id)
	}
	return ds, nil
}

// FindByReferenceName retrieves a DataSet by its human/reference name.
//
// Purpose:
//   Retrieves a dataset definition by human-friendly reference name with case-insensitive matching.
//
// Where it is used:
//   - Called by DataSetService.GetByReferenceName and preview/execution endpoints.
//
// When can it be used:
//   - When querying or executing a dataset by slug or reference name (e.g. "active_users_monthly").
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
//
// Purpose:
//   Lists all stored datasets, optionally filtering by status ("ACTIVE", "DRAFT", "ARCHIVED").
//
// Where it is used:
//   - Called by dataset catalog listing APIs.
//
// When can it be used:
//   - When listing datasets for administrative or studio browsing.
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
//
// Purpose:
//   Evicts a dataset from both the primary ID map and the reference name index.
//
// Where it is used:
//   - Called by DataSetService.Delete.
//
// When can it be used:
//   - When deleting an obsolete or draft dataset definition.
func (r *InMemDataSetRepository) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if ds, exists := r.datasets[id]; exists {
		delete(r.byRef, strings.ToLower(ds.ReferenceName))
		delete(r.datasets, id)
	}
	return nil
}
