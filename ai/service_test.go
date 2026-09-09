package ai

import (
	"context"
	"strings"
	"testing"

	"github.com/SanjayDrop5528/models-go-engine/model"
	"github.com/SanjayDrop5528/models-go-engine/registry"
)

type mockClient struct {
	response    *ChatResponse
	err         error
	capturedReq *ChatRequest
}

func (m *mockClient) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	m.capturedReq = req
	if m.err != nil {
		return nil, m.err
	}
	return m.response, nil
}

func TestPlanToDataSet(t *testing.T) {
	reg := registry.NewModelRegistry()
	_, _ = reg.SaveModelConfig(&model.ModelConfig{
		ID:     "spares_stores",
		Name:   "spares_stores",
		Table:  "stores",
		Schema: "spares",
	})

	svc := NewAIService(nil, reg, nil)

	plan := &AIQueryPlan{
		BaseModel: "stores",
		Schema:    "spares",
		Joins: []AIJoin{
			{
				FromTable: "stores",
				FromField: "id",
				ToTable:   "store_spares",
				ToField:   "store_id",
				NamedAs:   "store_spares",
				JoinType:  "LEFT",
			},
		},
		Filters: []AIFilter{
			{
				Table:    "stores",
				Field:    "status",
				Operator: "=",
				Value:    "active",
			},
		},
		Aggregations: []AIAggregate{
			{
				Name:       "total_capacity",
				Function:   "SUM",
				Table:      "stores",
				Field:      "capacity",
				GroupByKey: "location",
			},
		},
		GroupBy: []AIGroupBy{
			{
				Table: "stores",
				Field: "location",
			},
		},
		Explanation: "Show active stores total capacity by location",
	}

	ds, err := svc.PlanToDataSet(plan, "postgres")
	if err != nil {
		t.Fatalf("unexpected error converting plan: %v", err)
	}

	if ds.BaseCollection.Collection != "stores" {
		t.Errorf("expected collection stores, got %s", ds.BaseCollection.Collection)
	}
	if ds.BaseCollection.Schema != "spares" {
		t.Errorf("expected schema spares, got %s", ds.BaseCollection.Schema)
	}
	if len(ds.JoinCollections) != 1 {
		t.Fatalf("expected 1 join, got %d", len(ds.JoinCollections))
	}
	if ds.JoinCollections[0].ToCollection != "store_spares" {
		t.Errorf("expected join to store_spares, got %s", ds.JoinCollections[0].ToCollection)
	}
	if len(ds.CustomColumns) != 1 {
		t.Fatalf("expected 1 custom column, got %d", len(ds.CustomColumns))
	}
	if ds.CustomColumns[0].CustomAggregateFnName != "SUM" {
		t.Errorf("expected aggregate SUM, got %s", ds.CustomColumns[0].CustomAggregateFnName)
	}
	if len(ds.GroupByFields) != 1 || ds.GroupByFields[0].FieldName != "location" {
		t.Errorf("expected groupby field location, got %v", ds.GroupByFields)
	}
}

func TestExtractAIQueryPlanFenced(t *testing.T) {
	rawJSON := "```json\n{\n  \"base_model\": \"stores\",\n  \"schema\": \"spares\",\n  \"explanation\": \"Testing fenced block\"\n}\n```"
	plan, err := extractAIQueryPlan(rawJSON)
	if err != nil {
		t.Fatalf("failed parsing fenced json: %v", err)
	}
	if plan.BaseModel != "stores" {
		t.Errorf("expected stores, got %s", plan.BaseModel)
	}
}

func TestGenerateDataSetMockClient(t *testing.T) {
	mockResp := &ChatResponse{
		ConversationID: "conv-12345-abcde",
		Response: `{
  "base_model": "stores",
  "schema": "spares",
  "filters": [
    { "field": "status", "operator": "=", "value": "active" }
  ],
  "explanation": "Filtered active stores"
}`,
	}
	svc := NewAIService(&mockClient{response: mockResp}, registry.NewModelRegistry(), nil)

	resp, err := svc.GenerateDataSet(context.Background(), &GenerateRequest{
		Prompt:         "show active stores",
		ConversationID: "conv-12345-abcde",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ConversationID != "conv-12345-abcde" {
		t.Errorf("expected conv-12345-abcde, got %s", resp.ConversationID)
	}
	if resp.DataSet.BaseCollection.Collection != "stores" {
		t.Errorf("expected stores, got %s", resp.DataSet.BaseCollection.Collection)
	}
	if resp.DataSet.BaseCollection.Filter["status"] != "active" {
		t.Errorf("expected filter status active, got %v", resp.DataSet.BaseCollection.Filter)
	}
	if resp.DataSet.ConversationID != "conv-12345-abcde" {
		t.Errorf("expected DataSet.ConversationID to be conv-12345-abcde, got %s", resp.DataSet.ConversationID)
	}
}

func TestConversationIDAutoTrackingInEngine(t *testing.T) {
	mockResp := &ChatResponse{
		ConversationID: "engine-conv-9999",
		Response: `{
  "base_model": "stores",
  "schema": "spares",
  "explanation": "Active stores"
}`,
	}
	mock := &mockClient{response: mockResp}
	svc := NewAIService(mock, registry.NewModelRegistry(), nil)

	// Turn 1: No conversation ID provided by caller
	resp1, err := svc.GenerateDataSet(context.Background(), &GenerateRequest{
		Prompt: "first request without conversation id",
	})
	if err != nil {
		t.Fatalf("turn 1 failed: %v", err)
	}
	if resp1.ConversationID != "engine-conv-9999" {
		t.Errorf("turn 1 expected engine-conv-9999, got %s", resp1.ConversationID)
	}
	if resp1.DataSet.ConversationID != "engine-conv-9999" {
		t.Errorf("turn 1 expected DataSet.ConversationID engine-conv-9999, got %s", resp1.DataSet.ConversationID)
	}
	if svc.GetActiveConversationID() != "engine-conv-9999" {
		t.Errorf("expected engine memory to hold engine-conv-9999, got %s", svc.GetActiveConversationID())
	}

	// Turn 2: Caller sends another prompt with NO conversation ID — engine must automatically pass engine-conv-9999
	mock.capturedReq = nil
	resp2, err := svc.GenerateDataSet(context.Background(), &GenerateRequest{
		Prompt: "second request without conversation id",
	})
	if err != nil {
		t.Fatalf("turn 2 failed: %v", err)
	}
	if mock.capturedReq == nil || mock.capturedReq.ConversationID != "engine-conv-9999" {
		t.Errorf("expected client to receive conversation_id engine-conv-9999, got %v", mock.capturedReq)
	}
	if resp2.DataSet.ConversationID != "engine-conv-9999" {
		t.Errorf("turn 2 expected DataSet.ConversationID engine-conv-9999, got %s", resp2.DataSet.ConversationID)
	}

	// Reset conversation
	svc.ResetConversation()
	if svc.GetActiveConversationID() != "" {
		t.Errorf("expected active conversation ID to be empty after reset, got %s", svc.GetActiveConversationID())
	}
}

func TestTwoLayerMetadataDiscoveryAndEnrichedContext(t *testing.T) {
	reg := registry.NewModelRegistry()

	// 1. Setup ModelConfigs
	_, _ = reg.SaveModelConfig(&model.ModelConfig{
		ID:     "spares_stores",
		Name:   "stores",
		Table:  "stores",
		Schema: "spares",
	})
	_, _ = reg.SaveModelConfig(&model.ModelConfig{
		ID:     "spares_store_spares",
		Name:   "store_spares",
		Table:  "store_spares",
		Schema: "spares",
	})
	_, _ = reg.SaveModelConfig(&model.ModelConfig{
		ID:     "spares_spares",
		Name:   "spares",
		Table:  "spares",
		Schema: "spares",
	})
	// An unrelated model that shouldn't be matched
	_, _ = reg.SaveModelConfig(&model.ModelConfig{
		ID:     "hospital_doctors",
		Name:   "doctors",
		Table:  "doctors",
		Schema: "hospital",
	})

	// 2. Setup DataModel fields & relationships
	_, _ = reg.SaveDataModel(&model.DataModel{
		ModelID:      "spares_stores",
		ColumnName:   "id",
		DataType:     "UUID",
		IsPrimaryKey: true,
	})
	_, _ = reg.SaveDataModel(&model.DataModel{
		ModelID:    "spares_stores",
		ColumnName: "location",
		DataType:   "VARCHAR",
	})
	_, _ = reg.SaveDataModel(&model.DataModel{
		ModelID:    "spares_stores",
		ColumnName: "capacity",
		DataType:   "INTEGER",
	})

	_, _ = reg.SaveDataModel(&model.DataModel{
		ModelID:      "spares_store_spares",
		ColumnName:   "id",
		DataType:     "UUID",
		IsPrimaryKey: true,
	})
	_, _ = reg.SaveDataModel(&model.DataModel{
		ModelID:    "spares_store_spares",
		ColumnName: "store_id",
		DataType:   "UUID",
		Reference: &model.OrbitalRefSpec{
			Model:     "stores",
			Attribute: "id",
		},
	})
	_, _ = reg.SaveDataModel(&model.DataModel{
		ModelID:    "spares_store_spares",
		ColumnName: "spare_id",
		DataType:   "UUID",
		Reference: &model.OrbitalRefSpec{
			Model:     "spares",
			Attribute: "id",
		},
	})

	mock := &mockClient{
		response: &ChatResponse{
			Response: `{"base_model": "stores", "schema": "spares", "explanation": "Stores query"}`,
		},
	}
	svc := NewAIService(mock, reg, nil)

	// Layer 1 Test: User asks for stores and spares inventory
	prompt := "Show store capacity with store spares"
	discovered := svc.DiscoverRelatedTables(prompt, nil)
	if len(discovered) == 0 {
		t.Fatalf("expected discovered tables, got 0")
	}

	tableMap := make(map[string]bool)
	for _, d := range discovered {
		tableMap[d.Table] = true
	}

	if !tableMap["stores"] {
		t.Errorf("expected stores to be discovered")
	}
	if !tableMap["store_spares"] {
		t.Errorf("expected store_spares to be discovered via relationship traversal")
	}
	if tableMap["doctors"] {
		t.Errorf("unrelated table 'doctors' should NOT be discovered")
	}

	// Layer 2 Test: Schema context formulation
	enrichedCtx, tableNames := svc.BuildEnrichedSchemaContext(discovered)
	if len(tableNames) < 2 {
		t.Errorf("expected at least 2 table names, got %d", len(tableNames))
	}
	if !strings.Contains(enrichedCtx, "stores") {
		t.Errorf("expected enriched context to contain 'stores'")
	}
	if !strings.Contains(enrichedCtx, "store_spares.store_id = stores.id") {
		t.Errorf("expected enriched context to contain relationship 'store_spares.store_id = stores.id', got:\n%s", enrichedCtx)
	}

	// Full End-to-End Test with Mock
	res, err := svc.GenerateDataSet(context.Background(), &GenerateRequest{
		Prompt: prompt,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.DiscoveredTables) == 0 {
		t.Errorf("expected response to have DiscoveredTables")
	}
}


