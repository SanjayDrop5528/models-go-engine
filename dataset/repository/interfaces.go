// Package repository provides persistence abstractions and implementations for DataSet models.
//
// File: interfaces.go
// Usage:
//   This file defines the DataSetRepository interface for creating, retrieving, listing,
//   and deleting dataset configurations. It decouples the dataset service layer from the
//   storage mechanism, allowing datasets to be saved to database tables or in-memory stores.
package repository

import (
	"context"

	"github.com/SanjayDrop5528/models-go-engine/dataset/domain"
)

// DataSetRepository abstracts persistence of DataSet metadata.
//
// Purpose:
//   Defines CRUD persistence operations for DataSet definitions.
//
// Where it is used:
//   - Implemented by InMemDataSetRepository (inmemory.go)
//   - Implemented by AdapterDataSetRepository (adapter.go)
//   - Injected into DataSetService to handle Save, Load, and List requests.
//
// When can it be used:
//   - When persisting dataset studio configurations, loading by reference name, or listing datasets.
type DataSetRepository interface {
	// Save persists or updates a dataset definition.
	Save(ctx context.Context, ds *domain.DataSet) error

	// FindByID retrieves a dataset by its unique UUID/identifier.
	FindByID(ctx context.Context, id string) (*domain.DataSet, error)

	// FindByReferenceName retrieves a dataset by its unique reference name identifier.
	FindByReferenceName(ctx context.Context, refName string) (*domain.DataSet, error)

	// List returns all datasets matching an optional status filter ("ACTIVE", "DRAFT", "ARCHIVED").
	List(ctx context.Context, status string) ([]*domain.DataSet, error)

	// Delete removes a dataset from the repository.
	Delete(ctx context.Context, id string) error
}

