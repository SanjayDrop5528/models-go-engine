package project_test

import (
	"context"
	"github.com/SanjayDrop5528/models-go-engine/adapter"
	"github.com/SanjayDrop5528/models-go-engine/model"
	"github.com/SanjayDrop5528/models-go-engine/project"
	"testing"
)

func TestMetadataPersistenceAndRestoreFromDB(t *testing.T) {
	ctx := context.Background()
	mockAdp := adapter.NewMockAdapter()

	engine1 := project.New(mockAdp)

	cfg := &model.ModelConfig{
		ID:          "product",
		Name:        "product",
		Description: "Product Catalog",
		Status:      model.ModelConfigStatusActive,
		Version:     1,
	}
	if _, err := engine1.CreateModelConfig(ctx, cfg); err != nil {
		t.Fatalf("failed to create model config: %v", err)
	}

	dm1 := &model.DataModel{
		ID:           "product_id",
		ModelID:      "product",
		ColumnName:   "id",
		JSONField:    "id",
		DataType:     model.TypeLong,
		IsPrimaryKey: true,
		IsRequired:   true,
		Status:       model.DataModelStatusActive,
	}
	dm2 := &model.DataModel{
		ID:         "product_sku",
		ModelID:    "product",
		ColumnName: "sku",
		JSONField:  "sku",
		DataType:   model.TypeString,
		IsRequired: true,
		Status:     model.DataModelStatusActive,
	}
	if _, err := engine1.AddDataModel(ctx, dm1); err != nil {
		t.Fatalf("failed to add dm1: %v", err)
	}
	if _, err := engine1.AddDataModel(ctx, dm2); err != nil {
		t.Fatalf("failed to add dm2: %v", err)
	}

	engine2 := project.New(mockAdp)

	restoredCfg, err := engine2.GetModelConfig(ctx, "product")
	if err != nil {
		t.Fatalf("expected restored model_config 'product', got error: %v", err)
	}
	if restoredCfg.Name != "product" {
		t.Fatalf("expected name 'product', got '%s'", restoredCfg.Name)
	}

	fields := engine2.ListDataModels(ctx, "product")
	if len(fields) != 2 {
		t.Fatalf("expected 2 restored fields, got %d", len(fields))
	}

	activeModel, err := engine2.GetRegistry().GetActive("product")
	if err != nil {
		t.Fatalf("expected active model 'product', got error: %v", err)
	}
	if len(activeModel.Attributes) != 2 {
		t.Fatalf("expected 2 attributes on active model, got %d", len(activeModel.Attributes))
	}
}
