// Package service implements domain services for entity metadata lifecycle,
// schema synchronization, and dataset management.
//
// File: model_service.go
// Usage:
//   This file implements the ModelService, orchestrating CRUD and lifecycle state transitions
//   for entity models, model configurations (ModelConfig), data field specifications (DataModel),
//   and compiled active execution models. It enforces validation rules on drafts and provides
//   the Reinit engine to rebuild runtime models from stored metadata.
package service

import (
	"context"
	"fmt"

	"github.com/SanjayDrop5528/models-go-engine/mapping"
	"github.com/SanjayDrop5528/models-go-engine/model"
	"github.com/SanjayDrop5528/models-go-engine/registry"
	"github.com/SanjayDrop5528/models-go-engine/validation"
)

// ModelService manages model metadata definitions and life cycles.
type ModelService struct {
	registry *registry.ModelRegistry
}

// NewModelService creates a new ModelService.
//
// Purpose:
//   Initializes a ModelService wired to the central ModelRegistry.
//
// Where it is used:
//   - Instantiated in API server bootstrapping and engine initialization.
//
// When can it be used:
//   - When managing model drafts, configurations, and schema metadata.
func NewModelService(reg *registry.ModelRegistry) *ModelService {
	return &ModelService{
		registry: reg,
	}
}

// CreateDraft registers a new model definition draft.
//
// Purpose:
//   Validates and saves a new model draft, generating a UUID if not provided.
//
// Where it is used:
//   - Called by model designer APIs when drafting new entities.
//
// When can it be used:
//   - When creating a new unpublished entity definition.
func (s *ModelService) CreateDraft(ctx context.Context, m *model.Model) (*model.Model, error) {
	if m != nil && m.ID == "" {
		m.ID = mapping.GenerateUUID()
	}
	if err := validation.ValidateModel(m); err != nil {
		return nil, fmt.Errorf("model validation failed: %w", err)
	}
	return s.registry.SaveDraft(m)
}

// UpdateDraft updates an existing model draft.
//
// Purpose:
//   Validates and updates an existing draft entity model.
//
// Where it is used:
//   - Called by model editor APIs during draft modifications.
//
// When can it be used:
//   - When editing attributes or settings on a draft model.
func (s *ModelService) UpdateDraft(ctx context.Context, id string, m *model.Model) (*model.Model, error) {
	m.ID = id
	if err := validation.ValidateModel(m); err != nil {
		return nil, fmt.Errorf("model validation failed: %w", err)
	}
	return s.registry.SaveDraft(m)
}

// GetDraft retrieves a model definition draft or active version.
//
// Purpose:
//   Retrieves the latest draft or falls back to active version if no draft exists.
//
// Where it is used:
//   - Called by model designer and schema inspection endpoints.
//
// When can it be used:
//   - When loading an entity model for editing or inspection.
func (s *ModelService) GetDraft(ctx context.Context, id string) (*model.Model, error) {
	return s.registry.GetDraft(id)
}

// GetActive retrieves the active published model definition.
//
// Purpose:
//   Looks up the currently active, published version of an entity model.
//
// Where it is used:
//   - Called by execution engines, CRUD services, and validation pipelines.
//
// When can it be used:
//   - When resolving model metadata for live database operations.
func (s *ModelService) GetActive(ctx context.Context, id string) (*model.Model, error) {
	return s.registry.GetActive(id)
}

// List returns all registered models.
//
// Purpose:
//   Retrieves a slice of all registered models across drafts and active entities.
//
// Where it is used:
//   - Called by admin and navigation catalog endpoints.
//
// When can it be used:
//   - When displaying all models in the user interface.
func (s *ModelService) List(ctx context.Context) []*model.Model {
	return s.registry.List()
}

// Delete removes a model metadata definition.
//
// Purpose:
//   Evicts a model definition from the registry.
//
// Where it is used:
//   - Called by model deletion endpoints.
//
// When can it be used:
//   - When decommissioning or deleting an entity model.
func (s *ModelService) Delete(ctx context.Context, id string) error {
	return s.registry.Delete(id)
}

// CreateModelConfig registers a new model_config.
//
// Purpose:
//   Validates and saves a new model configuration entity to the registry.
//
// Where it is used:
//   - Called by model administration APIs when introducing a new model entity.
//
// When can it be used:
//   - When defining an entity before declaring its individual columns.
func (s *ModelService) CreateModelConfig(ctx context.Context, cfg *model.ModelConfig) (*model.ModelConfig, error) {
	if cfg != nil && cfg.ID == "" {
		cfg.ID = mapping.GenerateUUID()
	}
	if err := validation.ValidateModelConfig(cfg); err != nil {
		return nil, fmt.Errorf("model_config validation failed: %w", err)
	}
	return s.registry.SaveModelConfig(cfg)
}

// GetModelConfig retrieves a model_config by ID or name.
//
// Purpose:
//   Looks up a model configuration by its unique UUID or name.
//
// Where it is used:
//   - Called by schema inspector and model configuration endpoints.
//
// When can it be used:
//   - When viewing entity settings or verifying model presence.
func (s *ModelService) GetModelConfig(ctx context.Context, idOrName string) (*model.ModelConfig, error) {
	return s.registry.GetModelConfig(idOrName)
}

// ListModelConfigs returns all model_configs.
//
// Purpose:
//   Retrieves all configured model entities.
//
// Where it is used:
//   - Called by management consoles and Reinit routines.
//
// When can it be used:
//   - When displaying all available entities or synchronizing full database schemas.
func (s *ModelService) ListModelConfigs(ctx context.Context) []*model.ModelConfig {
	return s.registry.ListModelConfigs()
}

// UpdateModelConfig updates an existing model_config.
//
// Purpose:
//   Validates and updates configuration settings on an existing model.
//
// Where it is used:
//   - Called by model configuration editors.
//
// When can it be used:
//   - When modifying table name, schema, description, or status on a model.
func (s *ModelService) UpdateModelConfig(ctx context.Context, id string, cfg *model.ModelConfig) (*model.ModelConfig, error) {
	cfg.ID = id
	if err := validation.ValidateModelConfig(cfg); err != nil {
		return nil, fmt.Errorf("model_config validation failed: %w", err)
	}
	return s.registry.SaveModelConfig(cfg)
}

// AddDataModel registers a field definition for a model.
//
// Purpose:
//   Validates and stores a new column/field definition, ensuring custom types reference valid models.
//
// Where it is used:
//   - Called by column editor APIs when adding attributes to a model.
//
// When can it be used:
//   - When defining an attribute, data type, or orbital reference on an entity.
func (s *ModelService) AddDataModel(ctx context.Context, dm *model.DataModel) (*model.DataModel, error) {
	if dm != nil && dm.ID == "" {
		dm.ID = mapping.GenerateUUID()
	}
	if err := validation.ValidateDataModel(dm); err != nil {
		return nil, fmt.Errorf("data_model validation failed: %w", err)
	}
	if dm.CustomTypeID != nil && *dm.CustomTypeID != "" {
		if err := validation.ValidateCustomType(s.registry.GetModelConfig, dm); err != nil {
			return nil, err
		}
	}
	return s.registry.SaveDataModel(dm)
}

// GetDataModel retrieves a field definition by model ID and field ID.
//
// Purpose:
//   Looks up a specific attribute by its parent model ID and field ID.
//
// Where it is used:
//   - Called when inspecting or editing a specific field.
//
// When can it be used:
//   - When retrieving column settings or validation rules for an attribute.
func (s *ModelService) GetDataModel(ctx context.Context, modelID, fieldID string) (*model.DataModel, error) {
	return s.registry.GetDataModel(modelID, fieldID)
}

// ListDataModels returns all field definitions for a model.
//
// Purpose:
//   Retrieves all configured columns for a given model.
//
// Where it is used:
//   - Called during Reinit compilation and UI schema exploration.
//
// When can it be used:
//   - When listing the attributes of a table.
func (s *ModelService) ListDataModels(ctx context.Context, modelID string) []*model.DataModel {
	return s.registry.ListDataModels(modelID)
}

// DeleteDataModel removes a field definition from a model.
//
// Purpose:
//   Deletes an attribute definition from the registry.
//
// Where it is used:
//   - Called when deleting an attribute or dropping a column from an entity.
//
// When can it be used:
//   - When modifying model schemas to remove fields.
func (s *ModelService) DeleteDataModel(ctx context.Context, modelID, fieldID string) error {
	return s.registry.DeleteDataModel(modelID, fieldID)
}

// Reinit rebuilds and compiles all models from ModelConfig and DataModel definitions.
//
// Purpose:
//   Compiles persistent ModelConfig and DataModel metadata into executable Model instances,
//   validating models and promoting active entities into the active registry cache.
//
// Where it is used:
//   - Called during server startup, metadata synchronization, and schema reloading.
//
// When can it be used:
//   - Whenever metadata catalog records have been imported or modified and runtime models must be rebuilt.
func (s *ModelService) Reinit(ctx context.Context, modelIDs ...string) ([]string, error) {
	configs := s.registry.ListModelConfigs()
	targetMap := make(map[string]bool)
	if len(modelIDs) > 0 {
		for _, id := range modelIDs {
			targetMap[id] = true
		}
	} else {
		for _, c := range configs {
			targetMap[c.ID] = true
		}
	}

	reinitialized := make([]string, 0)
	for _, cfg := range configs {
		if !targetMap[cfg.ID] {
			continue
		}
		fields := s.registry.ListDataModels(cfg.ID)
		execModel := model.BuildModel(cfg, fields, "default_db", model.StorageRelational)
		if err := validation.ValidateModel(execModel); err != nil {
			return nil, fmt.Errorf("model validation failed during reinit for '%s': %w", cfg.ID, err)
		}
		_, _ = s.registry.SaveDraft(execModel)
		if cfg.Status == model.ModelConfigStatusActive {
			_, _ = s.registry.SetActive(cfg.ID, execModel)
		}
		reinitialized = append(reinitialized, cfg.ID)
	}
	return reinitialized, nil
}


