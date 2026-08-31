package project_test

import (
	"context"
	"fmt"
	"github.com/SanjayDrop5528/models-go-engine/adapter"
	"github.com/SanjayDrop5528/models-go-engine/model"
	"github.com/SanjayDrop5528/models-go-engine/operation"
	"github.com/SanjayDrop5528/models-go-engine/project"
	"github.com/SanjayDrop5528/models-go-engine/query"
	"github.com/SanjayDrop5528/models-go-engine/service"
	"testing"
	"time"
)

func TestProjectInitializationAndAdapterConfig(t *testing.T) {
	mockAdapter := adapter.NewMockAdapter()

	proj, err := project.NewProject(project.ProjectConfig{
		ID:          "proj-test-1",
		Name:        "Test Enterprise System",
		Description: "Enterprise backend project",
		AdapterConfig: project.AdapterConfig{
			AdapterType: "memory",
			Database:    "test_db",
			QueryContext: project.QueryContextConfig{
				DefaultTimeout:     10 * time.Second,
				MaxLimit:           500,
				SlowQueryThreshold: 100 * time.Millisecond,
			},
			Options: map[string]any{
				"in_memory_persistence": true,
			},
		},
		CreatedBy: "admin-user",
	}, mockAdapter)

	if err != nil {
		t.Fatalf("failed to initialize project: %v", err)
	}

	if proj.ID != "proj-test-1" {
		t.Errorf("expected project ID 'proj-test-1', got '%s'", proj.ID)
	}
	if proj.AdapterConfig.AdapterType != "memory" {
		t.Errorf("expected adapter type 'memory', got '%s'", proj.AdapterConfig.AdapterType)
	}
	if proj.Engine == nil {
		t.Fatal("expected non-nil Engine attached to Project")
	}
	if proj.Engine.GetAdapter() != mockAdapter {
		t.Error("expected project engine to bind the configured adapter")
	}
}

func TestModelConfigAndDataModelLifecycle(t *testing.T) {
	ctx := context.Background()
	mockAdapter := adapter.NewMockAdapter()

	proj, err := project.NewProject(project.ProjectConfig{
		Name: "HR Project",
		AdapterConfig: project.AdapterConfig{
			AdapterType: "memory",
			Database:    "hr_db",
		},
	}, mockAdapter)
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}
	engine := proj.Engine

	// 1. Create ModelConfig (Notice: clean struct with no adapter_type or connection_config_id)
	userConfig := &model.ModelConfig{
		ID:                   "user",
		Name:                 "User",
		RefName:              "users",
		IsAttributeReference: false,
		Description:          "Application Users",
		Status:               model.ModelConfigStatusDraft,
		Version:              1,
		IsSystem:             false,
		CreatedBy:            "admin",
		UpdatedBy:            "admin",
	}
	createdConfig, err := engine.CreateModelConfig(ctx, userConfig)
	if err != nil {
		t.Fatalf("failed to create model_config: %v", err)
	}
	if createdConfig.Name != "User" {
		t.Errorf("expected name 'User', got '%s'", createdConfig.Name)
	}

	// 2. Add DataModel fields
	fields := []*model.DataModel{
		{
			ModelID:      "user",
			ColumnName:   "id",
			JSONField:    "id",
			DataType:     model.TypeLong,
			IsPrimaryKey: true,
			IsRequired:   true,
			Status:       model.DataModelStatusActive,
		},
		{
			ModelID:    "user",
			ColumnName: "email",
			JSONField:  "email",
			DataType:   model.TypeString,
			IsRequired: true,
			IsUnique:   true,
			Status:     model.DataModelStatusActive,
		},
		{
			ModelID:      "user",
			ColumnName:   "role",
			JSONField:    "role",
			DataType:     model.TypeString,
			DefaultValue: "member",
			Status:       model.DataModelStatusActive,
		},
	}

	for _, f := range fields {
		if _, err := engine.AddDataModel(ctx, f); err != nil {
			t.Fatalf("failed to add data_model field '%s': %v", f.ColumnName, err)
		}
	}

	storedFields := engine.ListDataModels(ctx, "user")
	if len(storedFields) != 3 {
		t.Fatalf("expected 3 data_model fields, got %d", len(storedFields))
	}

	// 3. Schema Apply via Project Engine
	applyResult, err := engine.ApplySchema(ctx, "user", service.ApplyRequest{})
	if err != nil {
		t.Fatalf("failed to apply schema: %v", err)
	}
	if applyResult.Status != model.StatusActive {
		t.Errorf("expected active status, got '%s'", applyResult.Status)
	}

	// 4. Base CRUD operations via Project Engine
	createdUser, err := engine.Create(ctx, "user", map[string]any{
		"id":    int64(101),
		"email": "alice@example.com",
	})
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}
	if createdUser["role"] != "member" {
		t.Errorf("expected default role 'member', got '%v'", createdUser["role"])
	}

	// FindOne
	fetched, err := engine.FindOne(ctx, "user", int64(101))
	if err != nil {
		t.Fatalf("failed to find user: %v", err)
	}
	if fetched["email"] != "alice@example.com" {
		t.Errorf("expected email alice@example.com, got %v", fetched["email"])
	}

	// Find
	list, total, err := engine.Find(ctx, "user", query.Query{})
	if err != nil {
		t.Fatalf("failed to list users: %v", err)
	}
	if total != 1 || len(list) != 1 {
		t.Errorf("expected 1 user, got total %d, count %d", total, len(list))
	}

	// Patch
	updated, err := engine.Patch(ctx, "user", int64(101), map[string]any{
		"role": "admin",
	})
	if err != nil {
		t.Fatalf("failed to patch user: %v", err)
	}
	if updated["role"] != "admin" {
		t.Errorf("expected role 'admin', got '%v'", updated["role"])
	}

	// Delete
	if err := engine.Delete(ctx, "user", int64(101)); err != nil {
		t.Fatalf("failed to delete user: %v", err)
	}
}

func TestCustomTypeValidation(t *testing.T) {
	ctx := context.Background()
	mockAdapter := adapter.NewMockAdapter()

	proj, err := project.NewProject(project.ProjectConfig{
		Name: "Custom Type Test",
		AdapterConfig: project.AdapterConfig{
			AdapterType: "memory",
			Database:    "test_db",
		},
	}, mockAdapter)
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}
	engine := proj.Engine

	// 1. Create an Attribute Reference Model (Address)
	addressModel := &model.ModelConfig{
		ID:                   "address",
		Name:                 "Address",
		IsAttributeReference: true, // Mark as usable as custom type
		Status:               model.ModelConfigStatusActive,
		CreatedBy:            "admin",
	}
	if _, err := engine.CreateModelConfig(ctx, addressModel); err != nil {
		t.Fatalf("failed to create address model: %v", err)
	}

	// 2. Create a Regular Entity Model (Customer)
	customerModel := &model.ModelConfig{
		ID:                   "customer",
		Name:                 "Customer",
		IsAttributeReference: false,
		Status:               model.ModelConfigStatusDraft,
		CreatedBy:            "admin",
	}
	if _, err := engine.CreateModelConfig(ctx, customerModel); err != nil {
		t.Fatalf("failed to create customer model: %v", err)
	}

	// 3. Add field with custom_type_id referencing Address -> should succeed
	customTypeID := "address"
	validField := &model.DataModel{
		ModelID:      "customer",
		ColumnName:   "billing_address",
		DataType:     model.TypeJSON,
		CustomTypeID: &customTypeID,
	}
	if _, err := engine.AddDataModel(ctx, validField); err != nil {
		t.Fatalf("expected valid custom_type_id to succeed, got: %v", err)
	}

	// 4. Try referencing a model that is NOT an attribute reference -> should fail
	invalidCustomTypeID := "customer" // customer is not marked with is_attribute_reference=true
	invalidField := &model.DataModel{
		ModelID:      "customer",
		ColumnName:   "nested_customer",
		DataType:     model.TypeJSON,
		CustomTypeID: &invalidCustomTypeID,
	}
	if _, err := engine.AddDataModel(ctx, invalidField); err == nil {
		t.Fatal("expected error when custom_type_id references non-attribute-reference model, got nil")
	}
}

func TestOrbitalReferenceValidation(t *testing.T) {
	ctx := context.Background()
	mockAdapter := adapter.NewMockAdapter()

	proj, err := project.NewProject(project.ProjectConfig{
		Name: "Orbital Reference Project",
		AdapterConfig: project.AdapterConfig{
			AdapterType: "memory",
			Database:    "corp_db",
		},
	}, mockAdapter)
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}
	engine := proj.Engine

	// 1. Setup Department model
	deptConfig := &model.ModelConfig{
		ID:        "department",
		Name:      "Department",
		Status:    model.ModelConfigStatusActive,
		CreatedBy: "admin",
	}
	_, _ = engine.CreateModelConfig(ctx, deptConfig)
	_, _ = engine.AddDataModel(ctx, &model.DataModel{
		ModelID: "department", ColumnName: "id", DataType: model.TypeString, IsPrimaryKey: true,
	})
	_, _ = engine.AddDataModel(ctx, &model.DataModel{
		ModelID: "department", ColumnName: "name", DataType: model.TypeString,
	})
	_, _ = engine.AddDataModel(ctx, &model.DataModel{
		ModelID: "department", ColumnName: "status", DataType: model.TypeString,
	})
	_, err = engine.ApplySchema(ctx, "department", service.ApplyRequest{})
	if err != nil {
		t.Fatalf("failed to apply department schema: %v", err)
	}

	// Insert active department
	_, err = engine.Create(ctx, "department", map[string]any{
		"id":     "dept-eng",
		"name":   "Engineering",
		"status": "active",
	})
	if err != nil {
		t.Fatalf("failed to insert department: %v", err)
	}

	// 2. Setup Employee model with orbital_reference to Department.id
	empConfig := &model.ModelConfig{
		ID:        "employee",
		Name:      "Employee",
		Status:    model.ModelConfigStatusActive,
		CreatedBy: "admin",
	}
	_, _ = engine.CreateModelConfig(ctx, empConfig)
	_, _ = engine.AddDataModel(ctx, &model.DataModel{
		ModelID: "employee", ColumnName: "id", DataType: model.TypeString, IsPrimaryKey: true,
	})

	orbitalModelID := "department"
	orbitalFieldID := "id"
	_, _ = engine.AddDataModel(ctx, &model.DataModel{
		ModelID:                    "employee",
		ColumnName:                 "department_id",
		DataType:                   model.TypeString,
		IsOrbitalReference:         true,
		OrbitalReferenceModelID:    &orbitalModelID,
		OrbitalReferenceFieldID:    &orbitalFieldID,
		OrbitalReferenceValidation: model.OrbitalValidationExistsActive,
	})
	_, err = engine.ApplySchema(ctx, "employee", service.ApplyRequest{})
	if err != nil {
		t.Fatalf("failed to apply employee schema: %v", err)
	}

	// 3. Create employee with existing active department -> should succeed
	empSuccess, err := engine.Create(ctx, "employee", map[string]any{
		"id":            "emp-1",
		"department_id": "dept-eng",
	})
	if err != nil {
		t.Fatalf("expected valid orbital reference to succeed: %v", err)
	}
	if empSuccess["id"] != "emp-1" {
		t.Errorf("expected emp-1, got %v", empSuccess["id"])
	}

	// 4. Create employee with non-existent department -> should fail orbital validation
	_, err = engine.Create(ctx, "employee", map[string]any{
		"id":            "emp-2",
		"department_id": "dept-non-existent",
	})
	if err == nil {
		t.Fatal("expected orbital reference validation error for non-existent department, got nil")
	}
}

func TestGenericOperationExecution(t *testing.T) {
	ctx := context.Background()
	mockAdapter := adapter.NewMockAdapter()

	proj, err := project.NewProject(project.ProjectConfig{
		Name: "Operation Test Project",
		AdapterConfig: project.AdapterConfig{
			AdapterType: "memory",
			Database:    "corp_db",
		},
	}, mockAdapter)
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}
	engine := proj.Engine

	// 1. Register an operation (Function: calculate_salary)
	calcSalaryOp := &operation.OperationConfig{
		Name:   "calculate_salary",
		Type:   operation.OpFunction,
		Target: "calculate_salary",
		Parameters: []operation.OperationParameter{
			{Name: "employee_id", DataType: model.TypeLong, Required: true},
			{Name: "bonus_multiplier", DataType: model.TypeFloat, DefaultValue: 1.0},
		},
		ReturnType: model.TypeDecimal,
		IsReadOnly: true,
	}

	registered, err := engine.RegisterOperation(ctx, calcSalaryOp)
	if err != nil {
		t.Fatalf("failed to register operation: %v", err)
	}
	if registered.Name != "calculate_salary" {
		t.Errorf("expected calculate_salary, got %s", registered.Name)
	}

	// 2. Execute operation with valid arguments -> should succeed
	res, err := engine.ExecuteOperation(ctx, "calculate_salary", map[string]any{
		"employee_id": 101,
	})
	if err != nil {
		t.Fatalf("failed to execute operation: %v", err)
	}
	if res.Status != "SUCCESS" {
		t.Errorf("expected SUCCESS, got %s", res.Status)
	}

	// 3. Execute with missing required parameter -> should fail validation
	_, err = engine.ExecuteOperation(ctx, "calculate_salary", map[string]any{
		"bonus_multiplier": 2.0,
	})
	if err == nil {
		t.Fatal("expected error on missing required employee_id parameter, got nil")
	}
}

func TestTransactionLifecycle(t *testing.T) {
	ctx := context.Background()
	mockAdapter := adapter.NewMockAdapter()

	proj, err := project.NewProject(project.ProjectConfig{
		Name: "Tx Project",
		AdapterConfig: project.AdapterConfig{
			AdapterType: "memory",
			Database:    "tx_db",
		},
	}, mockAdapter)
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}
	engine := proj.Engine

	// Setup model
	_, _ = engine.CreateModelConfig(ctx, &model.ModelConfig{
		ID:     "account",
		Name:   "Account",
		Status: model.ModelConfigStatusActive,
	})
	_, _ = engine.AddDataModel(ctx, &model.DataModel{
		ModelID: "account", ColumnName: "id", DataType: model.TypeLong, IsPrimaryKey: true,
	})
	_, _ = engine.AddDataModel(ctx, &model.DataModel{
		ModelID: "account", ColumnName: "balance", DataType: model.TypeDecimal,
	})
	_, _ = engine.ApplySchema(ctx, "account", service.ApplyRequest{})

	// 1. Successful Transaction
	err = engine.Transaction(ctx, func(tx adapter.Transaction) error {
		ref := model.ModelRef{Name: "Account", StorageName: "account"}
		_, err := tx.Create(ctx, ref, map[string]any{"id": int64(1), "balance": 500.0})
		if err != nil {
			return err
		}
		_, err = tx.Create(ctx, ref, map[string]any{"id": int64(2), "balance": 1000.0})
		return err
	})
	if err != nil {
		t.Fatalf("expected transaction to succeed: %v", err)
	}

	// 2. Failing Transaction (auto-rollback)
	err = engine.Transaction(ctx, func(tx adapter.Transaction) error {
		ref := model.ModelRef{Name: "Account", StorageName: "account"}
		_, _ = tx.Create(ctx, ref, map[string]any{"id": int64(3), "balance": 200.0})
		return fmt.Errorf("simulated business rule violation")
	})
	if err == nil {
		t.Fatal("expected transaction error, got nil")
	}
}

