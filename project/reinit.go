package project

import (
	"context"
	"fmt"
	"github.com/SanjayDrop5528/models-go-engine/model"
	"github.com/SanjayDrop5528/models-go-engine/service"
	"github.com/SanjayDrop5528/models-go-engine/validation"
	"strings"
)

// ReinitOptions configures how models and data_models are re-initialized in the engine.
type ReinitOptions struct {
	ModelIDs     []string             `json:"model_ids,omitempty"`     // Specific model IDs to re-init. If empty, re-inits all.
	ModelConfigs []*model.ModelConfig `json:"model_configs,omitempty"` // New or updated ModelConfigs to register.
	DataModels   []*model.DataModel   `json:"data_models,omitempty"`   // New or updated DataModels to register.
	AutoMigrate  bool                 `json:"auto_migrate,omitempty"`  // If true, applies diff-based schema migrations.
	SyncFromDB   bool                 `json:"sync_from_db,omitempty"`  // If true, introspects live database schema to sync models.
	ClearCache   bool                 `json:"clear_cache,omitempty"`   // If true, clears cached compilation before rebuilding.
}

// ReinitOption is a functional configuration option for Reinit.
type ReinitOption func(*ReinitOptions)

// WithModels limits re-initialization to specific model IDs.
func WithModels(ids ...string) ReinitOption {
	return func(o *ReinitOptions) {
		o.ModelIDs = append(o.ModelIDs, ids...)
	}
}

// WithModelConfigs provides additional or updated ModelConfigs to load before re-initialization.
func WithModelConfigs(cfgs ...*model.ModelConfig) ReinitOption {
	return func(o *ReinitOptions) {
		o.ModelConfigs = append(o.ModelConfigs, cfgs...)
	}
}

// WithDataModels provides additional or updated DataModels to load before re-initialization.
func WithDataModels(dms ...*model.DataModel) ReinitOption {
	return func(o *ReinitOptions) {
		o.DataModels = append(o.DataModels, dms...)
	}
}

// WithAutoMigrate enables applying diff-based schema changes to the live database during reinit.
func WithAutoMigrate(autoMigrate bool) ReinitOption {
	return func(o *ReinitOptions) {
		o.AutoMigrate = autoMigrate
	}
}

// WithSyncFromDB enables live database schema introspection to synchronize model definitions.
func WithSyncFromDB(syncFromDB bool) ReinitOption {
	return func(o *ReinitOptions) {
		o.SyncFromDB = syncFromDB
	}
}

// WithClearCache clears existing compiled execution models before rebuilding.
func WithClearCache(clearCache bool) ReinitOption {
	return func(o *ReinitOptions) {
		o.ClearCache = clearCache
	}
}

// ReinitResult summarizes the outcome of model re-initialization.
type ReinitResult struct {
	Status          string            `json:"status"`
	TotalModels     int               `json:"total_models"`
	ReinitModels    []string          `json:"reinit_models"`
	AppliedSchemas  []string          `json:"applied_schemas,omitempty"`
	SyncedFromDB    []string          `json:"synced_from_db,omitempty"`
	FieldCounts     map[string]int    `json:"field_counts"`
	ValidationRules map[string]string `json:"validation_rules"`
	Message         string            `json:"message"`
}

// LoadModels loads initial ModelConfigs and DataModels into the engine at startup.
func (e *Engine) LoadModels(ctx context.Context, configs []*model.ModelConfig, dataModels []*model.DataModel) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	for _, cfg := range configs {
		if cfg == nil {
			continue
		}
		if err := validation.ValidateModelConfig(cfg); err != nil {
			return fmt.Errorf("invalid model_config '%s': %w", cfg.ID, err)
		}
		saved, err := e.registry.SaveModelConfig(cfg)
		if err != nil {
			return err
		}
		if e.adapter != nil {
			if m, err := modelConfigToMap(saved); err == nil {
				_, _ = e.adapter.Create(ctx, model.ModelRef{StorageName: "model_configs", Name: "model_configs"}, m)
			}
		}
	}

	for _, dm := range dataModels {
		if dm == nil {
			continue
		}
		if err := validation.ValidateDataModel(dm); err != nil {
			return fmt.Errorf("invalid data_model '%s.%s': %w", dm.ModelID, dm.ColumnName, err)
		}
		if dm.CustomTypeID != nil && *dm.CustomTypeID != "" {
			if err := validation.ValidateCustomType(func(idOrName string) (*model.ModelConfig, error) {
				return e.registry.GetModelConfig(idOrName)
			}, dm); err != nil {
				return err
			}
		}
		saved, err := e.registry.SaveDataModel(dm)
		if err != nil {
			return err
		}
		if e.adapter != nil {
			if m, err := dataModelToMap(saved); err == nil {
				_, _ = e.adapter.Create(ctx, model.ModelRef{StorageName: "data_models", Name: "data_models"}, m)
			}
		}
	}

	// Compile all execution models
	return e.rebuildAllCompiledModelsLocked(ctx)
}

// Reinit re-initializes all or selected models and data_models in the engine.
// If called with no options, it completely reconstructs all registered models from their metadata.
func (e *Engine) Reinit(ctx context.Context, opts ...ReinitOption) (*ReinitResult, error) {
	options := &ReinitOptions{}
	for _, opt := range opts {
		opt(options)
	}

	return e.ReinitWithOptions(ctx, *options)
}

// ReinitWithOptions executes re-initialization using an options struct.
func (e *Engine) ReinitWithOptions(ctx context.Context, options ReinitOptions) (*ReinitResult, error) {
	// 1. Ingest any newly supplied ModelConfigs
	for _, cfg := range options.ModelConfigs {
		if cfg == nil {
			continue
		}
		if _, err := e.CreateModelConfig(ctx, cfg); err != nil {
			return nil, err
		}
	}

	// 2. Ingest any newly supplied DataModels
	for _, dm := range options.DataModels {
		if dm == nil {
			continue
		}
		if _, err := e.AddDataModel(ctx, dm); err != nil {
			return nil, err
		}
	}

	// 3. Determine target models
	allConfigs := e.registry.ListModelConfigs()
	targetMap := make(map[string]bool)
	if len(options.ModelIDs) > 0 {
		for _, id := range options.ModelIDs {
			targetMap[strings.TrimSpace(id)] = true
		}
	} else {
		for _, cfg := range allConfigs {
			targetMap[cfg.ID] = true
		}
	}

	result := &ReinitResult{
		Status:          "SUCCESS",
		TotalModels:     len(allConfigs),
		ReinitModels:    make([]string, 0),
		AppliedSchemas:  make([]string, 0),
		SyncedFromDB:    make([]string, 0),
		FieldCounts:     make(map[string]int),
		ValidationRules: make(map[string]string),
	}

	storageDatabase := ""
	if e.project != nil {
		storageDatabase = e.project.AdapterConfig.Database
	}

	// 4. Reconstruct, compile, and validate each target model
	for _, cfg := range allConfigs {
		if !targetMap[cfg.ID] {
			continue
		}

		// Optional: Sync from live database
		if options.SyncFromDB && !cfg.IsAttributeReference {
			syncedModel, err := e.SyncSchema(ctx, cfg.ID)
			if err == nil && syncedModel != nil {
				result.SyncedFromDB = append(result.SyncedFromDB, cfg.ID)
			}
		}

		fields := e.registry.ListDataModels(cfg.ID)
		result.FieldCounts[cfg.ID] = len(fields)

		// Compile into executable Model
		execModel := model.BuildModel(cfg, fields, storageDatabase, model.StorageRelational)
		if err := validation.ValidateModel(execModel); err != nil {
			return nil, fmt.Errorf("compiled model validation failed for '%s': %w", cfg.ID, err)
		}

		_, _ = e.registry.SaveDraft(execModel)
		if cfg.Status == model.ModelConfigStatusActive {
			_, _ = e.registry.SetActive(cfg.ID, execModel)
		}

		result.ReinitModels = append(result.ReinitModels, cfg.ID)
		result.ValidationRules[cfg.ID] = fmt.Sprintf("%d fields, active_status=%s, ref_type=%t", len(fields), cfg.Status, cfg.IsAttributeReference)

		// Optional: AutoMigrate to DB
		if options.AutoMigrate && !cfg.IsAttributeReference {
			_, err := e.ApplySchema(ctx, cfg.ID, service.ApplyRequest{})
			if err == nil {
				result.AppliedSchemas = append(result.AppliedSchemas, cfg.ID)
			}
		}
	}

	result.Message = fmt.Sprintf("Successfully re-initialized %d models in engine.", len(result.ReinitModels))
	return result, nil
}

func (e *Engine) rebuildAllCompiledModelsLocked(ctx context.Context) error {
	configs := e.registry.ListModelConfigs()
	storageDatabase := ""
	if e.project != nil {
		storageDatabase = e.project.AdapterConfig.Database
	}

	for _, cfg := range configs {
		fields := e.registry.ListDataModels(cfg.ID)
		execModel := model.BuildModel(cfg, fields, storageDatabase, model.StorageRelational)
		if err := validation.ValidateModel(execModel); err != nil {
			return fmt.Errorf("model '%s' compilation validation failed: %w", cfg.ID, err)
		}
		_, _ = e.registry.SaveDraft(execModel)
		if cfg.Status == model.ModelConfigStatusActive {
			_, _ = e.registry.SetActive(cfg.ID, execModel)
		}
	}
	return nil
}
