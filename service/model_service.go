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
func NewModelService(reg *registry.ModelRegistry) *ModelService {
	return &ModelService{
		registry: reg,
	}
}

// CreateDraft registers a new model definition draft.
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
func (s *ModelService) UpdateDraft(ctx context.Context, id string, m *model.Model) (*model.Model, error) {
	m.ID = id
	if err := validation.ValidateModel(m); err != nil {
		return nil, fmt.Errorf("model validation failed: %w", err)
	}
	return s.registry.SaveDraft(m)
}

// GetDraft retrieves a model definition draft or active version.
func (s *ModelService) GetDraft(ctx context.Context, id string) (*model.Model, error) {
	return s.registry.GetDraft(id)
}

// GetActive retrieves the active published model definition.
func (s *ModelService) GetActive(ctx context.Context, id string) (*model.Model, error) {
	return s.registry.GetActive(id)
}

// List returns all registered models.
func (s *ModelService) List(ctx context.Context) []*model.Model {
	return s.registry.List()
}

// Delete removes a model metadata definition.
func (s *ModelService) Delete(ctx context.Context, id string) error {
	return s.registry.Delete(id)
}

// CreateModelConfig registers a new model_config.
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
func (s *ModelService) GetModelConfig(ctx context.Context, idOrName string) (*model.ModelConfig, error) {
	return s.registry.GetModelConfig(idOrName)
}

// ListModelConfigs returns all model_configs.
func (s *ModelService) ListModelConfigs(ctx context.Context) []*model.ModelConfig {
	return s.registry.ListModelConfigs()
}

// UpdateModelConfig updates an existing model_config.
func (s *ModelService) UpdateModelConfig(ctx context.Context, id string, cfg *model.ModelConfig) (*model.ModelConfig, error) {
	cfg.ID = id
	if err := validation.ValidateModelConfig(cfg); err != nil {
		return nil, fmt.Errorf("model_config validation failed: %w", err)
	}
	return s.registry.SaveModelConfig(cfg)
}

// AddDataModel registers a field definition for a model.
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
func (s *ModelService) GetDataModel(ctx context.Context, modelID, fieldID string) (*model.DataModel, error) {
	return s.registry.GetDataModel(modelID, fieldID)
}

// ListDataModels returns all field definitions for a model.
func (s *ModelService) ListDataModels(ctx context.Context, modelID string) []*model.DataModel {
	return s.registry.ListDataModels(modelID)
}

// DeleteDataModel removes a field definition from a model.
func (s *ModelService) DeleteDataModel(ctx context.Context, modelID, fieldID string) error {
	return s.registry.DeleteDataModel(modelID, fieldID)
}

// Reinit rebuilds and compiles all models from ModelConfig and DataModel definitions.
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


