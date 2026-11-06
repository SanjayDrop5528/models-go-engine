package dataset_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/SanjayDrop5528/models-go-engine/adapter"
	"github.com/SanjayDrop5528/models-go-engine/dataset/domain"
	"github.com/SanjayDrop5528/models-go-engine/dataset/repository"
	"github.com/SanjayDrop5528/models-go-engine/dataset/resolver"
	"github.com/SanjayDrop5528/models-go-engine/dataset/service"
	"github.com/SanjayDrop5528/models-go-engine/execution"
	"github.com/SanjayDrop5528/models-go-engine/model"
	"github.com/SanjayDrop5528/models-go-engine/query"
)

// mockQueryAdapter simulates database execution for raw queries
type mockQueryAdapter struct {
	adapter.Adapter
	lastTarget string
	rows       []map[string]any
	savedData  map[string]any
}

func (m *mockQueryAdapter) Name() string { return "postgres" }
func (m *mockQueryAdapter) Execute(ctx context.Context, req execution.ExecutionRequest) (*execution.ExecutionResult, error) {
	m.lastTarget = req.Target
	return &execution.ExecutionResult{
		Data:   m.rows,
		Status: "SUCCESS",
	}, nil
}
func (m *mockQueryAdapter) Create(ctx context.Context, mRef model.ModelRef, data map[string]any) (map[string]any, error) {
	m.savedData = data
	return data, nil
}
func (m *mockQueryAdapter) Update(ctx context.Context, mRef model.ModelRef, id any, data map[string]any) (map[string]any, error) {
	m.savedData = data
	return data, nil
}
func (m *mockQueryAdapter) FindOne(ctx context.Context, mRef model.ModelRef, id any) (map[string]any, error) {
	if m.savedData != nil {
		return m.savedData, nil
	}
	return nil, nil
}
func (m *mockQueryAdapter) Find(ctx context.Context, mRef model.ModelRef, q query.Query) ([]map[string]any, int64, error) {
	if m.savedData != nil {
		return []map[string]any{m.savedData}, 1, nil
	}
	return nil, 0, nil
}

func TestDataSet_UnmarshalJSON_PipelineMode(t *testing.T) {
	// Case 1: camelCase pipelineMode = "raw"
	rawJSON1 := `{"name":"Raw Query","reference_name":"raw_q1","pipelineMode":"raw","pipeline":"SELECT 1"}`
	var ds1 domain.DataSet
	if err := json.Unmarshal([]byte(rawJSON1), &ds1); err != nil {
		t.Fatalf("failed unmarshaling ds1: %v", err)
	}
	if ds1.PipelineMode != "raw" {
		t.Errorf("expected ds1.PipelineMode = 'raw', got: %s", ds1.PipelineMode)
	}
	if !ds1.IsRawDataset() {
		t.Errorf("expected ds1.IsRawDataset() = true")
	}

	// Case 2: snake_case pipeline_mode = "builder"
	rawJSON2 := `{"name":"Builder Query","reference_name":"b_q1","pipeline_mode":"builder","pipeline":"SELECT 1"}`
	var ds2 domain.DataSet
	if err := json.Unmarshal([]byte(rawJSON2), &ds2); err != nil {
		t.Fatalf("failed unmarshaling ds2: %v", err)
	}
	if ds2.PipelineMode != "builder" {
		t.Errorf("expected ds2.PipelineMode = 'builder', got: %s", ds2.PipelineMode)
	}
	if ds2.IsRawDataset() {
		t.Errorf("expected ds2.IsRawDataset() = false when PipelineMode is explicitly 'builder'")
	}

	// Case 3: legacy dataset_type = "RAW" maps to pipeline_mode = "raw"
	rawJSON3 := `{"name":"Legacy RAW Query","reference_name":"legacy_q","dataset_type":"RAW","pipeline":"SELECT 1"}`
	var ds3 domain.DataSet
	if err := json.Unmarshal([]byte(rawJSON3), &ds3); err != nil {
		t.Fatalf("failed unmarshaling ds3: %v", err)
	}
	if ds3.PipelineMode != "raw" {
		t.Errorf("expected ds3.PipelineMode = 'raw' from legacy dataset_type, got: %s", ds3.PipelineMode)
	}
}

func TestDataSetService_RawMode_PreservesPipelineAndBuildsColumnsFromConfig(t *testing.T) {
	repo := repository.NewDataSetRepository()
	fnRegistry := resolver.NewFunctionRegistry()
	modelResolver := resolver.NewModelResolver(nil)

	// Register sample model for type resolution
	modelResolver.RegisterModel(&resolver.ModelDefinition{
		Table: "employees",
		Fields: map[string]*resolver.FieldDefinition{
			"id":     {Name: "id", ColumnName: "id", DataType: "uuid", IsPrimaryKey: true},
			"salary": {Name: "salary", ColumnName: "salary", DataType: "decimal"},
		},
	})

	mockAdapter := &mockQueryAdapter{
		rows: []map[string]any{
			{
				"id":        "11111111-2222-3333-4444-555555555555",
				"salary":    75000.50,
				"bonus":     1500,
				"is_active": true,
				"note":      "Top performer",
			},
		},
	}

	svc := service.NewDataSetService(repo, modelResolver, modelResolver, fnRegistry, mockAdapter)

	userOriginalQuery := "SELECT id, salary, bonus, is_active, note FROM employees WHERE department_id = '{{department_id}}'"
	ds := &domain.DataSet{
		Name:              "Raw Employees Query",
		ReferenceName:     "raw_employees",
		PipelineMode:      "raw",
		Pipeline:          userOriginalQuery,
		ReferencePipeline: userOriginalQuery,
		Driver:            "postgres",
		SaveMode:          domain.SaveModeQuery,
		Status:            "ACTIVE",
		BaseCollection: domain.BaseCollection{
			Collection: "employees",
		},
		FilterParams: []domain.FilterParam{
			{
				ParamName:    "department_id",
				DefaultValue: "dep-100",
			},
		},
		// Intentionally omit SelectedList: raw mode should NOT require selected_list
		SelectedList: nil,
	}

	// 1. Preview Test
	ctx := context.Background()
	previewRes, err := svc.Preview(ctx, ds)
	if err != nil {
		t.Fatalf("Preview failed in raw mode: %v", err)
	}

	// Important check 1: query must NOT be overwritten with parameterized query
	if previewRes.Pipeline != userOriginalQuery {
		t.Errorf("expected Preview to preserve user's untouched query '%s', got '%s'", userOriginalQuery, previewRes.Pipeline)
	}
	if previewRes.ReferencePipeline != userOriginalQuery {
		t.Errorf("expected Preview to preserve user's untouched reference query '%s', got '%s'", userOriginalQuery, previewRes.ReferencePipeline)
	}

	// Important check 2: Columns must be populated from response rows + model config
	if len(previewRes.Columns) != 5 {
		t.Fatalf("expected 5 columns derived from rows, got %d", len(previewRes.Columns))
	}

	colMap := make(map[string]string)
	for _, col := range previewRes.Columns {
		colMap[col.Field] = col.DataType
	}

	// id must be "uuid" from model config
	if colMap["id"] != "uuid" {
		t.Errorf("expected 'id' column to have DataType 'uuid' from model config, got: %s", colMap["id"])
	}
	// salary must be "decimal" from model config
	if colMap["salary"] != "decimal" {
		t.Errorf("expected 'salary' column to have DataType 'decimal' from model config, got: %s", colMap["salary"])
	}
	// bonus was not in model config -> inferred as INTEGER or NUMBER
	if colMap["bonus"] != "INTEGER" && colMap["bonus"] != "NUMBER" {
		t.Errorf("expected 'bonus' column to have DataType 'INTEGER' or 'NUMBER', got: %s", colMap["bonus"])
	}
	// is_active was not in model config -> inferred as BOOLEAN
	if colMap["is_active"] != "BOOLEAN" {
		t.Errorf("expected 'is_active' column to have DataType 'BOOLEAN', got: %s", colMap["is_active"])
	}

	// 2. Save Test
	savedDS, err := svc.Save(ctx, ds)
	if err != nil {
		t.Fatalf("Save failed in raw mode: %v", err)
	}
	if savedDS.Pipeline != userOriginalQuery {
		t.Errorf("expected Save to preserve user's untouched query '%s', got '%s'", userOriginalQuery, savedDS.Pipeline)
	}
	if savedDS.PipelineMode != "raw" {
		t.Errorf("expected Save to preserve PipelineMode = 'raw', got '%s'", savedDS.PipelineMode)
	}
}

func TestAdapterDataSetRepository_PipelineModeRoundTrip(t *testing.T) {
	mockAdapter := &mockQueryAdapter{}
	adapterRepo := repository.NewAdapterDataSetRepository(mockAdapter)

	ds := &domain.DataSet{
		Name:              "Raw Dataset Adapter",
		ReferenceName:     "raw_adapter_ds",
		PipelineMode:      "raw",
		Pipeline:          "SELECT * FROM test",
		ReferencePipeline: "SELECT * FROM test",
		Driver:            "postgres",
		SaveMode:          domain.SaveModeQuery,
		Status:            "ACTIVE",
		BaseCollection: domain.BaseCollection{
			Collection: "test",
		},
	}

	ctx := context.Background()
	if err := adapterRepo.Save(ctx, ds); err != nil {
		t.Fatalf("failed saving to adapter repo: %v", err)
	}

	// Check underlying data map passed to adapter
	if mockAdapter.savedData == nil {
		t.Fatalf("expected adapter Create or Update to be called with data map")
	}
	if mockAdapter.savedData["pipeline_mode"] != "raw" {
		t.Errorf("expected savedData['pipeline_mode'] = 'raw', got: %v", mockAdapter.savedData["pipeline_mode"])
	}

	// Load back using FindByID
	loaded, err := adapterRepo.FindByID(ctx, ds.ID)
	if err != nil {
		t.Fatalf("failed loading from adapter repo: %v", err)
	}
	if loaded.PipelineMode != "raw" {
		t.Errorf("expected loaded.PipelineMode = 'raw', got: %s", loaded.PipelineMode)
	}
	if !loaded.IsRawDataset() {
		t.Errorf("expected loaded.IsRawDataset() = true")
	}
}
