// Package project provides model re-initialization, runtime reloading, and live database sync capabilities.
//
// File: reinit.go
// Usage:
//   Defines ReinitOptions and functional options (WithModels, WithModelConfigs, WithDataModels, WithAutoMigrate)
//   allowing dynamic reloading, schema rebuilds, and live database sync without restarting the engine.
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
//
// Purpose:
//   Configures the reinit runner to process only the specified model identifiers.
//
// Where it is used:
//   - Passed as an option to Reinit when targeting specific models.
//
// When can it be used:
//   - Call when updating a single model or a subset of models.
func WithModels(ids ...string) ReinitOption {
	return func(o *ReinitOptions) {
		o.ModelIDs = append(o.ModelIDs, ids...)
	}
}

// WithModelConfigs provides additional or updated ModelConfigs to load before re-initialization.
//
// Purpose:
//   Injects model configs dynamically during the reinit cycle.
//
// Where it is used:
//   - Passed as an option to Reinit when loading new models into the engine.
//
// When can it be used:
//   - Call when introducing new models during runtime reload.
func WithModelConfigs(cfgs ...*model.ModelConfig) ReinitOption {
	return func(o *ReinitOptions) {
		o.ModelConfigs = append(o.ModelConfigs, cfgs...)
	}
}

// WithDataModels provides additional or updated DataModels to load before re-initialization.
//
// Purpose:
//   Injects field attribute definitions dynamically during the reinit cycle.
//
// Where it is used:
//   - Passed as an option to Reinit when updating column schemas.
//
// When can it be used:
//   - Call when altering fields on models during runtime reload.
func WithDataModels(dms ...*model.DataModel) ReinitOption {
	return func(o *ReinitOptions) {
		o.DataModels = append(o.DataModels, dms...)
	}
}

// WithAutoMigrate enables applying diff-based schema changes to the live database during reinit.
//
// Purpose:
//   Instructs the reinit runner to automatically execute live database DDL migrations for updated models.
//
// Where it is used:
//   - Passed as an option to Reinit when schema updates must be synced to the database.
//
// When can it be used:
//   - Call when you want model changes to automatically update database tables.
func WithAutoMigrate(autoMigrate bool) ReinitOption {
	return func(o *ReinitOptions) {
		o.AutoMigrate = autoMigrate
	}
}

// WithSyncFromDB enables live database schema introspection to synchronize model definitions.
//
// Purpose:
//   Instructs the reinit process to introspect live database tables and update model definitions accordingly.
//
// Where it is used:
//   - Passed as an option to Reinit when importing or refreshing database tables.
//
// When can it be used:
//   - Call when syncing engine models with external schema changes.
func WithSyncFromDB(syncFromDB bool) ReinitOption {
	return func(o *ReinitOptions) {
		o.SyncFromDB = syncFromDB
	}
}

// WithClearCache clears existing compiled execution models before rebuilding.
//
// Purpose:
//   Flushes cached query plans and compiled model artifacts prior to rebuilding.
//
// Where it is used:
//   - Passed as an option to Reinit when performing a clean rebuild.
//
// When can it be used:
//   - Call when forcing full recompilation of all engine models.
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
//
// Purpose:
//   Validates, registers, persists to metadata tables, and compiles models into memory.
//
// Where it is used:
//   - Called by NewWithModels during server startup and project bootstrapping.
//
// When can it be used:
//   - Call when loading seed models or declarative configs into an engine.
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
//
// Purpose:
//   Reconstructs and recompiles models, with options for auto-migration and live database sync.
//
// Where it is used:
//   - Called by POST /api/project/reinit endpoints, admin tools, and integration tests.
//
// When can it be used:
//   - Call whenever model definitions change and the engine must rebuild runtime structures.
func (e *Engine) Reinit(ctx context.Context, opts ...ReinitOption) (*ReinitResult, error) {
	options := &ReinitOptions{}
	for _, opt := range opts {
		opt(options)
	}

	return e.ReinitWithOptions(ctx, *options)
}

// ReinitWithOptions executes re-initialization using an options struct.
//
// Purpose:
//   Executes the full re-initialization pipeline with fine-grained configuration.
//
// Where it is used:
//   - Called by Reinit and internal workflow managers.
//
// When can it be used:
//   - Call when running re-initialization with pre-constructed ReinitOptions.
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

// rebuildAllCompiledModelsLocked rebuilds compiled execution models while holding the engine lock.
//
// Purpose:
//   Compiles ModelConfig and DataModel field definitions into runnable Model instances.
//
// Where it is used:
//   - Called internally by LoadModels and RestoreFromDB.
//
// When can it be used:
//   - Internal helper for recompiling models under lock.
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
