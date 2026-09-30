package project

import (
	"context"
	"testing"

	"github.com/SanjayDrop5528/models-go-engine/adapter"
	"github.com/SanjayDrop5528/models-go-engine/model"
	"github.com/SanjayDrop5528/models-go-engine/query"
)

func TestDatabaseRequiresRegisteredActiveModelForModelAndTableCRUD(t *testing.T) {
	ctx := context.Background()
	engine := New(adapter.NewMockAdapter())
	configs := []*model.ModelConfig{{
		ID: "orders", Name: "Order", RefName: "orders", Schema: "sales", Table: "orders",
		IsTable: true, Status: model.ModelConfigStatusActive, Version: 1,
	}}
	fields := []*model.DataModel{{
		ID: "orders_id", ModelID: "orders", ColumnName: "id", JSONField: "id",
		DataType: model.TypeString, IsPrimaryKey: true, Status: model.DataModelStatusActive,
	}}
	if err := engine.LoadModels(ctx, configs, fields); err != nil {
		t.Fatal(err)
	}

	created, err := engine.DB().Model(ctx, "orders").Create(ctx, map[string]any{"id": "ord-1"})
	if err != nil || created["id"] != "ord-1" {
		t.Fatalf("model create failed: row=%v err=%v", created, err)
	}
	rows, _, err := engine.DB().Table(ctx, "sales.orders").Find(ctx, query.NewQuery())
	if err != nil || len(rows) != 1 {
		t.Fatalf("table find failed: rows=%v err=%v", rows, err)
	}
	if _, _, err := engine.DB().Table(ctx, "sales.unknown").Find(ctx, query.NewQuery()); err == nil {
		t.Fatal("unregistered table query must be rejected")
	}
}
