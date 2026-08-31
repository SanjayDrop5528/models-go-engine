package service_test

import (
	"context"
	"github.com/SanjayDrop5528/models-go-engine/adapter"
	"github.com/SanjayDrop5528/models-go-engine/crud"
	"github.com/SanjayDrop5528/models-go-engine/diff"
	"github.com/SanjayDrop5528/models-go-engine/model"
	"github.com/SanjayDrop5528/models-go-engine/query"
	"github.com/SanjayDrop5528/models-go-engine/registry"
	"github.com/SanjayDrop5528/models-go-engine/service"
	"testing"
)

func setupTestServices() (*service.ModelService, *service.SchemaService, *crud.Engine, *adapter.MockAdapter) {
	mockAdapter := adapter.NewMockAdapter()
	adpRegistry := adapter.NewRegistry()
	adpRegistry.Register("memory", mockAdapter)

	modelReg := registry.NewModelRegistry()
	modelSvc := service.NewModelService(modelReg)
	schemaSvc := service.NewSchemaService(modelReg, adpRegistry)
	crudEng := crud.NewEngine(adpRegistry)

	return modelSvc, schemaSvc, crudEng, mockAdapter
}

func TestComplete_ModelChangeFlow(t *testing.T) {
	ctx := context.Background()
	modelSvc, schemaSvc, crudEng, memAdapter := setupTestServices()

	// 1. User creates Model: Employee (id, name, email, age)
	initialModel := &model.Model{
		ID:          "employee",
		Name:        "Employee",
		StorageName: "employees",
		Database:    "memory",
		StorageType: model.StorageRelational,
		Attributes: []model.Attribute{
			{Name: "id", Type: model.TypeLong, AutoIncrement: true},
			{Name: "name", Type: model.TypeString, Nullable: false},
			{Name: "email", Type: model.TypeString, Nullable: false},
			{Name: "age", Type: model.TypeInt, Nullable: true},
		},
		PrimaryKey: &model.PrimaryKey{
			Columns: []string{"id"},
		},
	}

	draft, err := modelSvc.CreateDraft(ctx, initialModel)
	if err != nil {
		t.Fatalf("failed to create draft: %v", err)
	}
	if draft.Status != model.StatusDraft {
		t.Fatalf("expected StatusDraft, got %s", draft.Status)
	}

	// 2. Preview initial schema creation
	preview, err := schemaSvc.Preview(ctx, "employee", diff.DiffHints{})
	if err != nil {
		t.Fatalf("preview failed: %v", err)
	}
	if len(preview.Changes) != 1 || preview.Changes[0].Type != diff.OpCreateTable {
		t.Fatalf("expected CREATE_TABLE in preview")
	}

	// 3. Apply initial schema creation
	applyRes, err := schemaSvc.Apply(ctx, "employee", service.ApplyRequest{})
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if applyRes.Status != model.StatusActive {
		t.Fatalf("expected active model status, got %s", applyRes.Status)
	}

	// Verify live database schema in adapter
	liveSchema, err := memAdapter.GetSchema(ctx, initialModel.Ref())
	if err != nil || liveSchema == nil {
		t.Fatalf("table was not created in adapter: %v", err)
	}
	if len(liveSchema.Attributes) != 4 {
		t.Fatalf("expected 4 attributes in live DB, got %d", len(liveSchema.Attributes))
	}

	// 4. Insert dynamic data into employees table
	createdRecord, err := crudEng.Create(ctx, draft, map[string]any{
		"name":  "Alice",
		"email": "alice@example.com",
		"age":   30,
	})
	if err != nil {
		t.Fatalf("CRUD Create failed: %v", err)
	}
	if createdRecord["id"] == nil || createdRecord["name"] != "Alice" {
		t.Fatalf("invalid created record: %v", createdRecord)
	}

	// 5. User changes Model: Adds 'salary' NUMERIC
	updatedDraft := &model.Model{
		ID:          "employee",
		Name:        "Employee",
		StorageName: "employees",
		Database:    "memory",
		StorageType: model.StorageRelational,
		Attributes: []model.Attribute{
			{Name: "id", Type: model.TypeLong, AutoIncrement: true},
			{Name: "name", Type: model.TypeString, Nullable: false},
			{Name: "email", Type: model.TypeString, Nullable: false},
			{Name: "age", Type: model.TypeInt, Nullable: true},
			{Name: "salary", Type: model.TypeDecimal, Precision: 10, Scale: 2, Nullable: true},
		},
		PrimaryKey: &model.PrimaryKey{
			Columns: []string{"id"},
		},
	}
	_, err = modelSvc.UpdateDraft(ctx, "employee", updatedDraft)
	if err != nil {
		t.Fatalf("failed to update draft: %v", err)
	}

	// 6. Calculate Diff: Must ONLY detect ADD_COLUMN salary (NOT recreate table)
	diffRes, err := schemaSvc.GetDiff(ctx, "employee", diff.DiffHints{})
	if err != nil {
		t.Fatalf("diff failed: %v", err)
	}
	if len(diffRes.Operations) != 1 {
		t.Fatalf("expected exactly 1 minimal diff operation, got %d", len(diffRes.Operations))
	}
	if diffRes.Operations[0].Type != diff.OpAddColumn || diffRes.Operations[0].ObjectName != "salary" {
		t.Fatalf("expected ADD_COLUMN salary, got %v", diffRes.Operations[0])
	}

	// 7. Preview
	preview2, err := schemaSvc.Preview(ctx, "employee", diff.DiffHints{})
	if err != nil {
		t.Fatalf("preview failed: %v", err)
	}
	if preview2.HasDestructive {
		t.Fatal("adding nullable salary should not be destructive")
	}

	// 8. Apply changes
	applyRes2, err := schemaSvc.Apply(ctx, "employee", service.ApplyRequest{})
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if len(applyRes2.AppliedChanges) != 1 {
		t.Fatalf("expected 1 applied change, got %d", len(applyRes2.AppliedChanges))
	}

	// Verify live database schema in adapter has 5 columns now
	liveSchema2, _ := memAdapter.GetSchema(ctx, initialModel.Ref())
	if len(liveSchema2.Attributes) != 5 {
		t.Fatalf("expected 5 columns in live DB after ADD salary, got %d", len(liveSchema2.Attributes))
	}

	// Verify existing records still exist with salary = nil
	records, total, err := crudEng.Find(ctx, updatedDraft, query.NewQuery())
	if err != nil || total != 1 {
		t.Fatalf("expected 1 existing record retained, got total %d, err: %v", total, err)
	}
	if records[0]["name"] != "Alice" {
		t.Fatalf("data corrupted: %v", records[0])
	}

	// 9. Destructive Change Flow: Remove 'salary'
	destructiveModel := &model.Model{
		ID:          "employee",
		Name:        "Employee",
		StorageName: "employees",
		Database:    "memory",
		StorageType: model.StorageRelational,
		Attributes: []model.Attribute{
			{Name: "id", Type: model.TypeLong, AutoIncrement: true},
			{Name: "name", Type: model.TypeString, Nullable: false},
			{Name: "email", Type: model.TypeString, Nullable: false},
			{Name: "age", Type: model.TypeInt, Nullable: true},
		},
		PrimaryKey: &model.PrimaryKey{
			Columns: []string{"id"},
		},
	}
	_, _ = modelSvc.UpdateDraft(ctx, "employee", destructiveModel)

	// Attempt Apply WITHOUT AllowDestructive -> MUST FAIL
	_, err = schemaSvc.Apply(ctx, "employee", service.ApplyRequest{AllowDestructive: false})
	if err == nil {
		t.Fatal("expected apply to fail on destructive change without explicit confirmation")
	}

	// Attempt Apply WITH AllowDestructive = true -> MUST SUCCEED
	applyRes3, err := schemaSvc.Apply(ctx, "employee", service.ApplyRequest{AllowDestructive: true})
	if err != nil {
		t.Fatalf("destructive apply failed: %v", err)
	}
	if len(applyRes3.AppliedChanges) != 1 || applyRes3.AppliedChanges[0].Type != diff.OpRemoveColumn {
		t.Fatalf("expected 1 REMOVE_COLUMN operation applied")
	}

	// Verify live database schema in adapter is back to 4 columns
	liveSchema3, _ := memAdapter.GetSchema(ctx, initialModel.Ref())
	if len(liveSchema3.Attributes) != 4 {
		t.Fatalf("expected 4 columns after dropping salary, got %d", len(liveSchema3.Attributes))
	}
}
