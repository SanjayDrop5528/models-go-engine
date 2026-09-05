package project

import (
	"context"
	"errors"
	"fmt"
	"log"
	"github.com/SanjayDrop5528/models-go-engine/adapter"
	datasetrepo "github.com/SanjayDrop5528/models-go-engine/dataset/repository"
	datasetres "github.com/SanjayDrop5528/models-go-engine/dataset/resolver"
	datasetsvc "github.com/SanjayDrop5528/models-go-engine/dataset/service"
	"github.com/SanjayDrop5528/models-go-engine/diff"
	"github.com/SanjayDrop5528/models-go-engine/execution"
	"github.com/SanjayDrop5528/models-go-engine/mapping"
	"github.com/SanjayDrop5528/models-go-engine/model"
	"github.com/SanjayDrop5528/models-go-engine/operation"
	"github.com/SanjayDrop5528/models-go-engine/plan"
	"github.com/SanjayDrop5528/models-go-engine/query"
	"github.com/SanjayDrop5528/models-go-engine/registry"
	"github.com/SanjayDrop5528/models-go-engine/schema"
	"github.com/SanjayDrop5528/models-go-engine/service"
	"github.com/SanjayDrop5528/models-go-engine/validation"
	"strings"
	"sync"
)

// Engine is the base project engine orchestrating model metadata, schema lifecycle,
// dynamic data CRUD operations, custom types, and orbital reference validation.
type Engine struct {
	mu               sync.RWMutex
	project          *Project
	adapter          adapter.Adapter
	registry         *registry.ModelRegistry
	diffEngine       *diff.DiffEngine
	dataSetRepo      datasetrepo.DataSetRepository
	functionRegistry *datasetres.InMemFunctionRegistry
	dataSetService   *datasetsvc.DataSetService
}

// NewEngine creates a new base Engine for a Project.
func NewEngine(proj *Project, adp adapter.Adapter) *Engine {
	dsRepo := datasetrepo.NewAdapterDataSetRepository(adp)
	fnReg := datasetres.NewFunctionRegistry()
	modelResolver := datasetres.NewModelResolver(nil)
	dsSvc := datasetsvc.NewDataSetService(dsRepo, modelResolver, modelResolver, fnReg, adp)

	return &Engine{
		project:          proj,
		adapter:          adp,
		registry:         registry.NewModelRegistry(),
		diffEngine:       diff.NewDiffEngine(),
		dataSetRepo:      dsRepo,
		functionRegistry: fnReg,
		dataSetService:   dsSvc,
	}
}

// GetDataSetService returns the dataset engine service instance.
func (e *Engine) GetDataSetService() *datasetsvc.DataSetService {
	return e.dataSetService
}

// GetDataSetRepository returns the dataset repository.
func (e *Engine) GetDataSetRepository() datasetrepo.DataSetRepository {
	return e.dataSetRepo
}

// GetFunctionRegistry returns the function registry for datasets.
func (e *Engine) GetFunctionRegistry() *datasetres.InMemFunctionRegistry {
	return e.functionRegistry
}

// GetProject returns the parent Project metadata.
func (e *Engine) GetProject() *Project {
	return e.project
}

// GetAdapter returns the configured database adapter.
func (e *Engine) GetAdapter() adapter.Adapter {
	return e.adapter
}

// GetDatabaseName returns the target database name of the connected adapter.
func (e *Engine) GetDatabaseName() string {
	if e.adapter == nil {
		return "in-memory"
	}
	return e.adapter.DatabaseName()
}

// NativeClient returns the underlying native database handle (*sql.DB, *mongo.Client, etc.).
func (e *Engine) NativeClient() any {
	if e.adapter == nil {
		return nil
	}
	return e.adapter.NativeClient()
}

// GetNativeClient returns the underlying native database handle (*sql.DB, *mongo.Client, etc.).
func (e *Engine) GetNativeClient() any {
	return e.NativeClient()
}

// GetRegistry returns the underlying model registry.
func (e *Engine) GetRegistry() *registry.ModelRegistry {
	return e.registry
}

// EnsureMetadataTables ensures system metadata tables ('model_configs' and 'data_models') exist in the adapter.
func (e *Engine) EnsureMetadataTables(ctx context.Context) error {
	if e.adapter == nil {
		return errors.New("no active adapter configured")
	}
	return e.adapter.EnsureMetadataTables(ctx)
}

// ImportLiveMetadata delegates live database schema introspection to the adapter and populates registry and metadata tables.
func (e *Engine) ImportLiveMetadata(ctx context.Context) (map[string]any, error) {
	log.Println("[Schema Engine] [Import Start] Initiating live database schema import & registry synchronization...")
	if e.adapter == nil {
		err := errors.New("no active database adapter configured in engine")
		log.Printf("[Schema Engine] ✖ [Import Error] %v", err)
		return nil, err
	}

	configs, fields, err := e.adapter.ImportLiveMetadata(ctx)
	if err != nil {
		log.Printf("[Schema Engine] ✖ [Import Error] Adapter introspection failed: %v", err)
		return nil, fmt.Errorf("live schema introspection failed: %w", err)
	}

	importedModels := make([]string, 0, len(configs))
	for _, cfg := range configs {
		existing, _ := e.GetModelConfig(ctx, cfg.ID)
		if existing == nil {
			if _, err := e.CreateModelConfig(ctx, cfg); err != nil {
				log.Printf("[Schema Engine] ⚠ [Import Warning] Failed storing ModelConfig '%s': %v", cfg.ID, err)
			} else {
				log.Printf("[Schema Engine] [Import Registry] Created ModelConfig '%s' (Table: '%s', Schema: '%s')", cfg.ID, cfg.Table, cfg.Schema)
			}
		} else {
			if _, err := e.UpdateModelConfig(ctx, cfg.ID, cfg); err != nil {
				log.Printf("[Schema Engine] ⚠ [Import Warning] Failed updating ModelConfig '%s': %v", cfg.ID, err)
			} else {
				log.Printf("[Schema Engine] [Import Registry] Updated ModelConfig '%s' (Table: '%s', Schema: '%s')", cfg.ID, cfg.Table, cfg.Schema)
			}
		}
		importedModels = append(importedModels, cfg.ID)
	}

	fieldSuccessCount := 0
	for _, f := range fields {
		if _, err := e.AddDataModel(ctx, f); err != nil {
			log.Printf("[Schema Engine] ⚠ [Import Warning] Failed storing DataModel field '%s.%s': %v", f.ModelID, f.ColumnName, err)
		} else {
			fieldSuccessCount++
		}
	}

	// Rebuild compiled execution models and activate them in the registry
	dbName := ""
	if e.project != nil {
		dbName = e.project.AdapterConfig.Database
	}
	if dbName == "" && e.adapter != nil {
		dbName = e.adapter.Name()
	}

	activeCount := 0
	for _, cfg := range configs {
		allFields := e.registry.ListDataModels(cfg.ID)
		execModel := model.BuildModel(cfg, allFields, dbName, model.StorageRelational)
		if execModel != nil {
			_, _ = e.registry.SaveDraft(execModel)
			if cfg.Status == model.ModelConfigStatusActive {
				if _, err := e.registry.SetActive(cfg.ID, execModel); err == nil {
					activeCount++
				}
			}
		}
	}

	summaryMsg := fmt.Sprintf("Successfully imported & activated %d models (%v) and %d fields from live database via adapter!", activeCount, importedModels, fieldSuccessCount)
	log.Printf("[Schema Engine] ✔ [Import Success] %s", summaryMsg)

	return map[string]any{
		"status":          "SUCCESS",
		"message":         summaryMsg,
		"imported_tables": importedModels,
		"imported_fields": fieldSuccessCount,
		"active_models":   activeCount,
		"total_configs":   len(configs),
	}, nil
}

// =========================================================================
// 1. Model Configuration Management
// =========================================================================

// CreateModelConfig validates and stores a new ModelConfig.
func (e *Engine) CreateModelConfig(ctx context.Context, cfg *model.ModelConfig) (*model.ModelConfig, error) {
	if cfg == nil {
		return nil, errors.New("model_config cannot be nil")
	}
	if cfg.ID == "" {
		cfg.ID = mapping.GenerateUUID()
	}
	if err := validation.ValidateModelConfig(cfg); err != nil {
		return nil, fmt.Errorf("model_config validation failed: %w", err)
	}

	saved, err := e.registry.SaveModelConfig(cfg)
	if err != nil {
		return nil, err
	}

	// Persist metadata to database collection/table 'model_configs'
	if e.adapter != nil {
		if m, err := modelConfigToMap(saved); err == nil {
			if _, err := e.adapter.Create(ctx, model.ModelRef{StorageName: "model_configs", Name: "model_configs"}, m); err != nil {
				_, _ = e.adapter.Update(ctx, model.ModelRef{StorageName: "model_configs", Name: "model_configs"}, saved.ID, m)
			}
		}
	}

	// Rebuild and save draft model if fields exist
	fields := e.registry.ListDataModels(saved.ID)
	execModel := model.BuildModel(saved, fields, e.project.AdapterConfig.Database, model.StorageRelational)
	_, _ = e.registry.SaveDraft(execModel)

	return saved, nil
}

// UpdateModelConfig updates an existing ModelConfig.
func (e *Engine) UpdateModelConfig(ctx context.Context, id string, cfg *model.ModelConfig) (*model.ModelConfig, error) {
	cfg.ID = id
	if err := validation.ValidateModelConfig(cfg); err != nil {
		return nil, fmt.Errorf("model_config validation failed: %w", err)
	}

	saved, err := e.registry.SaveModelConfig(cfg)
	if err != nil {
		return nil, err
	}

	// Persist metadata update to database collection/table 'model_configs'
	if e.adapter != nil {
		if m, err := modelConfigToMap(saved); err == nil {
			_, _ = e.adapter.Update(ctx, model.ModelRef{StorageName: "model_configs", Name: "model_configs"}, id, m)
		}
	}

	fields := e.registry.ListDataModels(saved.ID)
	execModel := model.BuildModel(saved, fields, e.project.AdapterConfig.Database, model.StorageRelational)
	_, _ = e.registry.SaveDraft(execModel)

	return saved, nil
}

// GetModelConfig retrieves a ModelConfig by ID or Name.
func (e *Engine) GetModelConfig(ctx context.Context, idOrName string) (*model.ModelConfig, error) {
	return e.registry.GetModelConfig(idOrName)
}

// ListModelConfigs returns all ModelConfigs in the project.
// Priority: DB metadata table → in-memory registry (after live import).
// If DB has rows, they are synced into the registry and returned.
// If DB is empty but registry has models (e.g. from a live schema import), those are returned directly.
func (e *Engine) ListModelConfigs(ctx context.Context) []*model.ModelConfig {
	if e.adapter != nil {
		mcRef := model.ModelRef{StorageName: "model_configs", Name: "model_configs"}
		rows, _, err := e.adapter.Find(ctx, mcRef, query.NewQuery().LimitOffset(10000, 0))
		if err == nil && len(rows) > 0 {
			var configs []*model.ModelConfig
			for _, row := range rows {
				cfg, err := mapToModelConfig(row)
				if err == nil && cfg != nil && cfg.ID != "" {
					configs = append(configs, cfg)
					_, _ = e.registry.SaveModelConfig(cfg)
				}
			}
			if len(configs) > 0 {
				log.Printf("[Engine] Loaded %d model_config(s) from database table 'model_configs' via %s adapter", len(configs), e.adapter.Name())
				return configs
			}
		}
	}
	// DB table is empty or adapter unavailable — return from in-memory registry.
	// This covers models loaded via live-import (ImportLiveMetadata) which populate
	// the registry but do NOT write to the model_configs metadata table.
	all := e.registry.ListModelConfigs()
	if len(all) > 0 {
		log.Printf("[Engine] Serving %d model_config(s) from in-memory registry (populated via live import)", len(all))
	} else {
		log.Println("[Engine] No model_configs found in database table or in-memory registry")
	}
	return all
}


// =========================================================================
// 2. DataModel Field Management & Custom Type Checks
// =========================================================================

// AddDataModel validates and adds a field definition to a model.
func (e *Engine) AddDataModel(ctx context.Context, dm *model.DataModel) (*model.DataModel, error) {
	if dm != nil && dm.ID == "" {
		dm.ID = mapping.GenerateUUID()
	}
	if err := validation.ValidateDataModel(dm); err != nil {
		return nil, fmt.Errorf("data_model validation failed: %w", err)
	}

	// Verify custom_type_id if provided
	if dm.CustomTypeID != nil && *dm.CustomTypeID != "" {
		err := validation.ValidateCustomType(func(idOrName string) (*model.ModelConfig, error) {
			return e.registry.GetModelConfig(idOrName)
		}, dm)
		if err != nil {
			return nil, err
		}
	}

	saved, err := e.registry.SaveDataModel(dm)
	if err != nil {
		return nil, err
	}

	// Persist metadata to database collection/table 'data_models'
	if e.adapter != nil {
		if m, err := dataModelToMap(saved); err == nil {
			if _, err := e.adapter.Create(ctx, model.ModelRef{StorageName: "data_models", Name: "data_models"}, m); err != nil {
				_, _ = e.adapter.Update(ctx, model.ModelRef{StorageName: "data_models", Name: "data_models"}, saved.ID, m)
			}
		}
	}

	// Rebuild and save draft model if model_config exists
	if cfg, err := e.registry.GetModelConfig(saved.ModelID); err == nil && cfg != nil {
		fields := e.registry.ListDataModels(cfg.ID)
		dbName := ""
		if e.project != nil {
			dbName = e.project.AdapterConfig.Database
		}
		execModel := model.BuildModel(cfg, fields, dbName, model.StorageRelational)
		_, _ = e.registry.SaveDraft(execModel)
	}

	return saved, nil
}

// CreateDataModel validates and adds a field definition to a model (alias for AddDataModel).
func (e *Engine) CreateDataModel(ctx context.Context, dm *model.DataModel) (*model.DataModel, error) {
	return e.AddDataModel(ctx, dm)
}

// GetDataModel retrieves a data_model field definition.
func (e *Engine) GetDataModel(ctx context.Context, modelID, fieldID string) (*model.DataModel, error) {
	return e.registry.GetDataModel(modelID, fieldID)
}

// ListDataModels returns all field definitions for a model.
func (e *Engine) ListDataModels(ctx context.Context, modelID string) []*model.DataModel {
	return e.registry.ListDataModels(modelID)
}

// DeleteDataModel removes a field definition from a model.
func (e *Engine) DeleteDataModel(ctx context.Context, modelID, fieldID string) error {
	dm, _ := e.GetDataModel(ctx, modelID, fieldID)
	actualID := fieldID
	if dm != nil && dm.ID != "" {
		actualID = dm.ID
	}

	if e.adapter != nil {
		_ = e.adapter.Delete(ctx, model.ModelRef{StorageName: "data_models", Name: "data_models"}, actualID)
	}

	if err := e.registry.DeleteDataModel(modelID, fieldID); err != nil {
		return err
	}

	// Rebuild and save draft model so schema diff accurately detects field removal
	if cfg, err := e.registry.GetModelConfig(modelID); err == nil && cfg != nil {
		fields := e.registry.ListDataModels(cfg.ID)
		dbName := ""
		if e.project != nil {
			dbName = e.project.AdapterConfig.Database
		}
		execModel := model.BuildModel(cfg, fields, dbName, model.StorageRelational)
		_, _ = e.registry.SaveDraft(execModel)
	}

	return nil
}

// DeleteModelConfig removes a model_config and its compiled model from registry.
func (e *Engine) DeleteModelConfig(ctx context.Context, idOrName string) error {
	if e.adapter != nil {
		_ = e.adapter.Delete(ctx, model.ModelRef{StorageName: "model_configs", Name: "model_configs"}, idOrName)
	}
	return e.registry.Delete(idOrName)
}

// RestoreFromDB queries the database for stored model_configs and data_models,
// reconstructs the in-memory models, and activates them.
func (e *Engine) RestoreFromDB(ctx context.Context) error {
	if e.adapter == nil {
		log.Println("[RestoreFromDB] Skipping restore: No database adapter attached.")
		return nil
	}

	dbName := ""
	if provider, ok := e.adapter.(interface{ GetDatabaseName() string }); ok {
		dbName = provider.GetDatabaseName()
	}
	if dbName == "" && e.project != nil {
		dbName = e.project.AdapterConfig.Database
	}
	if dbName == "" {
		dbName = e.adapter.Name()
	}

	log.Printf("[RestoreFromDB] Restoring models from database adapter '%s' (Database: '%s')...", e.adapter.Name(), dbName)

	// 1. Fetch model_configs directly from database
	mcRef := model.ModelRef{StorageName: "model_configs", Name: "model_configs"}
	mcRows, _, err := e.adapter.Find(ctx, mcRef, query.NewQuery().LimitOffset(10000, 0))
	if err != nil {
		log.Printf("[RestoreFromDB] ⚠ Could not fetch 'model_configs' table from database '%s': %v", dbName, err)
		return err
	}
	if len(mcRows) == 0 {
		log.Printf("[RestoreFromDB] Info: 'model_configs' table in database '%s' is empty. No stored models to restore.", dbName)
		return nil
	}

	var configs []*model.ModelConfig
	for _, row := range mcRows {
		cfg, err := mapToModelConfig(row)
		if err == nil && cfg != nil && cfg.ID != "" {
			configs = append(configs, cfg)
			_, _ = e.registry.SaveModelConfig(cfg)
		}
	}

	// 2. Fetch data_models directly from database
	dmRef := model.ModelRef{StorageName: "data_models", Name: "data_models"}
	dmRows, _, err := e.adapter.Find(ctx, dmRef, query.NewQuery().LimitOffset(10000, 0))
	if err == nil && len(dmRows) > 0 {
		for _, row := range dmRows {
			dm, err := mapToDataModel(row)
			if err == nil && dm != nil && dm.ID != "" {
				_, _ = e.registry.SaveDataModel(dm)
			}
		}
	}

	// 3. Rebuild execution models and promote active models in registry
	activeCount := 0
	for _, cfg := range configs {
		fields := e.registry.ListDataModels(cfg.ID)
		execModel := model.BuildModel(cfg, fields, dbName, model.StorageRelational)
		if execModel != nil {
			_, _ = e.registry.SaveDraft(execModel)
			if cfg.Status == model.ModelConfigStatusActive {
				if _, err := e.registry.SetActive(cfg.ID, execModel); err == nil {
					activeCount++
				}
			}
		}
	}

	log.Printf("[RestoreFromDB] ✔ Restored %d ModelConfig(s) (%d active) and %d DataModel field(s) from database '%s'!", len(configs), activeCount, len(dmRows), dbName)
	return nil
}

// =========================================================================
// 3. Schema Operations & Safe Diff-Based Migrations
// =========================================================================

// GetSchema returns the live database schema for the model.
func (e *Engine) GetSchema(ctx context.Context, modelID string) (*schema.Schema, error) {
	m, err := e.getOrBuildDraftModel(modelID)
	if err != nil {
		return nil, err
	}
	return e.adapter.GetSchema(ctx, m.Ref())
}

// GetDiff computes the diff between live database schema and target model definition.
func (e *Engine) GetDiff(ctx context.Context, modelID string, hints diff.DiffHints) (*diff.SchemaDiff, error) {
	desiredModel, err := e.getOrBuildDraftModel(modelID)
	if err != nil {
		return nil, err
	}

	currentSchema, err := e.adapter.GetSchema(ctx, desiredModel.Ref())
	if err != nil {
		return nil, fmt.Errorf("failed to introspect database schema: %w", err)
	}

	desiredSchema := schema.FromModel(desiredModel)
	return e.diffEngine.Compare(currentSchema, desiredSchema, hints)
}

// PreviewSchema computes the migration diff and generates SQL/DDL preview statements.
func (e *Engine) PreviewSchema(ctx context.Context, modelID string, hints diff.DiffHints) (*plan.SchemaPreview, error) {
	desiredModel, err := e.getOrBuildDraftModel(modelID)
	if err != nil {
		return nil, err
	}

	diffResult, err := e.GetDiff(ctx, modelID, hints)
	if err != nil {
		return nil, err
	}

	schemaPlan := plan.BuildPlan(desiredModel.ID, desiredModel.StorageName, desiredModel.Database, diffResult)
	return e.adapter.PreviewSchemaChange(ctx, schemaPlan)
}

// ApplySchema executes the complete safe migration flow against the project's adapter.
func (e *Engine) ApplySchema(ctx context.Context, modelID string, req service.ApplyRequest) (*service.ApplyResult, error) {
	desiredModel, err := e.getOrBuildDraftModel(modelID)
	if err != nil {
		return nil, err
	}

	_ = e.registry.SetStatus(modelID, model.StatusApplying)

	currentSchema, err := e.adapter.GetSchema(ctx, desiredModel.Ref())
	if err != nil {
		_ = e.registry.SetStatus(modelID, model.StatusFailed)
		return nil, fmt.Errorf("failed to re-introspect database schema before apply: %w", err)
	}

	desiredSchema := schema.FromModel(desiredModel)
	freshDiff, err := e.diffEngine.Compare(currentSchema, desiredSchema, req.Hints)
	if err != nil {
		_ = e.registry.SetStatus(modelID, model.StatusFailed)
		return nil, fmt.Errorf("failed to compute fresh schema diff: %w", err)
	}

	if !freshDiff.HasChanges {
		activeModel, _ := e.registry.SetActive(modelID, desiredModel)
		return &service.ApplyResult{
			ModelID:        modelID,
			StorageName:    desiredModel.StorageName,
			Database:       desiredModel.Database,
			AppliedChanges: nil,
			VerifiedSchema: currentSchema,
			Status:         activeModel.Status,
			Message:        "Schema is already up to date. No changes applied.",
		}, nil
	}

	freshPlan := plan.BuildPlan(desiredModel.ID, desiredModel.StorageName, desiredModel.Database, freshDiff)
	if err := validation.ValidatePlan(freshPlan, req.AllowDestructive); err != nil {
		_ = e.registry.SetStatus(modelID, model.StatusDraft)
		return nil, err
	}

	if err := e.adapter.ApplySchemaChange(ctx, freshPlan); err != nil {
		_ = e.registry.SetStatus(modelID, model.StatusFailed)
		return nil, fmt.Errorf("adapter failed to apply schema changes: %w", err)
	}

	verifiedSchema, err := e.adapter.GetSchema(ctx, desiredModel.Ref())
	if err != nil {
		_ = e.registry.SetStatus(modelID, model.StatusDegraded)
		return nil, fmt.Errorf("schema applied but failed post-migration verification: %w", err)
	}

	activeModel, err := e.registry.SetActive(modelID, desiredModel)
	if err != nil {
		return nil, fmt.Errorf("failed to activate model metadata: %w", err)
	}

	// Update ModelConfig status to active
	if cfg, err := e.registry.GetModelConfig(modelID); err == nil {
		cfg.Status = model.ModelConfigStatusActive
		_, _ = e.registry.SaveModelConfig(cfg)
	}

	return &service.ApplyResult{
		ModelID:        modelID,
		StorageName:    desiredModel.StorageName,
		Database:       desiredModel.Database,
		AppliedChanges: freshPlan.Operations,
		VerifiedSchema: verifiedSchema,
		Status:         activeModel.Status,
		Message:        fmt.Sprintf("Successfully applied %d schema change(s)", len(freshPlan.Operations)),
	}, nil
}

// SyncSchema introspects the database and synchronizes the local model definition.
func (e *Engine) SyncSchema(ctx context.Context, modelID string) (*model.Model, error) {
	m, err := e.getOrBuildDraftModel(modelID)
	if err != nil {
		return nil, err
	}

	liveSchema, err := e.adapter.GetSchema(ctx, m.Ref())
	if err != nil {
		return nil, fmt.Errorf("failed to introspect database: %w", err)
	}
	if liveSchema == nil || len(liveSchema.Attributes) == 0 {
		return nil, fmt.Errorf("table/collection '%s' does not exist in database '%s'", m.StorageName, m.Database)
	}

	newAttrs := make([]model.Attribute, 0, len(liveSchema.Attributes))
	for _, sa := range liveSchema.Attributes {
		newAttrs = append(newAttrs, model.Attribute{
			Name:          sa.Name,
			Type:          sa.Type,
			Length:        sa.Length,
			Precision:     sa.Precision,
			Scale:         sa.Scale,
			Nullable:      sa.Nullable,
			Default:       sa.Default,
			Unique:        sa.Unique,
			AutoIncrement: sa.AutoIncrement,
			Comment:       sa.Comment,
		})
	}
	m.Attributes = newAttrs
	if liveSchema.PrimaryKey != nil {
		m.PrimaryKey = &model.PrimaryKey{
			Name:    liveSchema.PrimaryKey.Name,
			Columns: liveSchema.PrimaryKey.Columns,
		}
	}
	return e.registry.SetActive(modelID, m)
}

// =========================================================================
// 4. Dynamic Data Operations & Orbital Reference Validation
// =========================================================================

// Create validates, verifies orbital references, coerces, and inserts a record.
func (e *Engine) Create(ctx context.Context, modelID string, data map[string]any) (map[string]any, error) {
	m, err := e.getOrBuildDraftModel(modelID)
	if err != nil {
		return nil, err
	}

	var allErrs []*validation.ValidationError

	if err := validation.ValidateData(m, data); err != nil {
		if me, ok := err.(*validation.MultiValidationError); ok {
			allErrs = append(allErrs, me.Errors...)
		} else if ve, ok := err.(*validation.ValidationError); ok {
			allErrs = append(allErrs, ve)
		} else {
			allErrs = append(allErrs, validation.NewValidationError("", err.Error()))
		}
	}

	if err := e.validateOrbitalReferences(ctx, modelID, data); err != nil {
		if me, ok := err.(*validation.MultiValidationError); ok {
			allErrs = append(allErrs, me.Errors...)
		} else if ve, ok := err.(*validation.ValidationError); ok {
			allErrs = append(allErrs, ve)
		} else {
			allErrs = append(allErrs, validation.NewValidationError("", err.Error()))
		}
	}

	if len(allErrs) > 0 {
		return nil, validation.NewMultiValidationError(allErrs)
	}

	sanitized, err := mapping.SanitizeInput(m, data)
	if err != nil {
		return nil, fmt.Errorf("data sanitization failed: %w", err)
	}

	return e.adapter.Create(ctx, m.Ref(), sanitized)
}

// Find executes a query against the adapter.
func (e *Engine) Find(ctx context.Context, modelID string, q query.Query) ([]map[string]any, int64, error) {
	m, err := e.getOrBuildDraftModel(modelID)
	if err != nil {
		return nil, 0, err
	}
	return e.adapter.Find(ctx, m.Ref(), q)
}

// FindOne gets a record by primary key identifier.
func (e *Engine) FindOne(ctx context.Context, modelID string, id any) (map[string]any, error) {
	m, err := e.getOrBuildDraftModel(modelID)
	if err != nil {
		return nil, err
	}
	return e.adapter.FindOne(ctx, m.Ref(), id)
}

// Update updates fields in an existing record by payload keys.
func (e *Engine) Update(ctx context.Context, modelID string, id any, data map[string]any) (map[string]any, error) {
	m, err := e.getOrBuildDraftModel(modelID)
	if err != nil {
		return nil, err
	}

	// 1. Verify that the target record exists in the database
	_, err = e.adapter.FindOne(ctx, m.Ref(), id)
	if err != nil {
		return nil, fmt.Errorf("record '%v' not found in model '%s'", id, modelID)
	}

	var allErrs []*validation.ValidationError

	// 2. Validate raw payload fields (data types, regex, bounds, non-null)
	if err := validation.ValidatePartialData(m, data); err != nil {
		if me, ok := err.(*validation.MultiValidationError); ok {
			allErrs = append(allErrs, me.Errors...)
		} else if ve, ok := err.(*validation.ValidationError); ok {
			allErrs = append(allErrs, ve)
		} else {
			allErrs = append(allErrs, validation.NewValidationError("", err.Error()))
		}
	}

	if err := e.validateOrbitalReferences(ctx, modelID, data); err != nil {
		if me, ok := err.(*validation.MultiValidationError); ok {
			allErrs = append(allErrs, me.Errors...)
		} else if ve, ok := err.(*validation.ValidationError); ok {
			allErrs = append(allErrs, ve)
		} else {
			allErrs = append(allErrs, validation.NewValidationError("", err.Error()))
		}
	}

	if len(allErrs) > 0 {
		return nil, validation.NewMultiValidationError(allErrs)
	}

	// 3. Coerce input values for database storage
	sanitized, err := mapping.SanitizePartialInput(m, data)
	if err != nil {
		return nil, fmt.Errorf("data sanitization failed: %w", err)
	}

	// 4. Update adapter with payload given
	return e.adapter.Update(ctx, m.Ref(), id, sanitized)
}

// Patch partially updates fields in an existing record by payload keys.
func (e *Engine) Patch(ctx context.Context, modelID string, id any, data map[string]any) (map[string]any, error) {
	return e.Update(ctx, modelID, id, data)
}

// Delete removes a record by primary key identifier.
func (e *Engine) Delete(ctx context.Context, modelID string, id any) error {
	m, err := e.getOrBuildDraftModel(modelID)
	if err != nil {
		return err
	}
	return e.adapter.Delete(ctx, m.Ref(), id)
}

// GetOrBuildDraftModel retrieves or compiles an execution Model for the given ID.
func (e *Engine) GetOrBuildDraftModel(modelID string) (*model.Model, error) {
	return e.getOrBuildDraftModel(modelID)
}

// ValidateOrbitalReferences checks orbital reference constraints against the live database adapter.
func (e *Engine) ValidateOrbitalReferences(ctx context.Context, modelID string, data map[string]any) error {
	return e.validateOrbitalReferences(ctx, modelID, data)
}

func (e *Engine) getOrBuildDraftModel(modelID string) (*model.Model, error) {
	// First check if registered in model registry
	if m, err := e.registry.GetDraft(modelID); err == nil && m != nil {
		return m, nil
	}

	// Try building from ModelConfig + DataModel
	cfg, err := e.registry.GetModelConfig(modelID)
	if err != nil {
		return nil, fmt.Errorf("model '%s' not found in registry", modelID)
	}

	fields := e.registry.ListDataModels(cfg.ID)
	dbName := ""
	if e.project != nil {
		dbName = e.project.AdapterConfig.Database
	}
	execModel := model.BuildModel(cfg, fields, dbName, model.StorageRelational)
	return e.registry.SaveDraft(execModel)
}

// validateOrbitalReferences verifies that foreign keys/orbital references satisfy constraints.
func (e *Engine) validateOrbitalReferences(ctx context.Context, modelID string, data map[string]any) error {
	fields := e.registry.ListDataModels(modelID)
	var orbitalErrs []*validation.ValidationError

	// If no separate DataModels registered, inspect compiled model attributes
	if len(fields) == 0 {
		m, err := e.getOrBuildDraftModel(modelID)
		if err == nil && m != nil {
			for _, attr := range m.Attributes {
				if attr.Reference != nil && attr.Reference.Model != "" {
					fieldKey := attr.Name
					val, exists := data[fieldKey]
					if !exists || val == nil {
						continue
					}
					targetModelID := attr.Reference.Model
					targetRefModel, err := e.getOrBuildDraftModel(targetModelID)
					if err != nil {
						orbitalErrs = append(orbitalErrs, validation.NewValidationError(fieldKey, fmt.Sprintf("orbital reference failed: target model '%s' not found: %v", targetModelID, err)))
						continue
					}

					targetCol := "id"
					if attr.Reference.Attribute != "" {
						targetCol = attr.Reference.Attribute
					}

					q := query.NewQuery().Where(targetCol, query.OpEq, val).LimitOffset(1, 0)
					results, _, findErr := e.adapter.Find(ctx, targetRefModel.Ref(), q)
					if findErr != nil || len(results) == 0 {
						orbitalErrs = append(orbitalErrs, validation.NewReferenceNotFoundError(fieldKey, targetModelID, val))
					} else {
						record := results[0]
						if statusVal, ok := record["status"]; ok && statusVal != nil {
							if !strings.EqualFold(fmt.Sprintf("%v", statusVal), "active") {
								orbitalErrs = append(orbitalErrs, validation.NewValidationError(fieldKey, fmt.Sprintf("orbital reference validation failed: referenced record in '%s' is not active (status='%v')", targetModelID, statusVal)))
							}
						}
					}
				}
			}
		}
		return validation.NewMultiValidationError(orbitalErrs)
	}

	for _, dm := range fields {
		targetModelID := ""
		targetCol := "id"

		if dm.Reference != nil && dm.Reference.Model != "" {
			targetModelID = dm.Reference.Model
			if dm.Reference.Attribute != "" {
				targetCol = dm.Reference.Attribute
			}
		} else if dm.IsOrbitalReference && dm.OrbitalReferenceModelID != nil && *dm.OrbitalReferenceModelID != "" {
			targetModelID = *dm.OrbitalReferenceModelID
			if dm.OrbitalReferenceFieldID != nil && *dm.OrbitalReferenceFieldID != "" {
				targetCol = *dm.OrbitalReferenceFieldID
			}
		}

		if targetModelID == "" {
			continue
		}

		fieldKey := dm.ColumnName
		if fieldKey == "" {
			fieldKey = dm.JSONField
		}

		val, exists := data[fieldKey]
		if (!exists || val == nil) && dm.JSONField != "" {
			val, exists = data[dm.JSONField]
		}
		if (!exists || val == nil) && dm.ColumnName != "" {
			val, exists = data[dm.ColumnName]
		}
		if (!exists || val == nil) && dm.RefName != "" {
			val, exists = data[dm.RefName]
		}
		if (!exists || val == nil) && dm.ID != "" {
			val, exists = data[dm.ID]
		}

		if !exists || val == nil {
			continue
		}

		targetRefModel, err := e.getOrBuildDraftModel(targetModelID)
		if err != nil {
			orbitalErrs = append(orbitalErrs, validation.NewValidationError(fieldKey, fmt.Sprintf("orbital reference failed: target model '%s' not found: %v", targetModelID, err)))
			continue
		}

		// Perform validation strategy (exists, exists_active, not_exists)
		valStrategy := dm.OrbitalReferenceValidation
		if valStrategy == "" {
			valStrategy = model.OrbitalValidationExists
		}

		q := query.NewQuery().Where(targetCol, query.OpEq, val).LimitOffset(1, 0)
		results, _, findErr := e.adapter.Find(ctx, targetRefModel.Ref(), q)
		var record map[string]any
		if findErr == nil && len(results) > 0 {
			record = results[0]
		}

		switch valStrategy {
		case model.OrbitalValidationExists, model.OrbitalValidationExistsActive:
			if record == nil {
				orbitalErrs = append(orbitalErrs, validation.NewReferenceNotFoundError(fieldKey, targetModelID, val))
				continue
			}
			// Verify that target record status is active (if a status column exists on target record)
			if statusVal, ok := record["status"]; ok && statusVal != nil {
				if !strings.EqualFold(fmt.Sprintf("%v", statusVal), "active") {
					orbitalErrs = append(orbitalErrs, validation.NewValidationError(fieldKey, fmt.Sprintf("orbital reference validation failed: referenced record in '%s' is not active (status='%v')", targetModelID, statusVal)))
				}
			}
		case model.OrbitalValidationNotExists:
			if record != nil {
				orbitalErrs = append(orbitalErrs, validation.NewValidationError(fieldKey, fmt.Sprintf("orbital reference validation (not_exists) failed on field '%s': value '%v' already exists in model '%s'", fieldKey, val, targetModelID)))
			}
		}
	}
	return validation.NewMultiValidationError(orbitalErrs)
}

// =========================================================================
// 5. Generic Operation Execution (Function, Procedure, Command, etc.)
// =========================================================================

// RegisterOperation stores metadata for an operation (function, procedure, command, etc.).
func (e *Engine) RegisterOperation(ctx context.Context, op *operation.OperationConfig) (*operation.OperationConfig, error) {
	if op == nil {
		return nil, errors.New("operation cannot be nil")
	}
	if op.Name == "" {
		return nil, errors.New("operation name cannot be empty")
	}
	if op.Target == "" {
		op.Target = op.Name
	}
	return e.registry.SaveOperationConfig(op)
}

// GetOperation retrieves an operation metadata definition.
func (e *Engine) GetOperation(ctx context.Context, nameOrID string) (*operation.OperationConfig, error) {
	return e.registry.GetOperationConfig(nameOrID)
}

// ListOperations returns all registered operations.
func (e *Engine) ListOperations(ctx context.Context) []*operation.OperationConfig {
	return e.registry.ListOperationConfigs()
}

// ExecuteOperation validates parameters and executes an operation through the adapter.
func (e *Engine) ExecuteOperation(ctx context.Context, nameOrID string, args map[string]any) (*execution.ExecutionResult, error) {
	op, err := e.registry.GetOperationConfig(nameOrID)
	if err != nil {
		// Fallback to on-the-fly execution if not pre-registered
		req := execution.ExecutionRequest{
			Operation: operation.OpCustom,
			Target:    nameOrID,
			Arguments: args,
		}
		return e.adapter.Execute(ctx, req)
	}

	// Validate & coerce arguments against parameter definitions
	coercedArgs := make(map[string]any)
	for _, param := range op.Parameters {
		val, exists := args[param.Name]
		if !exists || val == nil {
			if param.DefaultValue != nil {
				coercedArgs[param.Name] = param.DefaultValue
				continue
			}
			if param.Required {
				return nil, fmt.Errorf("operation '%s': missing required parameter '%s'", op.Name, param.Name)
			}
			continue
		}

		coerced, err := mapping.CoerceValue(val, param.DataType)
		if err != nil {
			return nil, fmt.Errorf("operation '%s' parameter '%s': %w", op.Name, param.Name, err)
		}
		coercedArgs[param.Name] = coerced
	}

	// Include any unmapped arguments passed by caller
	for k, v := range args {
		if _, ok := coercedArgs[k]; !ok {
			coercedArgs[k] = v
		}
	}

	req := execution.ExecutionRequest{
		Operation: op.Type,
		Target:    op.Target,
		Arguments: coercedArgs,
	}

	return e.adapter.Execute(ctx, req)
}

// =========================================================================
// 6. Transaction Lifecycle
// =========================================================================

// Transaction executes a function within an atomic transaction with automatic rollback on error.
func (e *Engine) Transaction(ctx context.Context, fn func(tx adapter.Transaction) error) error {
	tx, err := e.adapter.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback(ctx)
			panic(p)
		}
	}()

	if err := fn(tx); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

