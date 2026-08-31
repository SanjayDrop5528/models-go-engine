package service

import (
	"context"
	"fmt"
	"github.com/SanjayDrop5528/models-go-engine/adapter"
	"github.com/SanjayDrop5528/models-go-engine/diff"
	"github.com/SanjayDrop5528/models-go-engine/model"
	"github.com/SanjayDrop5528/models-go-engine/plan"
	"github.com/SanjayDrop5528/models-go-engine/registry"
	"github.com/SanjayDrop5528/models-go-engine/schema"
	"github.com/SanjayDrop5528/models-go-engine/validation"
)

// ApplyRequest specifies options when applying a schema change.
type ApplyRequest struct {
	AllowDestructive bool           `json:"allow_destructive"`
	Hints            diff.DiffHints `json:"hints"`
}

// ApplyResult contains the outcome of an applied schema migration.
type ApplyResult struct {
	ModelID        string                 `json:"model_id"`
	StorageName    string                 `json:"storage_name"`
	Database       string                 `json:"database"`
	AppliedChanges []diff.SchemaOperation `json:"applied_changes"`
	VerifiedSchema *schema.Schema         `json:"verified_schema"`
	Status         model.ModelStatus      `json:"status"`
	Message        string                 `json:"message"`
}

// SchemaService orchestrates safe, diff-based schema migrations.
type SchemaService struct {
	registry   *registry.ModelRegistry
	adapters   *adapter.Registry
	diffEngine *diff.DiffEngine
}

// NewSchemaService creates a new SchemaService.
func NewSchemaService(reg *registry.ModelRegistry, adapters *adapter.Registry) *SchemaService {
	return &SchemaService{
		registry:   reg,
		adapters:   adapters,
		diffEngine: diff.NewDiffEngine(),
	}
}

// GetCurrentSchema fetches the live database schema from the target database adapter.
func (s *SchemaService) GetCurrentSchema(ctx context.Context, modelID string) (*schema.Schema, error) {
	m, err := s.registry.GetDraft(modelID)
	if err != nil {
		return nil, err
	}

	adp, err := s.adapters.Get(m.Database)
	if err != nil {
		return nil, err
	}

	return adp.GetSchema(ctx, m.Ref())
}

// GetDiff computes the difference between current live database schema and desired model schema.
func (s *SchemaService) GetDiff(ctx context.Context, modelID string, hints diff.DiffHints) (*diff.SchemaDiff, error) {
	desiredModel, err := s.registry.GetDraft(modelID)
	if err != nil {
		return nil, err
	}

	adp, err := s.adapters.Get(desiredModel.Database)
	if err != nil {
		return nil, err
	}

	currentSchema, err := adp.GetSchema(ctx, desiredModel.Ref())
	if err != nil {
		return nil, fmt.Errorf("failed to introspect database schema: %w", err)
	}

	desiredSchema := schema.FromModel(desiredModel)
	return s.diffEngine.Compare(currentSchema, desiredSchema, hints)
}

// Preview computes the diff and generates native preview statements from the adapter.
func (s *SchemaService) Preview(ctx context.Context, modelID string, hints diff.DiffHints) (*plan.SchemaPreview, error) {
	desiredModel, err := s.registry.GetDraft(modelID)
	if err != nil {
		return nil, err
	}

	diffResult, err := s.GetDiff(ctx, modelID, hints)
	if err != nil {
		return nil, err
	}

	schemaPlan := plan.BuildPlan(desiredModel.ID, desiredModel.StorageName, desiredModel.Database, diffResult)

	adp, err := s.adapters.Get(desiredModel.Database)
	if err != nil {
		return nil, err
	}

	return adp.PreviewSchemaChange(ctx, schemaPlan)
}

// Apply executes the complete safe migration flow:
// 1. Mark status APPLYING
// 2. Re-introspect live DB schema
// 3. Re-calculate diff
// 4. Validate safety rules & destructive flags
// 5. Build minimal plan
// 6. Adapter applies changes
// 7. Re-introspect & verify
// 8. Promote model to ACTIVE in registry
func (s *SchemaService) Apply(ctx context.Context, modelID string, req ApplyRequest) (*ApplyResult, error) {
	desiredModel, err := s.registry.GetDraft(modelID)
	if err != nil {
		return nil, err
	}

	adp, err := s.adapters.Get(desiredModel.Database)
	if err != nil {
		return nil, err
	}

	// 1. Mark status APPLYING
	_ = s.registry.SetStatus(modelID, model.StatusApplying)

	// 2. RE-INTROSPECT LIVE DATABASE
	currentSchema, err := adp.GetSchema(ctx, desiredModel.Ref())
	if err != nil {
		_ = s.registry.SetStatus(modelID, model.StatusFailed)
		return nil, fmt.Errorf("failed to re-introspect database schema before apply: %w", err)
	}

	// 3. RE-CALCULATE DIFF
	desiredSchema := schema.FromModel(desiredModel)
	freshDiff, err := s.diffEngine.Compare(currentSchema, desiredSchema, req.Hints)
	if err != nil {
		_ = s.registry.SetStatus(modelID, model.StatusFailed)
		return nil, fmt.Errorf("failed to compute fresh schema diff: %w", err)
	}

	// If no changes needed, promote directly
	if !freshDiff.HasChanges {
		activeModel, _ := s.registry.SetActive(modelID, desiredModel)
		return &ApplyResult{
			ModelID:        modelID,
			StorageName:    desiredModel.StorageName,
			Database:       desiredModel.Database,
			AppliedChanges: nil,
			VerifiedSchema: currentSchema,
			Status:         activeModel.Status,
			Message:        "Schema is already up to date. No changes applied.",
		}, nil
	}

	// 4. BUILD FRESH PLAN
	freshPlan := plan.BuildPlan(desiredModel.ID, desiredModel.StorageName, desiredModel.Database, freshDiff)

	// 5. VALIDATE SAFETY RULES
	if err := validation.ValidatePlan(freshPlan, req.AllowDestructive); err != nil {
		_ = s.registry.SetStatus(modelID, model.StatusDraft)
		return nil, err
	}

	// 6. ADAPTER EXECUTION
	if err := adp.ApplySchemaChange(ctx, freshPlan); err != nil {
		_ = s.registry.SetStatus(modelID, model.StatusFailed)
		return nil, fmt.Errorf("adapter failed to apply schema changes: %w", err)
	}

	// 7. VERIFY BY RE-INTROSPECTING POST-APPLY
	verifiedSchema, err := adp.GetSchema(ctx, desiredModel.Ref())
	if err != nil {
		_ = s.registry.SetStatus(modelID, model.StatusDegraded)
		return nil, fmt.Errorf("schema applied but failed post-migration verification: %w", err)
	}

	// 8. PROMOTE TO ACTIVE IN REGISTRY
	activeModel, err := s.registry.SetActive(modelID, desiredModel)
	if err != nil {
		return nil, fmt.Errorf("failed to activate model metadata: %w", err)
	}

	return &ApplyResult{
		ModelID:        modelID,
		StorageName:    desiredModel.StorageName,
		Database:       desiredModel.Database,
		AppliedChanges: freshPlan.Operations,
		VerifiedSchema: verifiedSchema,
		Status:         activeModel.Status,
		Message:        fmt.Sprintf("Successfully applied %d schema change(s)", len(freshPlan.Operations)),
	}, nil
}

// Sync introspects live database and populates or aligns model definition.
func (s *SchemaService) Sync(ctx context.Context, modelID string) (*model.Model, error) {
	m, err := s.registry.GetDraft(modelID)
	if err != nil {
		return nil, err
	}

	adp, err := s.adapters.Get(m.Database)
	if err != nil {
		return nil, err
	}

	liveSchema, err := adp.GetSchema(ctx, m.Ref())
	if err != nil {
		return nil, fmt.Errorf("failed to introspect database: %w", err)
	}

	if liveSchema == nil || len(liveSchema.Attributes) == 0 {
		return nil, fmt.Errorf("table/collection '%s' does not exist in database '%s'", m.StorageName, m.Database)
	}

	// Update model attributes from live schema
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
	return s.registry.SetActive(modelID, m)
}
