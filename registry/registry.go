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

	if _, ok := r.dataModels[cp.ModelID]; !ok {
		r.dataModels[cp.ModelID] = make(map[string]*model.DataModel)
	}
	r.dataModels[cp.ModelID][cp.ID] = &cp
	return &cp, nil
}

// GetDataModel retrieves a data_model field definition by model ID and field ID (or column_name/json_field).
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
		if strings.EqualFold(id, idOrName) || strings.EqualFold(cfg.Name, idOrName) || strings.EqualFold(cfg.RefName, idOrName) {
			return id
		}
	}
	return idOrName
}

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
