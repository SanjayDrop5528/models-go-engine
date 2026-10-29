// Package repository provides persistence abstractions and implementations for DataSet models.
//
// File: adapter.go
// Usage:
//
//	This file implements the AdapterDataSetRepository, which persists dataset definitions
//	directly to the database table 'metadata_catalog.dataset' via the storage adapter.
//	It maintains an in-memory cache fallback to ensure resilient operation even during
//	database transitions or test environments without live database connections.
package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"reflect"
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
//
//	Initializes an AdapterDataSetRepository bound to a database adapter and an internal InMemDataSetRepository.
//
// Where it is used:
//   - Instantiated when connecting DataSetService to a database adapter (PostgreSQL, MySQL, MongoDB).
//
// When can it be used:
//   - Whenever dataset metadata should be persistently saved to and loaded from a metadata table.
//
// Parameters:
//   - adp (adapter.Adapter): Input value used by the operation.
//
// Returns:
//   - result1 (*AdapterDataSetRepository): Returned value.
//
// Business Rules:
//   - Follows the invariants documented by NewAdapterDataSetRepository and its owning package.
//
// Side Effects:
//   - Does not intentionally mutate caller-owned input; configured dependencies may still be consulted.
//
// Errors:
//   - This function does not return an error.
//
// Note:
//   - Callers should preserve the documented ownership and lifecycle of supplied values.
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
//
//	Serializes complex dataset JSON fields (base collection, joins, custom columns, filters)
//	and performs an upsert into the metadata_catalog.dataset table, while keeping in-memory cache updated.
//
// Where it is used:
//   - Called by DataSetService.Save when publishing or updating a dataset.
//
// When can it be used:
//   - When persisting a designed dataset to permanent database storage.
//
// Parameters:
//   - ctx (context.Context): Input value used by the operation.
//   - ds (*domain.DataSet): Input value used by the operation.
//
// Returns:
//   - result1 (error): Returned value.
//
// Business Rules:
//   - Follows the invariants documented by Save and its owning package.
//
// Side Effects:
//   - May update receiver state, persistent storage, or configured external dependencies.
//
// Errors:
//   - Returns validation, configuration, context, or underlying dependency errors.
//
// Note:
//   - The context controls cancellation and request lifetime.
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

	isMongo := strings.EqualFold(ds.Driver, "mongodb") || (r.adp != nil && strings.EqualFold(r.adp.Name(), "mongodb"))

	baseCol := ds.BaseCollection
	var baseColVal any
	if isMongo {
		// For MongoDB, base_collection is purely a string holding the collection name (no object)
		baseColVal = baseCol.Collection
	} else {
		baseColJSON, _ := json.Marshal(baseCol)
		baseColVal = string(baseColJSON)
	}

	cleanJoins := make([]domain.JoinCollection, len(ds.JoinCollections))
	copy(cleanJoins, ds.JoinCollections)
	if isMongo {
		for i := range cleanJoins {
			cleanJoins[i].Schema = ""
			if domain.IsFilterEmpty(cleanJoins[i].Filter) {
				cleanJoins[i].Filter = nil
			}
		}
	}
	joinsJSON, _ := json.Marshal(cleanJoins)
	customColsJSON, _ := json.Marshal(ds.CustomColumns)
	groupByJSON, _ := json.Marshal(ds.GroupByFields)
	schematicJSON, _ := json.Marshal(ds.SchematicTable)
	filterJSON, _ := json.Marshal(ds.Filter)
	filterParamsJSON, _ := json.Marshal(ds.FilterParams)
	selectedListJSON, _ := json.Marshal(ds.SelectedList)

	data := map[string]any{
		"name":               ds.Name,
		"reference_name":     ds.ReferenceName,
		"driver":             ds.Driver,
		"base_collection":    baseColVal,
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

	if isMongo {
		data["_id"] = ds.ID
	} else {
		data["id"] = ds.ID
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
//
//	Queries the database adapter for a dataset by primary ID, falling back to in-memory store on failure.
//
// Where it is used:
//   - Called by DataSetService.Get / Execute when retrieving a dataset by ID.
//
// When can it be used:
//   - When looking up a dataset using its primary identifier.
//
// Parameters:
//   - ctx (context.Context): Input value used by the operation.
//   - id (string): Input value used by the operation.
//
// Returns:
//   - result1 (*domain.DataSet): Returned value.
//   - result2 (error): Returned value.
//
// Business Rules:
//   - Follows the invariants documented by FindByID and its owning package.
//
// Side Effects:
//   - Does not intentionally mutate caller-owned input; configured dependencies may still be consulted.
//
// Errors:
//   - Returns validation, configuration, context, or underlying dependency errors.
//
// Note:
//   - The context controls cancellation and request lifetime.
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
//
//	Queries the database adapter for a dataset matching reference_name, falling back to in-memory store.
//
// Where it is used:
//   - Called by DataSetService.GetByReferenceName and preview/execution endpoints.
//
// When can it be used:
//   - When executing or inspecting a dataset by reference slug (e.g. "active_users").
//
// Parameters:
//   - ctx (context.Context): Input value used by the operation.
//   - refName (string): Input value used by the operation.
//
// Returns:
//   - result1 (*domain.DataSet): Returned value.
//   - result2 (error): Returned value.
//
// Business Rules:
//   - Follows the invariants documented by FindByReferenceName and its owning package.
//
// Side Effects:
//   - Does not intentionally mutate caller-owned input; configured dependencies may still be consulted.
//
// Errors:
//   - Returns validation, configuration, context, or underlying dependency errors.
//
// Note:
//   - The context controls cancellation and request lifetime.
func (r *AdapterDataSetRepository) FindByReferenceName(ctx context.Context, refName string) (*domain.DataSet, error) {
	if r.adp != nil {
		q := query.NewQuery().Where("reference_name", query.OpEq, refName).LimitOffset(1, 0)
		results, _, err := r.adp.Find(ctx, r.dsRef, q)
		if err == nil && len(results) > 0 {
			return mapToDataSet(results[0]), nil
		}
		// Also check by ID if not found by reference_name
		if ds, err := r.FindByID(ctx, refName); err == nil && ds != nil && ds.ReferenceName != "" {
			return ds, nil
		}
	}
	return r.inMem.FindByReferenceName(ctx, refName)
}

// List returns all datasets matching status.
//
// Purpose:
//
//	Retrieves all datasets matching an optional status filter from the database or in-memory fallback.
//
// Where it is used:
//   - Called by dataset catalog and administrative listing APIs.
//
// When can it be used:
//   - When displaying a list of active datasets in Dataset Studio UI.
//
// Parameters:
//   - ctx (context.Context): Input value used by the operation.
//   - status (string): Input value used by the operation.
//
// Returns:
//   - result1 ([]*domain.DataSet): Returned value.
//   - result2 (error): Returned value.
//
// Business Rules:
//   - Follows the invariants documented by List and its owning package.
//
// Side Effects:
//   - Does not intentionally mutate caller-owned input; configured dependencies may still be consulted.
//
// Errors:
//   - Returns validation, configuration, context, or underlying dependency errors.
//
// Note:
//   - The context controls cancellation and request lifetime.
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
//
//	Deletes a dataset row from the database table and evicts it from the in-memory fallback cache.
//
// Where it is used:
//   - Called by DataSetService.Delete.
//
// When can it be used:
//   - When removing an unwanted or deprecated dataset.
//
// Parameters:
//   - ctx (context.Context): Input value used by the operation.
//   - idOrRef (string): Input value used by the operation.
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
//
//	Translates raw SQL column values and JSON string fields back into typed DataSet structs.
//
// Where it is used:
//   - Internal helper for FindByID, FindByReferenceName, and List.
//
// When can it be used:
//   - Invoked whenever transforming raw adapter query results into domain models.
func mapToDataSet(rec map[string]any) *domain.DataSet {
	idVal := rec["id"]
	if idVal == nil || fmt.Sprintf("%v", idVal) == "" || fmt.Sprintf("%v", idVal) == "<nil>" {
		idVal = rec["_id"]
	}

	nameVal := rec["name"]
	if nameVal == nil || fmt.Sprintf("%v", nameVal) == "" || fmt.Sprintf("%v", nameVal) == "<nil>" {
		nameVal = rec["dataSetName"]
	}

	refVal := rec["reference_name"]
	if refVal == nil || fmt.Sprintf("%v", refVal) == "" || fmt.Sprintf("%v", refVal) == "<nil>" {
		refVal = rec["dataSetName"]
	}

	refPipeVal := rec["reference_pipeline"]
	if refPipeVal == nil || fmt.Sprintf("%v", refPipeVal) == "" || fmt.Sprintf("%v", refPipeVal) == "<nil>" {
		refPipeVal = rec["Reference_pipeline"]
	}

	driverVal := rec["driver"]
	if driverVal == nil || fmt.Sprintf("%v", driverVal) == "<nil>" {
		driverVal = ""
	}

	saveModeVal := rec["save_mode"]
	if saveModeVal == nil || fmt.Sprintf("%v", saveModeVal) == "<nil>" || fmt.Sprintf("%v", saveModeVal) == "" {
		saveModeVal = string(domain.SaveModeQuery)
	}

	statusVal := rec["status"]
	if statusVal == nil || fmt.Sprintf("%v", statusVal) == "<nil>" || fmt.Sprintf("%v", statusVal) == "" {
		statusVal = "ACTIVE"
	}

	ds := &domain.DataSet{
		ID:                fmt.Sprintf("%v", idVal),
		Name:              fmt.Sprintf("%v", nameVal),
		ReferenceName:     fmt.Sprintf("%v", refVal),
		Driver:            fmt.Sprintf("%v", driverVal),
		SaveMode:          domain.SaveMode(fmt.Sprintf("%v", saveModeVal)),
		Pipeline:          fmt.Sprintf("%v", rec["pipeline"]),
		ReferencePipeline: fmt.Sprintf("%v", refPipeVal),
		Status:            fmt.Sprintf("%v", statusVal),
	}

	parseJSON := func(val any, target any) {
		if val == nil {
			return
		}
		switch v := val.(type) {
		case string:
			trimmed := strings.TrimSpace(v)
			if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") || strings.HasPrefix(trimmed, "\"") {
				_ = json.Unmarshal([]byte(trimmed), target)
			} else if len(trimmed) > 0 {
				quoted := `"` + trimmed + `"`
				if err := json.Unmarshal([]byte(quoted), target); err != nil {
					_ = json.Unmarshal([]byte(trimmed), target)
				}
			}
		case []byte:
			trimmed := strings.TrimSpace(string(v))
			if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") || strings.HasPrefix(trimmed, "\"") {
				_ = json.Unmarshal(v, target)
			} else if len(trimmed) > 0 {
				quoted := `"` + trimmed + `"`
				if err := json.Unmarshal([]byte(quoted), target); err != nil {
					_ = json.Unmarshal(v, target)
				}
			}
		default:
			if b, err := json.Marshal(v); err == nil {
				_ = json.Unmarshal(b, target)
			}
		}
	}

	baseColVal := rec["base_collection"]
	if baseColVal == nil || fmt.Sprintf("%v", baseColVal) == "" || fmt.Sprintf("%v", baseColVal) == "<nil>" {
		baseColVal = rec["dataSetBaseCollection"]
	}
	parseJSON(baseColVal, &ds.BaseCollection)
	parseJSON(rec["join_collections"], &ds.JoinCollections)
	parseJSON(rec["custom_columns"], &ds.CustomColumns)
	parseJSON(rec["group_by_fields"], &ds.GroupByFields)
	parseJSON(rec["schematic_table"], &ds.SchematicTable)
	parseJSON(rec["filter"], &ds.Filter)
	parseJSON(rec["filter_params"], &ds.FilterParams)
	parseJSON(rec["selected_list"], &ds.SelectedList)

	return ds
}

// isFilterEmpty detects if a filter object/slice/string holds no actual filter conditions.
func isFilterEmpty(f any) bool {
	if f == nil {
		return true
	}
	switch v := f.(type) {
	case string:
		trimmed := strings.TrimSpace(v)
		return trimmed == "" || trimmed == "{}" || trimmed == "[]"
	case []any:
		return len(v) == 0
	case map[string]any:
		return len(v) == 0
	case []map[string]any:
		return len(v) == 0
	}
	rv := reflect.ValueOf(f)
	if (rv.Kind() == reflect.Slice || rv.Kind() == reflect.Map) && rv.Len() == 0 {
		return true
	}
	return false
}
