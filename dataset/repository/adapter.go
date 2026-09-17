// Package repository provides persistence abstractions and implementations for DataSet models.
//
// File: adapter.go
// Usage:
//   This file implements the AdapterDataSetRepository, which persists dataset definitions
//   directly to the database table 'metadata_catalog.dataset' via the storage adapter.
//   It maintains an in-memory cache fallback to ensure resilient operation even during
//   database transitions or test environments without live database connections.
package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/SanjayDrop5528/models-go-engine/adapter"
	"github.com/SanjayDrop5528/models-go-engine/dataset/domain"
	"github.com/SanjayDrop5528/models-go-engine/model"
	"github.com/SanjayDrop5528/models-go-engine/query"
)

// AdapterDataSetRepository provides database-backed dataset storage using the database adapter.
type AdapterDataSetRepository struct {
	adp   adapter.Adapter
	inMem *InMemDataSetRepository
	dsRef model.ModelRef
}

// NewAdapterDataSetRepository creates a new adapter-backed dataset repository.
//
// Purpose:
//   Initializes an AdapterDataSetRepository bound to a database adapter and an internal InMemDataSetRepository.
//
// Where it is used:
//   - Instantiated when connecting DataSetService to a database adapter (PostgreSQL, MySQL, MongoDB).
//
// When can it be used:
//   - Whenever dataset metadata should be persistently saved to and loaded from a metadata table.
func NewAdapterDataSetRepository(adp adapter.Adapter) *AdapterDataSetRepository {
	return &AdapterDataSetRepository{
		adp:   adp,
		inMem: NewDataSetRepository(),
		dsRef: model.ModelRef{
			StorageName: "dataset",
			Name:        "dataset",
		},
	}
}

// Save persists dataset metadata to the database table 'metadata_catalog.dataset'.
//
// Purpose:
//   Serializes complex dataset JSON fields (base collection, joins, custom columns, filters)
//   and performs an upsert into the metadata_catalog.dataset table, while keeping in-memory cache updated.
//
// Where it is used:
//   - Called by DataSetService.Save when publishing or updating a dataset.
//
// When can it be used:
//   - When persisting a designed dataset to permanent database storage.
func (r *AdapterDataSetRepository) Save(ctx context.Context, ds *domain.DataSet) error {
	if ds == nil || ds.ReferenceName == "" {
		return domain.NewError(domain.ErrDataSetNotFound, "invalid dataset: reference_name is required")
	}

	if ds.ID == "" {
		if existing, err := r.FindByReferenceName(ctx, ds.ReferenceName); err == nil && existing != nil && existing.ID != "" {
			ds.ID = existing.ID
		} else {
			ds.ID = "ds_" + strings.ToLower(ds.ReferenceName)
		}
	}
	if ds.Status == "" {
		ds.Status = "ACTIVE"
	}
	if ds.CreatedAt.IsZero() {
		ds.CreatedAt = time.Now()
	}
	ds.UpdatedAt = time.Now()

	// Update fallback in-memory store
	_ = r.inMem.Save(ctx, ds)

	if r.adp == nil {
		return nil
	}

	baseColJSON, _ := json.Marshal(ds.BaseCollection)
	joinsJSON, _ := json.Marshal(ds.JoinCollections)
	customColsJSON, _ := json.Marshal(ds.CustomColumns)
	groupByJSON, _ := json.Marshal(ds.GroupByFields)
	schematicJSON, _ := json.Marshal(ds.SchematicTable)
	filterJSON, _ := json.Marshal(ds.Filter)
	filterParamsJSON, _ := json.Marshal(ds.FilterParams)
	selectedListJSON, _ := json.Marshal(ds.SelectedList)

	data := map[string]any{
		"id":                 ds.ID,
		"name":               ds.Name,
		"reference_name":     ds.ReferenceName,
		"driver":             ds.Driver,
		"base_collection":    string(baseColJSON),
		"join_collections":   string(joinsJSON),
		"custom_columns":     string(customColsJSON),
		"group_by_fields":    string(groupByJSON),
		"schematic_table":    string(schematicJSON),
		"filter":             string(filterJSON),
		"filter_params":      string(filterParamsJSON),
		"selected_list":      string(selectedListJSON),
		"save_mode":          string(ds.SaveMode),
		"pipeline":           ds.Pipeline,
		"reference_pipeline": ds.ReferencePipeline,
		"status":             ds.Status,
		"updated_at":         ds.UpdatedAt.Format(time.RFC3339),
	}

	// Try Update first; if not found, Create
	_, err := r.adp.Update(ctx, r.dsRef, ds.ID, data)
	if err != nil {
		data["created_at"] = ds.CreatedAt.Format(time.RFC3339)
		if _, createErr := r.adp.Create(ctx, r.dsRef, data); createErr != nil {
			log.Printf("[AdapterDataSetRepository] ⚠ Could not persist dataset '%s' to DB: %v", ds.ReferenceName, createErr)
		} else {
			log.Printf("[AdapterDataSetRepository] ✔ Persisted dataset '%s' to metadata_catalog.dataset table via adapter.", ds.ReferenceName)
		}
	} else {
		log.Printf("[AdapterDataSetRepository] ✔ Updated dataset '%s' in metadata_catalog.dataset table via adapter.", ds.ReferenceName)
	}

	return nil
}

// FindByID retrieves a dataset from DB or fallback in-memory store.
//
// Purpose:
//   Queries the database adapter for a dataset by primary ID, falling back to in-memory store on failure.
//
// Where it is used:
//   - Called by DataSetService.Get / Execute when retrieving a dataset by ID.
//
// When can it be used:
//   - When looking up a dataset using its primary identifier.
func (r *AdapterDataSetRepository) FindByID(ctx context.Context, id string) (*domain.DataSet, error) {
	if r.adp != nil {
		rec, err := r.adp.FindOne(ctx, r.dsRef, id)
		if err == nil && rec != nil {
			return mapToDataSet(rec), nil
		}
	}
	return r.inMem.FindByID(ctx, id)
}

// FindByReferenceName retrieves a dataset by its unique reference_name.
//
// Purpose:
//   Queries the database adapter for a dataset matching reference_name, falling back to in-memory store.
//
// Where it is used:
//   - Called by DataSetService.GetByReferenceName and preview/execution endpoints.
//
// When can it be used:
//   - When executing or inspecting a dataset by reference slug (e.g. "active_users").
func (r *AdapterDataSetRepository) FindByReferenceName(ctx context.Context, refName string) (*domain.DataSet, error) {
	if r.adp != nil {
		q := query.NewQuery().Where("reference_name", query.OpEq, refName).LimitOffset(1, 0)
		results, _, err := r.adp.Find(ctx, r.dsRef, q)
		if err == nil && len(results) > 0 {
			return mapToDataSet(results[0]), nil
		}
	}
	return r.inMem.FindByReferenceName(ctx, refName)
}

// List returns all datasets matching status.
//
// Purpose:
//   Retrieves all datasets matching an optional status filter from the database or in-memory fallback.
//
// Where it is used:
//   - Called by dataset catalog and administrative listing APIs.
//
// When can it be used:
//   - When displaying a list of active datasets in Dataset Studio UI.
func (r *AdapterDataSetRepository) List(ctx context.Context, status string) ([]*domain.DataSet, error) {
	if r.adp != nil {
		q := query.NewQuery()
		if status != "" {
			q = q.Where("status", query.OpEq, status)
		}
		results, _, err := r.adp.Find(ctx, r.dsRef, q)
		if err == nil && len(results) > 0 {
			var list []*domain.DataSet
			for _, rec := range results {
				list = append(list, mapToDataSet(rec))
			}
			return list, nil
		}
	}
	return r.inMem.List(ctx, status)
}

// Delete removes a dataset by ID.
//
// Purpose:
//   Deletes a dataset row from the database table and evicts it from the in-memory fallback cache.
//
// Where it is used:
//   - Called by DataSetService.Delete.
//
// When can it be used:
//   - When removing an unwanted or deprecated dataset.
func (r *AdapterDataSetRepository) Delete(ctx context.Context, idOrRef string) error {
	targetID := idOrRef
	if existing, err := r.FindByReferenceName(ctx, idOrRef); err == nil && existing != nil && existing.ID != "" {
		targetID = existing.ID
	}
	if r.adp != nil {
		_ = r.adp.Delete(ctx, r.dsRef, targetID)
	}
	_ = r.inMem.Delete(ctx, targetID)
	_ = r.inMem.Delete(ctx, idOrRef)
	return nil
}

// mapToDataSet deserializes a database record map into a domain.DataSet struct.
//
// Purpose:
//   Translates raw SQL column values and JSON string fields back into typed DataSet structs.
//
// Where it is used:
//   - Internal helper for FindByID, FindByReferenceName, and List.
//
// When can it be used:
//   - Invoked whenever transforming raw adapter query results into domain models.
func mapToDataSet(rec map[string]any) *domain.DataSet {
	ds := &domain.DataSet{
		ID:                fmt.Sprintf("%v", rec["id"]),
		Name:              fmt.Sprintf("%v", rec["name"]),
		ReferenceName:     fmt.Sprintf("%v", rec["reference_name"]),
		Driver:            fmt.Sprintf("%v", rec["driver"]),
		SaveMode:          domain.SaveMode(fmt.Sprintf("%v", rec["save_mode"])),
		Pipeline:          fmt.Sprintf("%v", rec["pipeline"]),
		ReferencePipeline: fmt.Sprintf("%v", rec["reference_pipeline"]),
		Status:            fmt.Sprintf("%v", rec["status"]),
	}

	parseJSON := func(val any, target any) {
		if val == nil {
			return
		}
		switch v := val.(type) {
		case string:
			_ = json.Unmarshal([]byte(v), target)
		case []byte:
			_ = json.Unmarshal(v, target)
		default:
			if b, err := json.Marshal(v); err == nil {
				_ = json.Unmarshal(b, target)
			}
		}
	}

	parseJSON(rec["base_collection"], &ds.BaseCollection)
	parseJSON(rec["join_collections"], &ds.JoinCollections)
	parseJSON(rec["custom_columns"], &ds.CustomColumns)
	parseJSON(rec["group_by_fields"], &ds.GroupByFields)
	parseJSON(rec["schematic_table"], &ds.SchematicTable)
	parseJSON(rec["filter"], &ds.Filter)
	parseJSON(rec["filter_params"], &ds.FilterParams)
	parseJSON(rec["selected_list"], &ds.SelectedList)

	return ds
}
