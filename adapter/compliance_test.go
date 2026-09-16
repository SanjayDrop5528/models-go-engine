package adapter

import (
	"context"
	"testing"

	"github.com/SanjayDrop5528/models-go-engine/model"
	"github.com/SanjayDrop5528/models-go-engine/query"
)

// RunAdapterComplianceSuite executes standard contract compliance tests against an Adapter implementation.
func RunAdapterComplianceSuite(t *testing.T, a Adapter, cleanup func()) {
	ctx := context.Background()
	if cleanup != nil {
		defer cleanup()
	}

	t.Run("NameAndDatabaseName", func(t *testing.T) {
		if a.Name() == "" {
			t.Error("adapter Name() must not be empty")
		}
		if a.DatabaseName() == "" {
			t.Error("adapter DatabaseName() must not be empty")
		}
	})

	t.Run("CapabilitiesMatrix", func(t *testing.T) {
		caps := GetCapabilities(a)
		if caps.Category == "" {
			t.Error("capabilities Category must not be empty")
		}
		if len(caps.SupportedSaveModes) == 0 {
			t.Error("capabilities must declare at least one supported SaveMode")
		}
	})

	t.Run("ConnectAndPing", func(t *testing.T) {
		if err := a.Connect(ctx); err != nil {
			t.Fatalf("Connect failed: %v", err)
		}
		if err := a.Ping(ctx); err != nil {
			t.Fatalf("Ping failed: %v", err)
		}
	})

	t.Run("MetadataLifecycle", func(t *testing.T) {
		if err := a.EnsureMetadataTables(ctx); err != nil {
			t.Fatalf("EnsureMetadataTables failed: %v", err)
		}
	})

	t.Run("BasicCRUD", func(t *testing.T) {
		mRef := model.ModelRef{
			ID:          "compliance_test_entity",
			Name:        "ComplianceTestEntity",
			StorageName: "compliance_test_entities",
			PrimaryKey:  "id",
		}

		// 1. Create
		record := map[string]any{
			"id":     "comp_1",
			"name":   "Test Entity",
			"status": "active",
		}
		created, err := a.Create(ctx, mRef, record)
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}
		if created == nil {
			t.Fatal("expected created record, got nil")
		}

		// 2. FindOne
		found, err := a.FindOne(ctx, mRef, "comp_1")
		if err != nil {
			t.Fatalf("FindOne failed: %v", err)
		}
		if found == nil || found["name"] != "Test Entity" {
			t.Errorf("unexpected FindOne result: %v", found)
		}

		// 3. Find
		q := query.NewQuery().Where("status", query.OpEq, "active")
		list, total, err := a.Find(ctx, mRef, q)
		if err != nil {
			t.Fatalf("Find failed: %v", err)
		}
		if total < 1 || len(list) < 1 {
			t.Errorf("expected at least 1 record, got total=%d len=%d", total, len(list))
		}

		// 4. Update
		updated, err := a.Update(ctx, mRef, "comp_1", map[string]any{
			"id":     "comp_1",
			"name":   "Updated Entity",
			"status": "active",
		})
		if err != nil {
			t.Fatalf("Update failed: %v", err)
		}
		if updated["name"] != "Updated Entity" {
			t.Errorf("expected updated name, got %v", updated["name"])
		}

		// 5. Delete
		if err := a.Delete(ctx, mRef, "comp_1"); err != nil {
			t.Fatalf("Delete failed: %v", err)
		}
	})
}

func TestMockAdapter_Compliance(t *testing.T) {
	mock := NewMockAdapter()
	RunAdapterComplianceSuite(t, mock, nil)
}
