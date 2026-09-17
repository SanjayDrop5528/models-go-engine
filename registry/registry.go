// Package registry manages in-memory catalogs of active models, drafts, field definitions, and custom operation configurations.
//
// File: registry.go
// Usage:
//   Provides the thread-safe ModelRegistry which stores and coordinates model lifecycles
//   (draft -> applying -> active), column definitions, and operations.
package registry

import (
	"errors"
	"fmt"
	"github.com/SanjayDrop5528/models-go-engine/model"
	"github.com/SanjayDrop5528/models-go-engine/operation"
	"strings"
	"sync"
	"time"
)

// ModelRegistry provides thread-safe in-memory storage for active and draft model metadata definitions.
type ModelRegistry struct {
	mu               sync.RWMutex
	active           map[string]*model.Model
	drafts           map[string]*model.Model
	byName           map[string]string // Name -> ID
	modelConfigs     map[string]*model.ModelConfig
	dataModels       map[string]map[string]*model.DataModel // ModelID -> DataModelID -> DataModel
	operationConfigs map[string]*operation.OperationConfig
}

// NewModelRegistry creates a new ModelRegistry.
//
// Purpose:
//   Instantiates the thread-safe in-memory model and operation catalog.
//
// Where it is used:
//   - Initialized during server bootstrap and used across services and adapters.
//
// When can it be used:
//   - Call when initializing the engine runtime or isolated test suites.
func NewModelRegistry() *ModelRegistry {
	return &ModelRegistry{
		active:           make(map[string]*model.Model),
		drafts:           make(map[string]*model.Model),
		byName:           make(map[string]string),
		modelConfigs:     make(map[string]*model.ModelConfig),
		dataModels:       make(map[string]map[string]*model.DataModel),
		operationConfigs: make(map[string]*operation.OperationConfig),
	}
}

// SaveOperationConfig stores an operation definition.
//
// Purpose:
//   Persists or updates custom procedure, function, or command configuration in the registry.
//
// Where it is used:
//   - Called by operation registration APIs and project initializers.
//
// When can it be used:
//   - Call whenever defining or modifying an executable operation.
func (r *ModelRegistry) SaveOperationConfig(op *operation.OperationConfig) (*operation.OperationConfig, error) {
	if op == nil {
		return nil, errors.New("operation_config cannot be nil")
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	cp := *op
	if cp.ID == "" {
		cp.ID = cp.Name
	}
	cp.UpdatedAt = time.Now().UTC()
	if cp.CreatedAt.IsZero() {
		cp.CreatedAt = cp.UpdatedAt
	}

	r.operationConfigs[strings.ToLower(cp.Name)] = &cp
	r.operationConfigs[strings.ToLower(cp.ID)] = &cp
	return &cp, nil
}

// GetOperationConfig retrieves an operation definition by name or ID.
//
// Purpose:
//   Looks up a registered procedure, function, or command configuration.
//
// Where it is used:
//   - Called by execution handlers and function resolvers.
//
// When can it be used:
//   - Call before executing a custom procedure or function.
func (r *ModelRegistry) GetOperationConfig(nameOrID string) (*operation.OperationConfig, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	op, ok := r.operationConfigs[strings.ToLower(nameOrID)]
	if !ok {
		return nil, fmt.Errorf("operation '%s' not found", nameOrID)
	}
	cp := *op
	return &cp, nil
}

// ListOperationConfigs returns all registered operation definitions.
//
// Purpose:
//   Lists all registered procedures, functions, and commands in the system.
//
// Where it is used:
//   - Called by GET /api/operations endpoints and documentation generators.
//
// When can it be used:
//   - Call when cataloging available operations.
func (r *ModelRegistry) ListOperationConfigs() []*operation.OperationConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()

	seen := make(map[string]*operation.OperationConfig)
	for _, op := range r.operationConfigs {
		seen[op.ID] = op
	}

	result := make([]*operation.OperationConfig, 0, len(seen))
	for _, op := range seen {
		cp := *op
		result = append(result, &cp)
	}
	return result
}

// SaveModelConfig saves or updates a model_config.
//
// Purpose:
//   Stores high-level model metadata (refName, database, description, version).
//
// Where it is used:
//   - Called by project management services and schema designers.
//
// When can it be used:
//   - Call when creating or updating model configuration entities.
func (r *ModelRegistry) SaveModelConfig(cfg *model.ModelConfig) (*model.ModelConfig, error) {
	if cfg == nil {
		return nil, errors.New("model_config cannot be nil")
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	cp := *cfg
	if cp.ID == "" {
		cp.ID = cp.Name
	}
	if cp.Status == "" {
		cp.Status = model.ModelConfigStatusDraft
	}
	if cp.Version == 0 {
		cp.Version = 1
	}
	cp.UpdatedAt = time.Now().UTC()
	if cp.CreatedAt.IsZero() {
		cp.CreatedAt = cp.UpdatedAt
	}

	r.modelConfigs[cp.ID] = &cp
	r.byName[strings.ToLower(cp.Name)] = cp.ID
	r.byName[strings.ToLower(cp.ID)] = cp.ID
	if cp.RefName != "" {
		r.byName[strings.ToLower(cp.RefName)] = cp.ID
	}
	return &cp, nil
}

// GetModelConfig retrieves a model_config by ID or Name.
//
// Purpose:
//   Finds model configuration by identifier, name, or refName.
//
// Where it is used:
//   - Called by API controllers and project synchronization services.
//
// When can it be used:
//   - Call when fetching model metadata.
func (r *ModelRegistry) GetModelConfig(idOrName string) (*model.ModelConfig, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	id := r.resolveID(idOrName)
	cfg, ok := r.modelConfigs[id]
	if !ok {
		return nil, fmt.Errorf("model_config '%s' not found", idOrName)
	}
	cp := *cfg
	return &cp, nil
}

// ListModelConfigs returns all stored model_configs.
//
// Purpose:
//   Retrieves a slice of all registered model_config objects.
//
// Where it is used:
//   - Called by GET /api/models endpoints.
//
// When can it be used:
//   - Call when listing configured models.
func (r *ModelRegistry) ListModelConfigs() []*model.ModelConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*model.ModelConfig, 0, len(r.modelConfigs))
	for _, cfg := range r.modelConfigs {
		cp := *cfg
		result = append(result, &cp)
	}
	return result
}

// SaveDataModel saves or updates a data_model field definition.
//
// Purpose:
//   Stores field-level attribute metadata (type, length, precision, scale, constraints).
//
// Where it is used:
//   - Called when configuring or modifying model attributes.
//
// When can it be used:
//   - Call when adding or editing a model column or field.
func (r *ModelRegistry) SaveDataModel(dm *model.DataModel) (*model.DataModel, error) {
	if dm == nil {
		return nil, errors.New("data_model cannot be nil")
	}
	if dm.ModelID == "" {
		return nil, errors.New("data_model must have model_id")
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	cp := *dm
	if cp.ID == "" {
		field := cp.ColumnName
		if field == "" {
			field = cp.JSONField
		}
		cp.ID = fmt.Sprintf("%s_%s", cp.ModelID, field)
	}
	if cp.Status == "" {
		cp.Status = model.DataModelStatusActive
	}
	cp.UpdatedAt = time.Now().UTC()
	if cp.CreatedAt.IsZero() {
		cp.CreatedAt = cp.UpdatedAt
	}
	targetCol := cp.ColumnName
	if targetCol == "" {
		targetCol = cp.JSONField
	}
	if targetCol != "" {
		for _, existing := range r.dataModels[cp.ModelID] {
			if existing.ID != cp.ID {
				existingCol := existing.ColumnName
				if existingCol == "" {
					existingCol = existing.JSONField
				}
				if strings.EqualFold(existingCol, targetCol) {
					return nil, fmt.Errorf("column name '%s' already exists in model '%s'. Duplicate column names are not allowed", targetCol, cp.ModelID)
				}
			}
		}
	}

	if _, ok := r.dataModels[cp.ModelID]; !ok {
		r.dataModels[cp.ModelID] = make(map[string]*model.DataModel)
	}
	r.dataModels[cp.ModelID][cp.ID] = &cp
	return &cp, nil
}

// GetDataModel retrieves a data_model field definition by model ID and field ID (or column_name/json_field).
//
// Purpose:
//   Looks up field metadata within a model scope.
//
// Where it is used:
//   - Called by field inspection endpoints and validators.
//
// When can it be used:
//   - Call when fetching metadata for a specific column or field.
func (r *ModelRegistry) GetDataModel(modelID, fieldID string) (*model.DataModel, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	fields, ok := r.dataModels[modelID]
	if !ok {
		return nil, fmt.Errorf("no data_model fields found for model '%s'", modelID)
	}
	if dm, ok := fields[fieldID]; ok {
		cp := *dm
		return &cp, nil
	}
	for _, dm := range fields {
		if strings.EqualFold(dm.ColumnName, fieldID) || strings.EqualFold(dm.JSONField, fieldID) || strings.EqualFold(dm.ID, fieldID) {
			cp := *dm
			return &cp, nil
		}
	}
	return nil, fmt.Errorf("data_model field '%s' not found in model '%s'", fieldID, modelID)
}

// ListDataModels returns all data_model fields for a model.
//
// Purpose:
//   Returns all column and field definitions for a specified model.
//
// Where it is used:
//   - Called by GET /api/models/:model/fields endpoints.
//
// When can it be used:
//   - Call when viewing all columns of a model.
func (r *ModelRegistry) ListDataModels(modelID string) []*model.DataModel {
	r.mu.RLock()
	defer r.mu.RUnlock()

	fields, ok := r.dataModels[modelID]
	if !ok {
		return nil
	}
	result := make([]*model.DataModel, 0, len(fields))
	for _, dm := range fields {
		cp := *dm
		result = append(result, &cp)
	}
	return result
}

// DeleteDataModel removes a data_model field definition by ID or column_name.
//
// Purpose:
//   Deletes a column or field definition from a model.
//
// Where it is used:
//   - Called by DELETE /api/models/:model/fields/:field endpoints.
//
// When can it be used:
//   - Call when dropping a field definition.
func (r *ModelRegistry) DeleteDataModel(modelID, fieldID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if fields, ok := r.dataModels[modelID]; ok {
		if _, ok := fields[fieldID]; ok {
			delete(fields, fieldID)
			return nil
		}
		for id, dm := range fields {
			if strings.EqualFold(dm.ColumnName, fieldID) || strings.EqualFold(dm.JSONField, fieldID) || strings.EqualFold(dm.ID, fieldID) {
				delete(fields, id)
				return nil
			}
		}
	}
	return nil
}

// SaveDraft saves or updates a model draft in DRAFT state.
//
// Purpose:
//   Stores an unapplied model definition in draft state pending schema migration.
//
// Where it is used:
//   - Called when editing models in UI or creating new models.
//
// When can it be used:
//   - Call prior to previewing or applying schema migrations.
func (r *ModelRegistry) SaveDraft(m *model.Model) (*model.Model, error) {
	if m == nil {
		return nil, errors.New("model cannot be nil")
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	// Clone to avoid mutating outside
	draft := cloneModel(m)
	if draft.ID == "" {
		draft.ID = draft.Name
	}
	draft.Status = model.StatusDraft
	draft.UpdatedAt = time.Now().UTC()
	if draft.CreatedAt.IsZero() {
		draft.CreatedAt = draft.UpdatedAt
	}

	r.drafts[draft.ID] = draft
	r.byName[strings.ToLower(draft.Name)] = draft.ID
	r.byName[strings.ToLower(draft.ID)] = draft.ID
	if draft.StorageName != "" {
		r.byName[strings.ToLower(draft.StorageName)] = draft.ID
	}
	return draft, nil
}

// GetDraft returns the draft version of a model.
//
// Purpose:
//   Retrieves the pending draft definition of a model (falling back to active if no draft exists).
//
// Where it is used:
//   - Called by SchemaService during diffing, previewing, and applying.
//
// When can it be used:
//   - Call when reading the desired state of a model before migration.
func (r *ModelRegistry) GetDraft(idOrName string) (*model.Model, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	id := r.resolveID(idOrName)
	draft, ok := r.drafts[id]
	if !ok {
		// Fallback to active if no draft exists
		active, okActive := r.active[id]
		if okActive {
			return cloneModel(active), nil
		}
		return nil, fmt.Errorf("model '%s' not found", idOrName)
	}
	return cloneModel(draft), nil
}

// GetActive returns the active, published version of a model.
//
// Purpose:
//   Retrieves the currently live, verified model definition.
//
// Where it is used:
//   - Called by CRUD operations, query planners, and dataset resolvers.
//
// When can it be used:
//   - Call when executing data queries against active tables or collections.
func (r *ModelRegistry) GetActive(idOrName string) (*model.Model, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	id := r.resolveID(idOrName)
	active, ok := r.active[id]
	if !ok {
		return nil, fmt.Errorf("active model '%s' not found", idOrName)
	}
	return cloneModel(active), nil
}

// SetStatus updates the status of a draft/active model.
//
// Purpose:
//   Mutates the lifecycle status (DRAFT, APPLYING, ACTIVE, FAILED, DEGRADED) of a model.
//
// Where it is used:
//   - Called by SchemaService during migration workflows.
//
// When can it be used:
//   - Call during migration progress tracking.
func (r *ModelRegistry) SetStatus(idOrName string, status model.ModelStatus) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	id := r.resolveID(idOrName)
	if draft, ok := r.drafts[id]; ok {
		draft.Status = status
		draft.UpdatedAt = time.Now().UTC()
	}
	if active, ok := r.active[id]; ok {
		active.Status = status
		active.UpdatedAt = time.Now().UTC()
	}
	return nil
}

// SetActive promotes a draft model to ACTIVE status upon successful database migration.
//
// Purpose:
//   Promotes a verified draft to active status, bumps version counter, and deletes the draft entry.
//
// Where it is used:
//   - Called by SchemaService.Apply upon migration success.
//
// When can it be used:
//   - Call when schema modifications are verified in the database.
func (r *ModelRegistry) SetActive(idOrName string, m *model.Model) (*model.Model, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	id := r.resolveID(idOrName)
	active := cloneModel(m)
	active.ID = id
	active.Status = model.StatusActive
	active.Version++
	active.UpdatedAt = time.Now().UTC()

	r.active[id] = active
	delete(r.drafts, id)
	r.byName[strings.ToLower(active.Name)] = id
	r.byName[strings.ToLower(active.ID)] = id
	if active.StorageName != "" {
		r.byName[strings.ToLower(active.StorageName)] = id
	}

	if cfg, ok := r.modelConfigs[id]; ok {
		cfg.Status = model.ModelConfigStatusActive
		cfg.UpdatedAt = active.UpdatedAt
	}

	return cloneModel(active), nil
}

// List returns all models (preferring active, or draft if active does not exist).
//
// Purpose:
//   Lists all registered models in the registry.
//
// Where it is used:
//   - Called by GET /api/models endpoints and validation engines.
//
// When can it be used:
//   - Call when listing all data models.
func (r *ModelRegistry) List() []*model.Model {
	r.mu.RLock()
	defer r.mu.RUnlock()

	seen := make(map[string]*model.Model)
	for id, m := range r.active {
		seen[id] = cloneModel(m)
	}
	for id, m := range r.drafts {
		if _, exists := seen[id]; !exists {
			seen[id] = cloneModel(m)
		}
	}

	result := make([]*model.Model, 0, len(seen))
	for _, m := range seen {
		result = append(result, m)
	}
	return result
}

// Delete removes a model and its draft/active instances from registry.
//
// Purpose:
//   Removes a model, its drafts, model_config, data_models, and alias entries from the registry.
//
// Where it is used:
//   - Called by DELETE /api/models/:model endpoints.
//
// When can it be used:
//   - Call when purging a model metadata definition.
func (r *ModelRegistry) Delete(idOrName string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	id := r.resolveID(idOrName)
	delete(r.active, id)
	delete(r.drafts, id)
	delete(r.modelConfigs, id)
	delete(r.dataModels, id)
	delete(r.byName, strings.ToLower(id))
	delete(r.byName, strings.ToLower(idOrName))
	return nil
}

// resolveID resolves an identifier or name into a canonical model ID.
//
// Purpose:
//   Performs case-insensitive lookup across aliases, table names, and refNames.
//
// Where it is used:
//   - Used internally by GetDraft, GetActive, SetActive, Delete, etc.
//
// When can it be used:
//   - Internal helper for ID resolution.
func (r *ModelRegistry) resolveID(idOrName string) string {
	lower := strings.ToLower(idOrName)
	if id, ok := r.byName[lower]; ok {
		return id
	}
	if _, ok := r.active[idOrName]; ok {
		return idOrName
	}
	if _, ok := r.drafts[idOrName]; ok {
		return idOrName
	}
	for id, m := range r.active {
		if strings.EqualFold(id, idOrName) || strings.EqualFold(m.Name, idOrName) || strings.EqualFold(m.StorageName, idOrName) {
			return id
		}
	}
	for id, m := range r.drafts {
		if strings.EqualFold(id, idOrName) || strings.EqualFold(m.Name, idOrName) || strings.EqualFold(m.StorageName, idOrName) {
			return id
		}
	}
	for id, cfg := range r.modelConfigs {
		if strings.EqualFold(id, idOrName) || strings.EqualFold(cfg.Name, idOrName) || strings.EqualFold(cfg.RefName, idOrName) || strings.EqualFold(cfg.Table, idOrName) || (cfg.Schema != "" && strings.EqualFold(fmt.Sprintf("%s.%s", cfg.Schema, cfg.Table), idOrName)) {
			return id
		}
	}
	return idOrName
}

// cloneModel creates a deep clone of a Model to ensure immutability outside mutex blocks.
//
// Purpose:
//   Safely duplicates attributes, indexes, relations, and metadata maps.
//
// Where it is used:
//   - Used internally by SaveDraft, GetDraft, GetActive, SetActive, and List.
//
// When can it be used:
//   - Internal cloning helper for model instances.
func cloneModel(m *model.Model) *model.Model {
	if m == nil {
		return nil
	}
	cp := *m
	cp.Attributes = make([]model.Attribute, len(m.Attributes))
	copy(cp.Attributes, m.Attributes)

	cp.Indexes = make([]model.Index, len(m.Indexes))
	copy(cp.Indexes, m.Indexes)

	cp.Relations = make([]model.Relation, len(m.Relations))
	copy(cp.Relations, m.Relations)

	if m.Metadata != nil {
		cp.Metadata = make(map[string]any, len(m.Metadata))
		for k, v := range m.Metadata {
			cp.Metadata[k] = v
		}
	}
	return &cp
}
