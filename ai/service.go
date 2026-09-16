package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/SanjayDrop5528/models-go-engine/dataset/domain"
	datasetsvc "github.com/SanjayDrop5528/models-go-engine/dataset/service"
	"github.com/SanjayDrop5528/models-go-engine/model"
	"github.com/SanjayDrop5528/models-go-engine/registry"
)

// GenerateRequest is the input payload for the AI query generator.
type GenerateRequest struct {
	Prompt         string          `json:"prompt"`
	ConversationID string          `json:"conversation_id,omitempty"`
	Driver         string          `json:"driver,omitempty"`
	CurrentDataSet *domain.DataSet `json:"current_dataset,omitempty"`
	ExistingQuery  string          `json:"existing_query,omitempty"`
}

// GenerateResponse represents the result of the AI generation turn.
type GenerateResponse struct {
	ConversationID   string          `json:"conversation_id"`
	Explanation      string          `json:"explanation"`
	Plan             *AIQueryPlan    `json:"plan,omitempty"`
	DataSet          *domain.DataSet `json:"dataset"`
	SQLPreview       string          `json:"sql_preview,omitempty"`
	DiscoveredTables []string        `json:"discovered_tables,omitempty"` // Layer 1 discovered candidate tables
}

// DiscoveredColumnMeta holds the attributes of a column in a discovered table.
type DiscoveredColumnMeta struct {
	Name         string `json:"name"`
	DataType     string `json:"data_type"`
	IsPrimaryKey bool   `json:"is_primary_key"`
	IsNullable   bool   `json:"is_nullable"`
	IsForeignKey bool   `json:"is_foreign_key"`
	RefTable     string `json:"ref_table,omitempty"`
	RefColumn    string `json:"ref_column,omitempty"`
}

// TableRelationship describes a discovered link between two tables.
type TableRelationship struct {
	FromTable  string `json:"from_table"`
	FromColumn string `json:"from_column"`
	ToTable    string `json:"to_table"`
	ToColumn   string `json:"to_column"`
	RelType    string `json:"rel_type"` // "FOREIGN_KEY" or "CONVENTION"
}

// DiscoveredTableMeta represents a table discovered in Layer 1.
type DiscoveredTableMeta struct {
	Config         *model.ModelConfig     `json:"config"`
	Table          string                 `json:"table"`
	Schema         string                 `json:"schema"`
	Columns        []DiscoveredColumnMeta `json:"columns"`
	Relationships  []TableRelationship    `json:"relationships"`
	RelevanceScore int                    `json:"relevance_score"`
}

// AIService orchestrates schema retrieval, calling LLM with conversational memory, and compiling data sets.
type AIService struct {
	client               Client
	registry             *registry.ModelRegistry
	dataSetService       *datasetsvc.DataSetService
	activeConvMu         sync.RWMutex
	activeConversationID string
}

// NewAIService creates a new AIService.
func NewAIService(client Client, reg *registry.ModelRegistry, dsSvc *datasetsvc.DataSetService) *AIService {
	if client == nil {
		client = NewHTTPClient("", "")
	}
	return &AIService{
		client:         client,
		registry:       reg,
		dataSetService: dsSvc,
	}
}

// GetActiveConversationID returns the active conversation ID tracked by the engine.
func (s *AIService) GetActiveConversationID() string {
	s.activeConvMu.RLock()
	defer s.activeConvMu.RUnlock()
	return s.activeConversationID
}

// SetActiveConversationID stores the active conversation ID in the engine.
func (s *AIService) SetActiveConversationID(id string) {
	s.activeConvMu.Lock()
	defer s.activeConvMu.Unlock()
	s.activeConversationID = strings.TrimSpace(id)
}

// ResetConversation clears the conversation ID memory in the engine.
func (s *AIService) ResetConversation() {
	s.activeConvMu.Lock()
	defer s.activeConvMu.Unlock()
	s.activeConversationID = ""
}

// GenerateDataSet converts a natural language prompt into an executable DataSet, preserving conversation memory.
func (s *AIService) GenerateDataSet(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error) {
	if strings.TrimSpace(req.Prompt) == "" {
		return nil, fmt.Errorf("prompt cannot be empty")
	}

	driver := req.Driver
	if driver == "" {
		if req.CurrentDataSet != nil && req.CurrentDataSet.Driver != "" {
			driver = req.CurrentDataSet.Driver
		} else {
			driver = "postgres"
		}
	}

	// Resolve existing compiled query if present
	existingQuery := strings.TrimSpace(req.ExistingQuery)
	if existingQuery == "" && req.CurrentDataSet != nil {
		if req.CurrentDataSet.ReferencePipeline != "" {
			existingQuery = strings.TrimSpace(req.CurrentDataSet.ReferencePipeline)
		} else if req.CurrentDataSet.Pipeline != "" {
			existingQuery = strings.TrimSpace(req.CurrentDataSet.Pipeline)
		}
	}

	// 1. Layer 1: Discover all relevant meta-tables matching the user intent and their relationships
	searchPrompt := req.Prompt
	if existingQuery != "" {
		searchPrompt = req.Prompt + " " + existingQuery
	}
	discovered := s.DiscoverRelatedTables(ctx, searchPrompt, req.CurrentDataSet)

	// 2. Layer 2: Formulate enriched schema context containing those tables, all their attributes, and relationships
	schemaContext, discoveredTableNames := s.BuildEnrichedSchemaContext(discovered)

	// 3. Build system instructions with enriched Layer 2 context
	systemPrompt := s.buildSystemPrompt(schemaContext, driver)

	// 4. Resolve conversation ID from request, current dataset, or engine memory
	convID := strings.TrimSpace(req.ConversationID)
	if convID == "" && req.CurrentDataSet != nil {
		convID = strings.TrimSpace(req.CurrentDataSet.ConversationID)
	}
	if convID == "" {
		convID = s.GetActiveConversationID()
	}

	// 5. Build user prompt: if an existing query or dataset exists, pass both the SQL and structure so AI refines and adds to it
	userPrompt := req.Prompt
	hasExisting := existingQuery != "" || (req.CurrentDataSet != nil && req.CurrentDataSet.BaseCollection.Collection != "")
	if hasExisting {
		var promptBuilder strings.Builder
		promptBuilder.WriteString("==================== EXISTING QUERY & DATASET STATE ====================\n")
		promptBuilder.WriteString("The user already has an existing query / dataset that has been built and wants to REFINE or EXTEND it.\n")
		promptBuilder.WriteString("CRITICAL REFINEMENT INSTRUCTIONS:\n")
		promptBuilder.WriteString("1. PRESERVE all existing tables, selected columns, joins, group by dimensions, and filters from the existing query/dataset unless the user explicitly requested to remove or replace them.\n")
		promptBuilder.WriteString("2. Add newly requested columns, joins, filters, or calculations on top of the existing query.\n\n")

		if existingQuery != "" {
			promptBuilder.WriteString(fmt.Sprintf("EXISTING COMPILED SQL / PIPELINE:\n```sql\n%s\n```\n\n", existingQuery))
		}

		if req.CurrentDataSet != nil && req.CurrentDataSet.BaseCollection.Collection != "" {
			dsJSON, _ := json.MarshalIndent(req.CurrentDataSet, "", "  ")
			promptBuilder.WriteString(fmt.Sprintf("CURRENT DATASET CONFIGURATION (JSON):\n%s\n\n", string(dsJSON)))
		}
		promptBuilder.WriteString("========================================================================\n\n")
		promptBuilder.WriteString(fmt.Sprintf("USER REFINEMENT / MODIFICATION REQUEST:\n%s", req.Prompt))
		userPrompt = promptBuilder.String()
	}

	chatReq := &ChatRequest{
		ConversationID: convID,
		Messages: []ChatMessage{
			{
				Role:    "system",
				Content: systemPrompt,
			},
			{
				Role:    "user",
				Content: userPrompt,
			},
		},
		Effort:    "medium",
		SkipCache: false,
	}

	// 4. Call LLM
	chatResp, err := s.client.Chat(ctx, chatReq)
	if err != nil {
		return nil, fmt.Errorf("AI generation failed: %w", err)
	}

	// Persist active conversation ID inside the engine memory
	effectiveConvID := chatResp.ConversationID
	if effectiveConvID == "" {
		effectiveConvID = convID
	}
	if effectiveConvID != "" {
		s.SetActiveConversationID(effectiveConvID)
	}

	rawContent := ""
	if len(chatResp.Choices) > 0 {
		rawContent = chatResp.Choices[0].Message.Content
	}
	if rawContent == "" && chatResp.Response != "" {
		rawContent = chatResp.Response
	}

	// 5. Extract JSON and parse into AIQueryPlan
	plan, err := extractAIQueryPlan(rawContent)
	if err != nil {
		log.Printf("[AI Service] ⚠ JSON parse failed: %v. Raw text: %s", err, rawContent)
		return nil, fmt.Errorf("failed parsing AI query plan: %w", err)
	}

	// 6. Convert AIQueryPlan into executable domain.DataSet
	ds, err := s.PlanToDataSet(plan, driver)
	if err != nil {
		return nil, fmt.Errorf("failed converting AI plan to DataSet: %w", err)
	}

	// Preserve existing dataset identity if refining an existing dataset
	if req.CurrentDataSet != nil && req.CurrentDataSet.BaseCollection.Collection != "" {
		if req.CurrentDataSet.ID != "" {
			ds.ID = req.CurrentDataSet.ID
		}
		if req.CurrentDataSet.Name != "" {
			ds.Name = req.CurrentDataSet.Name
		}
		if req.CurrentDataSet.ReferenceName != "" {
			ds.ReferenceName = req.CurrentDataSet.ReferenceName
		}
		if req.CurrentDataSet.SaveMode != "" && plan.SaveMode == "" {
			ds.SaveMode = req.CurrentDataSet.SaveMode
		}
	}

	// Persist conversation ID directly in the DataSet model itself
	ds.ConversationID = effectiveConvID

	// 7. Compile live SQL preview and store the compiled query directly in the DataSet model
	sqlPreview := ""
	if s.dataSetService != nil {
		prevResp, prevErr := s.dataSetService.Preview(ctx, ds)
		if prevErr == nil && prevResp != nil {
			if prevResp.ReferencePipeline != "" {
				sqlPreview = prevResp.ReferencePipeline
			} else if prevResp.Pipeline != "" {
				sqlPreview = prevResp.Pipeline
			}
		}
	}

	// Ensure the DataSet model itself contains the compiled query and all metadata
	ds.Pipeline = sqlPreview
	ds.ReferencePipeline = sqlPreview

	explanation := plan.Explanation
	if explanation == "" {
		explanation = fmt.Sprintf("Generated query on '%s' with %d projected columns.", ds.BaseCollection.Collection, len(ds.SelectedList))
	}

	return &GenerateResponse{
		ConversationID:   effectiveConvID,
		Explanation:      explanation,
		Plan:             plan,
		DataSet:          ds,
		SQLPreview:       sqlPreview,
		DiscoveredTables: discoveredTableNames,
	}, nil
}

// PlanToDataSet transforms an AIQueryPlan into an executable domain.DataSet.
func (s *AIService) PlanToDataSet(plan *AIQueryPlan, driver string) (*domain.DataSet, error) {
	if plan == nil {
		return nil, fmt.Errorf("query plan is nil")
	}

	baseTable := strings.TrimSpace(plan.BaseModel)
	if baseTable == "" {
		return nil, fmt.Errorf("base_model cannot be empty")
	}

	// Pillar 1: Metadata Resolver - Resolve schema and model config from registry
	baseSchema := plan.Schema
	if s.registry != nil {
		cfg, err := s.registry.GetModelConfig(baseTable)
		if err == nil && cfg != nil {
			// Pillar 3: Permission Validator
			if strings.EqualFold(string(cfg.Status), string(model.ModelConfigStatusInactive)) || strings.EqualFold(string(cfg.Status), string(model.ModelConfigStatusArchived)) {
				return nil, fmt.Errorf("permission denied: model '%s' is inactive", baseTable)
			}
			if baseSchema == "" && cfg.Schema != "" {
				baseSchema = cfg.Schema
			}
			baseTable = cfg.Table
		}
	}
	if baseSchema == "" {
		baseSchema = "public"
	}

	now := time.Now().UTC()
	ds := &domain.DataSet{
		ID:            fmt.Sprintf("ds_ai_%d", now.UnixNano()),
		Name:          fmt.Sprintf("AI Pipeline: %s", baseTable),
		ReferenceName: strings.ToLower(fmt.Sprintf("%s_query", baseTable)),
		Driver:        driver,
		SaveMode:      domain.SaveModeProcedure,
		Status:        "ACTIVE",
		CreatedAt:     now,
		UpdatedAt:     now,
		BaseCollection: domain.BaseCollection{
			Collection: baseTable,
			Schema:     baseSchema,
		},
	}
	if plan.SaveMode != "" {
		ds.SaveMode = domain.SaveMode(plan.SaveMode)
	}

	// Pillar 2: Relationship Graph - Convert Joins & Auto-resolve Join Conditions
	for _, j := range plan.Joins {
		fromTbl := j.FromTable
		if fromTbl == "" {
			fromTbl = baseTable
		}
		toTbl := j.ToTable
		jt := domain.JoinLeft
		if j.JoinType != "" {
			jt = domain.JoinType(strings.ToUpper(j.JoinType))
		}
		namedAs := j.NamedAs
		if namedAs == "" {
			namedAs = toTbl
		}
		schema := j.Schema
		if schema == "" && s.registry != nil {
			if cfg, err := s.registry.GetModelConfig(toTbl); err == nil && cfg != nil {
				// Permission Validator
				if strings.EqualFold(string(cfg.Status), string(model.ModelConfigStatusInactive)) || strings.EqualFold(string(cfg.Status), string(model.ModelConfigStatusArchived)) {
					return nil, fmt.Errorf("permission denied: joined model '%s' is inactive", toTbl)
				}
				if cfg.Schema != "" {
					schema = cfg.Schema
				}
				toTbl = cfg.Table
			}
		}

		// Auto-resolve join keys from Relationship Graph if omitted or empty
		fromField := strings.TrimSpace(j.FromField)
		toField := strings.TrimSpace(j.ToField)
		if fromField == "" || toField == "" {
			resFrom, resTo := s.resolveJoinPath(fromTbl, toTbl)
			if fromField == "" {
				fromField = resFrom
			}
			if toField == "" {
				toField = resTo
			}
		}

		convertToString := j.ConvertToString
		castMode := j.CastMode
		if castMode == "" {
			castMode = "BOTH"
		}

		// Metadata Resolver: Automatically inspect data types of both join columns for casting requirement
		if s.registry != nil {
			var fromType, toType string
			if dms := s.getFieldsForTable(fromTbl); len(dms) > 0 {
				for _, dm := range dms {
					if strings.EqualFold(dm.ColumnName, fromField) {
						fromType = string(dm.DataType)
						break
					}
				}
			}
			if dms := s.getFieldsForTable(toTbl); len(dms) > 0 {
				for _, dm := range dms {
					if strings.EqualFold(dm.ColumnName, toField) {
						toType = string(dm.DataType)
						break
					}
				}
			}

			// If data types exist and differ (e.g. UUID vs VARCHAR, INT vs VARCHAR), auto-enable casting
			if fromType != "" && toType != "" && !strings.EqualFold(fromType, toType) {
				convertToString = true
				if castMode == "" {
					castMode = "BOTH"
				}
			}
		}

		ds.JoinCollections = append(ds.JoinCollections, domain.JoinCollection{
			Schema:              schema,
			FromCollection:      fromTbl,
			FromCollectionField: fromField,
			ToCollection:        toTbl,
			ToCollectionField:   toField,
			NamedAs:             namedAs,
			JoinType:            jt,
			ConvertToString:     convertToString,
			CastMode:            castMode,
		})
	}

	// 2. Convert Filters
	filterMap := make(map[string]any)
	for _, f := range plan.Filters {
		fieldKey := f.Field
		if f.Table != "" && f.Table != baseTable {
			fieldKey = fmt.Sprintf("%s.%s", f.Table, f.Field)
		}
		op := strings.ToLower(strings.TrimSpace(f.Operator))
		fVal := normalizeFilterValue(f.Value)
		switch op {
		case "!=", "<>":
			filterMap[fieldKey] = map[string]any{"$ne": fVal}
		case ">":
			filterMap[fieldKey] = map[string]any{"$gt": fVal}
		case ">=":
			filterMap[fieldKey] = map[string]any{"$gte": fVal}
		case "<":
			filterMap[fieldKey] = map[string]any{"$lt": fVal}
		case "<=":
			filterMap[fieldKey] = map[string]any{"$lte": fVal}
		case "like", "contains":
			filterMap[fieldKey] = map[string]any{"$regex": fVal}
		case "in":
			filterMap[fieldKey] = map[string]any{"$in": fVal}
		case "is_null":
			filterMap[fieldKey] = nil
		default:
			filterMap[fieldKey] = fVal
		}
	}
	if len(filterMap) > 0 {
		ds.BaseCollection.Filter = filterMap
	}

	// 3. Convert GroupBy
	for _, g := range plan.GroupBy {
		tbl := g.Table
		if tbl == "" {
			tbl = baseTable
		}
		var dt string
		if dms := s.getFieldsForTable(tbl); len(dms) > 0 {
			for _, dm := range dms {
				if strings.EqualFold(dm.ColumnName, g.Field) {
					dt = string(dm.DataType)
					break
				}
			}
		}
		ds.GroupByFields = append(ds.GroupByFields, domain.GroupByField{
			TableName: tbl,
			FieldName: g.Field,
			Name:      g.Field,
			DataType:  dt,
		})
	}

	// 4. Convert Aggregations
	for _, agg := range plan.Aggregations {
		colName := agg.Name
		if colName == "" {
			colName = fmt.Sprintf("%s_%s", strings.ToLower(agg.Function), agg.Field)
		}
		srcTable := agg.Table
		if srcTable == "" {
			srcTable = baseTable
		}
		fnName := strings.ToUpper(agg.Function)
		if fnName == "" {
			fnName = "SUM"
		}

		customCol := domain.CustomColumn{
			CustomColumnName:      colName,
			CustomLabelName:       colName,
			CustomAggregateFnName: fnName,
			Type:                  "decimal",
			Fields: []domain.DataSetCustomField{
				{
					Name:      agg.Field,
					TableName: srcTable,
					FieldName: agg.Field,
					Type:      "decimal",
				},
			},
		}
		ds.CustomColumns = append(ds.CustomColumns, customCol)
	}

	// 5. Convert Projections (SelectedList)
	if len(plan.Select) > 0 {
		for _, sCol := range plan.Select {
			tbl := sCol.Table
			if tbl == "" {
				tbl = baseTable
			}
			fKey := fmt.Sprintf("%s.%s", tbl, sCol.Field)
			hName := sCol.HeaderName
			if hName == "" {
				hName = sCol.Field
			}
			dt := sCol.DataType
			if dt == "" {
				if dms := s.getFieldsForTable(tbl); len(dms) > 0 {
					for _, dm := range dms {
						if strings.EqualFold(dm.ColumnName, sCol.Field) {
							dt = string(dm.DataType)
							break
						}
					}
				}
			}
			if dt == "" {
				dt = "string"
			}
			ds.SelectedList = append(ds.SelectedList, domain.SelectedField{
				Field:      fKey,
				HeaderName: hName,
				DataType:   dt,
			})
		}
	} else {
		// Default projection: Group by fields + aggregation columns
		for _, g := range ds.GroupByFields {
			dt := g.DataType
			if dt == "" {
				dt = "string"
			}
			ds.SelectedList = append(ds.SelectedList, domain.SelectedField{
				Field:      fmt.Sprintf("%s.%s", g.TableName, g.FieldName),
				HeaderName: g.FieldName,
				DataType:   dt,
			})
		}
		for _, agg := range ds.CustomColumns {
			ds.SelectedList = append(ds.SelectedList, domain.SelectedField{
				Field:      agg.CustomColumnName,
				HeaderName: agg.CustomColumnName,
				DataType:   agg.Type,
			})
		}
	}

	// 6. Convert Runtime Parameters
	for _, p := range plan.FilterParams {
		ds.FilterParams = append(ds.FilterParams, domain.FilterParam{
			ParamName:     p.Name,
			ParamDataType: p.DataType,
		})
	}

	return ds, nil
}

// DiscoverRelatedTables (Layer 1): Sends all meta table names to the AI to identify relevant tables,
// and traverses relational links (FKs, Orbital References, abbreviations) to gather all connected meta-tables.
func (s *AIService) DiscoverRelatedTables(ctx context.Context, prompt string, currentDS *domain.DataSet) []*DiscoveredTableMeta {
	if s.registry == nil {
		return nil
	}

	configs := s.registry.ListModelConfigs()
	if len(configs) == 0 {
		return nil
	}

	keywords := extractPromptKeywords(prompt)
	fieldsByTable := make(map[string][]*model.DataModel)
	cfgByTable := make(map[string]*model.ModelConfig)
	var tableCatalog strings.Builder

	for _, cfg := range configs {
		tLower := strings.ToLower(cfg.Table)
		cfgByTable[tLower] = cfg
		cfgByTable[strings.ToLower(cfg.ID)] = cfg
		cfgByTable[fmt.Sprintf("%s.%s", strings.ToLower(cfg.Schema), tLower)] = cfg

		fields := s.registry.ListDataModels(cfg.ID)
		if len(fields) == 0 {
			fields = s.registry.ListDataModels(cfg.Table)
		}
		fieldsByTable[tLower] = fields

		tableCatalog.WriteString(fmt.Sprintf("- %s.%s\n", cfg.Schema, cfg.Table))
	}

	selectedSet := make(map[string]*model.ModelConfig)
	scores := make(map[string]int)

	// Step 1: Send ALL meta table names to AI to discover relevant tables
	if s.client != nil {
		discoveryPrompt := fmt.Sprintf(`You are an expert database metadata routing assistant.
Below is the list of ALL available database tables in the catalog.
Analyze the user's natural language request (even with typos, abbreviations, or informal phrasing).
Select all relevant tables needed to answer the query, including related tables required for JOINs (e.g. attendance records link to employees, orders link to customers).

ALL AVAILABLE DATABASE TABLES:
%s

USER REQUEST:
"%s"

Return strictly a JSON object:
{
  "relevant_tables": ["<schema.table or table_name>", ...]
}`, tableCatalog.String(), prompt)

		discReq := &ChatRequest{
			Messages: []ChatMessage{
				{Role: "system", Content: "You are a database metadata routing assistant. Return only valid JSON."},
				{Role: "user", Content: discoveryPrompt},
			},
			Effort: "low",
		}

		if discResp, err := s.client.Chat(ctx, discReq); err == nil {
			raw := discResp.Response
			if len(discResp.Choices) > 0 {
				raw = discResp.Choices[0].Message.Content
			}
			clean := cleanJSONBlock(raw)
			var parsed struct {
				RelevantTables []string `json:"relevant_tables"`
			}
			if jsonErr := json.Unmarshal([]byte(clean), &parsed); jsonErr == nil {
				for _, t := range parsed.RelevantTables {
					tClean := strings.ToLower(strings.TrimSpace(t))
					if cfg, ok := cfgByTable[tClean]; ok {
						selectedSet[strings.ToLower(cfg.Table)] = cfg
						scores[strings.ToLower(cfg.Table)] = 100
					} else if idx := strings.Index(tClean, "."); idx >= 0 {
						tblOnly := tClean[idx+1:]
						if cfg, ok := cfgByTable[tblOnly]; ok {
							selectedSet[strings.ToLower(cfg.Table)] = cfg
							scores[strings.ToLower(cfg.Table)] = 100
						}
					}
				}
			}
		}
	}

	// Current dataset base table and joined tables receive strong anchor weights
	if currentDS != nil {
		if currentDS.BaseCollection.Collection != "" {
			cCol := strings.ToLower(currentDS.BaseCollection.Collection)
			if cfg, ok := cfgByTable[cCol]; ok {
				selectedSet[strings.ToLower(cfg.Table)] = cfg
				scores[strings.ToLower(cfg.Table)] = 150
			}
		}
		for _, j := range currentDS.JoinCollections {
			if j.ToCollection != "" {
				jCol := strings.ToLower(j.ToCollection)
				if cfg, ok := cfgByTable[jCol]; ok {
					selectedSet[strings.ToLower(cfg.Table)] = cfg
					scores[strings.ToLower(cfg.Table)] = 140
				}
			}
			if j.FromCollection != "" {
				jCol := strings.ToLower(j.FromCollection)
				if cfg, ok := cfgByTable[jCol]; ok {
					selectedSet[strings.ToLower(cfg.Table)] = cfg
					scores[strings.ToLower(cfg.Table)] = 140
				}
			}
		}
	}

	// Step 2: Also run keyword scoring as safety net & supplement
	type scoredTable struct {
		cfg   *model.ModelConfig
		score int
	}
	var scoredList []scoredTable

	for _, cfg := range configs {
		tLower := strings.ToLower(cfg.Table)
		score := 0
		fields := fieldsByTable[tLower]

		for _, kw := range keywords {
			singKw := singularize(kw)
			singTbl := singularize(tLower)

			if singKw == singTbl || kw == tLower {
				score += 60
			} else if strings.Contains(tLower, kw) || strings.Contains(kw, tLower) {
				score += 30
			}

			if strings.EqualFold(cfg.Schema, kw) {
				score += 10
			}

			// Score on column/attribute matches
			for _, f := range fields {
				fCol := strings.ToLower(f.ColumnName)
				if fCol == kw || singularize(fCol) == singKw {
					score += 25
				} else if strings.Contains(fCol, kw) {
					score += 12
				}
			}
		}

		if score > 0 {
			scoredList = append(scoredList, scoredTable{cfg: cfg, score: score})
		}
	}

	sort.Slice(scoredList, func(i, j int) bool {
		return scoredList[i].score > scoredList[j].score
	})

	for i := 0; i < len(scoredList) && (len(selectedSet) < 4 || i < 2); i++ {
		tbl := strings.ToLower(scoredList[i].cfg.Table)
		if _, exists := selectedSet[tbl]; !exists {
			selectedSet[tbl] = scoredList[i].cfg
			scores[tbl] = scoredList[i].score
		}
	}

	if len(selectedSet) == 0 {
		for i, cfg := range configs {
			if i >= 3 {
				break
			}
			tbl := strings.ToLower(cfg.Table)
			selectedSet[tbl] = cfg
			scores[tbl] = 1
		}
	}

	// Step 3: Relational Graph Traversal (Expand with related tables & common abbreviations)
	commonAbbr := map[string]string{
		"emp":  "employees",
		"dept": "departments",
		"org":  "organizations",
		"cat":  "categories",
		"usr":  "users",
		"cust": "customers",
		"prod": "products",
	}

	for tbl := range selectedSet {
		for _, f := range fieldsByTable[tbl] {
			var targetTbl string
			if f.Reference != nil && f.Reference.Model != "" {
				targetTbl = strings.ToLower(f.Reference.Model)
			} else if f.IsOrbitalReference && f.OrbitalReferenceModelID != nil {
				targetTbl = strings.ToLower(*f.OrbitalReferenceModelID)
			} else if strings.HasSuffix(strings.ToLower(f.ColumnName), "_id") {
				stem := strings.TrimSuffix(strings.ToLower(f.ColumnName), "_id")
				if cfg, exists := cfgByTable[pluralize(stem)]; exists {
					targetTbl = strings.ToLower(cfg.Table)
				} else if cfg, exists := cfgByTable[stem]; exists {
					targetTbl = strings.ToLower(cfg.Table)
				} else if mapped, hasAbbr := commonAbbr[stem]; hasAbbr {
					if cfg, exists := cfgByTable[mapped]; exists {
						targetTbl = strings.ToLower(cfg.Table)
					}
				}
			}

			if targetTbl != "" {
				if targetCfg, exists := cfgByTable[targetTbl]; exists {
					if _, already := selectedSet[targetTbl]; !already && len(selectedSet) < 12 {
						selectedSet[targetTbl] = targetCfg
						scores[targetTbl] = 20
					}
				}
			}
		}
	}

	// Incoming references
	for _, cfg := range configs {
		tLower := strings.ToLower(cfg.Table)
		if _, already := selectedSet[tLower]; already {
			continue
		}
		for _, f := range fieldsByTable[tLower] {
			var targetTbl string
			if f.Reference != nil && f.Reference.Model != "" {
				targetTbl = strings.ToLower(f.Reference.Model)
			} else if f.IsOrbitalReference && f.OrbitalReferenceModelID != nil {
				targetTbl = strings.ToLower(*f.OrbitalReferenceModelID)
			} else if strings.HasSuffix(strings.ToLower(f.ColumnName), "_id") {
				stem := strings.TrimSuffix(strings.ToLower(f.ColumnName), "_id")
				if refCfg, exists := cfgByTable[pluralize(stem)]; exists {
					targetTbl = strings.ToLower(refCfg.Table)
				} else if mapped, hasAbbr := commonAbbr[stem]; hasAbbr {
					if refCfg, exists := cfgByTable[mapped]; exists {
						targetTbl = strings.ToLower(refCfg.Table)
					}
				}
			}
			if targetTbl != "" {
				if _, isTargetSelected := selectedSet[targetTbl]; isTargetSelected {
					matchesKeyword := false
					for _, kw := range keywords {
						if strings.Contains(tLower, kw) || strings.Contains(kw, tLower) {
							matchesKeyword = true
							break
						}
					}
					if (matchesKeyword || len(selectedSet) < 4) && len(selectedSet) < 12 {
						selectedSet[tLower] = cfg
						scores[tLower] = 30
						break
					}
				}
			}
		}
	}

	// 4. Assemble DiscoveredTableMeta with full attributes & relationships
	var results []*DiscoveredTableMeta
	for tbl, cfg := range selectedSet {
		meta := &DiscoveredTableMeta{
			Config:         cfg,
			Table:          cfg.Table,
			Schema:         cfg.Schema,
			RelevanceScore: scores[tbl],
		}
		if meta.Schema == "" {
			meta.Schema = "public"
		}

		fields := fieldsByTable[tbl]
		for _, f := range fields {
			col := DiscoveredColumnMeta{
				Name:         f.ColumnName,
				DataType:     string(f.DataType),
				IsPrimaryKey: f.IsPrimaryKey,
				IsNullable:   f.IsNullable,
			}
			if f.Reference != nil && f.Reference.Model != "" {
				col.IsForeignKey = true
				col.RefTable = f.Reference.Model
				col.RefColumn = f.Reference.Attribute
				if col.RefColumn == "" {
					col.RefColumn = "id"
				}
			} else if f.IsOrbitalReference && f.OrbitalReferenceModelID != nil {
				col.IsForeignKey = true
				col.RefTable = *f.OrbitalReferenceModelID
				col.RefColumn = "id"
				if f.OrbitalReferenceFieldID != nil && *f.OrbitalReferenceFieldID != "" {
					col.RefColumn = *f.OrbitalReferenceFieldID
				}
			} else if strings.HasSuffix(strings.ToLower(f.ColumnName), "_id") {
				stem := strings.TrimSuffix(strings.ToLower(f.ColumnName), "_id")
				var refCfg *model.ModelConfig
				if c, exists := cfgByTable[pluralize(stem)]; exists {
					refCfg = c
				} else if c, exists := cfgByTable[stem]; exists {
					refCfg = c
				} else if mapped, hasAbbr := commonAbbr[stem]; hasAbbr {
					if c, exists := cfgByTable[mapped]; exists {
						refCfg = c
					}
				}
				if refCfg != nil {
					col.IsForeignKey = true
					col.RefTable = refCfg.Table
					col.RefColumn = "id"
				}
			}
			meta.Columns = append(meta.Columns, col)
		}
		results = append(results, meta)
	}

	// Sort results by score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].RelevanceScore > results[j].RelevanceScore
	})

	// Find all cross-table relationships between the discovered tables
	discoveredMap := make(map[string]bool)
	for _, dm := range results {
		discoveredMap[strings.ToLower(dm.Table)] = true
	}

	for _, dm := range results {
		fromTbl := dm.Table
		for _, col := range dm.Columns {
			if col.IsForeignKey && col.RefTable != "" {
				toTbl := col.RefTable
				if discoveredMap[strings.ToLower(toTbl)] {
					rel := TableRelationship{
						FromTable:  fromTbl,
						FromColumn: col.Name,
						ToTable:    toTbl,
						ToColumn:   col.RefColumn,
						RelType:    "FOREIGN_KEY",
					}
					dm.Relationships = append(dm.Relationships, rel)
				}
			}
		}
	}

	return results
}

// BuildEnrichedSchemaContext (Layer 2): Takes discovered candidate tables and formats their complete attributes,
// types, keys, and explicit join relationships for the AI prompt.
func (s *AIService) BuildEnrichedSchemaContext(discovered []*DiscoveredTableMeta) (string, []string) {
	if len(discovered) == 0 {
		return "No relevant database tables identified.", nil
	}

	var sb strings.Builder
	sb.WriteString("LAYER 1 DISCOVERED DATABASE TABLES & ATTRIBUTES:\n")
	sb.WriteString("================================================\n")

	tableNames := make([]string, 0, len(discovered))
	seenRels := make(map[string]bool)
	var allRels []string

	for _, dt := range discovered {
		tableNames = append(tableNames, dt.Table)
		sb.WriteString(fmt.Sprintf("\nTable: %s.%s (Alias: %s)\n", dt.Schema, dt.Table, dt.Table))
		sb.WriteString("Columns:\n")
		for _, c := range dt.Columns {
			flags := make([]string, 0)
			if c.IsPrimaryKey {
				flags = append(flags, "PK")
			}
			if c.IsForeignKey && c.RefTable != "" {
				flags = append(flags, fmt.Sprintf("FK -> %s.%s", c.RefTable, c.RefColumn))
			}
			flagStr := ""
			if len(flags) > 0 {
				flagStr = fmt.Sprintf(" [%s]", strings.Join(flags, ", "))
			}
			sb.WriteString(fmt.Sprintf("  - %s (%s)%s\n", c.Name, c.DataType, flagStr))
		}

		for _, r := range dt.Relationships {
			key := fmt.Sprintf("%s.%s = %s.%s", r.FromTable, r.FromColumn, r.ToTable, r.ToColumn)
			if !seenRels[key] {
				seenRels[key] = true
				allRels = append(allRels, key)
			}
		}
	}

	if len(allRels) > 0 {
		sb.WriteString("\nDISCOVERED TABLE RELATIONSHIPS (Use for JOINs):\n")
		sb.WriteString("===============================================\n")
		for _, rel := range allRels {
			sb.WriteString(fmt.Sprintf("- %s\n", rel))
		}
	}

	return sb.String(), tableNames
}

var commonStopWords = map[string]bool{
	"the": true, "a": true, "an": true, "of": true, "in": true, "to": true, "for": true, "with": true,
	"on": true, "at": true, "from": true, "by": true, "about": true, "as": true, "into": true,
	"like": true, "through": true, "after": true, "over": true, "between": true, "out": true,
	"against": true, "during": true, "without": true, "before": true, "under": true, "around": true,
	"among": true, "show": true, "get": true, "find": true, "list": true, "select": true,
	"display": true, "fetch": true, "all": true, "total": true, "sum": true, "count": true,
	"average": true, "avg": true, "max": true, "min": true, "greater": true, "than": true,
	"less": true, "more": true, "where": true, "and": true, "or": true, "group": true,
	"is": true, "are": true, "was": true, "were": true, "be": true, "been": true, "also": true,
	"having": true, "order": true, "please": true, "query": true, "data": true,
}

func extractPromptKeywords(prompt string) []string {
	f := func(c rune) bool {
		return !unicode.IsLetter(c) && !unicode.IsNumber(c)
	}
	words := strings.FieldsFunc(strings.ToLower(prompt), f)
	var keywords []string
	seen := make(map[string]bool)
	for _, w := range words {
		w = strings.TrimSpace(w)
		if len(w) >= 2 && !commonStopWords[w] && !seen[w] {
			seen[w] = true
			keywords = append(keywords, w)
		}
	}
	return keywords
}

func singularize(w string) string {
	w = strings.ToLower(strings.TrimSpace(w))
	if strings.HasSuffix(w, "ies") && len(w) > 3 {
		return strings.TrimSuffix(w, "ies") + "y"
	}
	if strings.HasSuffix(w, "es") && len(w) > 3 {
		return strings.TrimSuffix(w, "es")
	}
	if strings.HasSuffix(w, "s") && len(w) > 2 {
		return strings.TrimSuffix(w, "s")
	}
	return w
}

func pluralize(w string) string {
	w = strings.ToLower(strings.TrimSpace(w))
	if strings.HasSuffix(w, "y") && len(w) > 2 {
		return strings.TrimSuffix(w, "y") + "ies"
	}
	if strings.HasSuffix(w, "s") {
		return w
	}
	return w + "s"
}

func (s *AIService) buildSystemPrompt(schemaContext, driver string) string {
	return fmt.Sprintf(`You are an expert SQL, Database and Data Engine architect specializing in turning Natural Language questions into structured Query Plans.
The target query database engine is: %s.

%s

YOUR TASK:
Analyze the user's natural language question or query refinement.
Produce a strictly valid JSON response matching the following AIQueryPlan structure. Do not output markdown or explanatory text outside the JSON.

JSON SCHEMA:
{
  "base_model": "<root table name>",
  "schema": "<schema name e.g. spares or public>",
  "select": [
    { "table": "<table name>", "field": "<column>", "header_name": "<alias>", "data_type": "<string|int|decimal|timestamp>" }
  ],
  "joins": [
    { "from_table": "<table 1>", "from_field": "<col 1>", "to_table": "<table 2>", "to_field": "<col 2>", "join_type": "LEFT|INNER", "convert_to_string": false, "cast_mode": "BOTH|FROM_ONLY|TO_ONLY" }
  ],
  "filters": [
    { "table": "<table name>", "field": "<column>", "operator": "=|!=|>|<|like|in|is_null", "value": <literal value>, "cast_as": "<optional sql type e.g. DATE|TEXT>" }
  ],
  "aggregations": [
    { "name": "<alias e.g. total_capacity>", "function": "SUM|AVG|COUNT|MIN|MAX", "table": "<table name>", "field": "<column>", "group_by_key": "<paired dimension column>" }
  ],
  "group_by": [
    { "table": "<table name>", "field": "<column>" }
  ],
  "filter_params": [
    { "name": "<param_name>", "data_type": "string|int|decimal|date|timestamp" }
  ],
  "driver": "%s",
  "save_mode": "PROCEDURE",
  "explanation": "<Short human-friendly summary of what query was generated>"
}

RULES:
1. ONLY use tables and columns present in the AVAILABLE DATABASE TABLES & FIELDS schema above.
2. DATA TYPES & CASTING:
   - Always pay attention to the column data types displayed in the schema (e.g. UUID, VARCHAR, INTEGER, TIMESTAMP).
   - In scenarios where joining columns have divergent or mismatched types (e.g. UUID to VARCHAR, or INTEGER to VARCHAR), set "convert_to_string": true and "cast_mode": "BOTH" so safe type-casting (CAST(... AS TEXT)) is performed.
   - For filter parameters and projections, assign the exact matching "data_type".
3. RELATIVE DATE FILTERS (Custom Options Macro):
   - For relative date or timestamp filters (e.g. "last 7 days", "past 30 days", "yesterday", "today"), use the dynamic macro syntax "C[<offset>]":
     * "last 7 days" -> operator: ">=", value: "C[-7d]"
     * "yesterday" / "past 1 day" -> operator: ">=", value: "C[-1d]"
     * "today" -> operator: ">=", value: "C[0d]"
     * "last 30 days" / "past month" -> operator: ">=", value: "C[-30d]" or "C[-1m]"
     * "last year" -> operator: ">=", value: "C[-1y]"
     * "next 7 days" -> operator: "<=", value: "C[+7d]"
   - The engine automatically compiles "C[-Nd]" into native SQL "(CURRENT_DATE - INTERVAL 'N days')".
4. If computing totals, averages, or counts across groups, add them to "aggregations" AND include the grouping columns in "group_by".
5. When joining tables, choose foreign key matches from the schema (e.g. stores.id = store_spares.store_id).
6. For multi-turn conversations, preserve existing selections and filters unless the user explicitly asks to replace or remove them.
7. Provide a clear, concise "explanation" summarizing the action taken.
8. Return ONLY the raw JSON object.`, driver, schemaContext, driver)
}

func cleanJSONBlock(content string) string {
	clean := strings.TrimSpace(content)
	if idx := strings.Index(clean, "```json"); idx != -1 {
		clean = clean[idx+7:]
		if endIdx := strings.Index(clean, "```"); endIdx != -1 {
			clean = clean[:endIdx]
		}
	} else if idx := strings.Index(clean, "```"); idx != -1 {
		clean = clean[idx+3:]
		if endIdx := strings.Index(clean, "```"); endIdx != -1 {
			clean = clean[:endIdx]
		}
	}
	return strings.TrimSpace(clean)
}

func extractAIQueryPlan(content string) (*AIQueryPlan, error) {
	clean := cleanJSONBlock(content)
	var plan AIQueryPlan
	if err := json.Unmarshal([]byte(clean), &plan); err != nil {
		return nil, fmt.Errorf("invalid json: %w (content: %s)", err, clean)
	}
	return &plan, nil
}

// normalizeFilterValue checks if the filter value is an interval expression and normalizes it to C[...] macro syntax.
func normalizeFilterValue(val any) any {
	if s, ok := val.(string); ok {
		sTrim := strings.TrimSpace(s)
		sLower := strings.ToLower(sTrim)
		// Check for expressions like now() - interval '7 days' or current_date - interval '7 days'
		if strings.Contains(sLower, "interval") {
			re := regexp.MustCompile(`(?i)(?:now\(\)|current_date|current_timestamp)\s*([+-])\s*interval\s*'(\d+)\s*([a-zA-Z]+)'`)
			if matches := re.FindStringSubmatch(sTrim); len(matches) == 4 {
				sign := matches[1]
				num := matches[2]
				unitRaw := strings.ToLower(matches[3])
				unit := "d"
				if strings.HasPrefix(unitRaw, "day") {
					unit = "d"
				} else if strings.HasPrefix(unitRaw, "month") {
					unit = "m"
				} else if strings.HasPrefix(unitRaw, "year") {
					unit = "y"
				} else if strings.HasPrefix(unitRaw, "hour") {
					unit = "h"
				} else if strings.HasPrefix(unitRaw, "week") {
					unit = "w"
				}
				return fmt.Sprintf("C[%s%s%s]", sign, num, unit)
			}
		}
		return sTrim
	}
	return val
}

// getFieldsForTable returns all DataModel fields for a given table name or model ID, resolving aliases and configs.
func (s *AIService) getFieldsForTable(tbl string) []*model.DataModel {
	if s.registry == nil {
		return nil
	}
	tbl = strings.TrimSpace(tbl)
	if tbl == "" {
		return nil
	}
	if cfg, err := s.registry.GetModelConfig(tbl); err == nil && cfg != nil {
		if fields := s.registry.ListDataModels(cfg.ID); len(fields) > 0 {
			return fields
		}
	}
	if fields := s.registry.ListDataModels(tbl); len(fields) > 0 {
		return fields
	}
	// Try without schema prefix (e.g. "iam.users" -> "users")
	if idx := strings.LastIndex(tbl, "."); idx != -1 {
		short := tbl[idx+1:]
		if cfg, err := s.registry.GetModelConfig(short); err == nil && cfg != nil {
			if fields := s.registry.ListDataModels(cfg.ID); len(fields) > 0 {
				return fields
			}
		}
		return s.registry.ListDataModels(short)
	}
	return nil
}

// resolveJoinPath (Relationship Graph):
// Determines the join keys between fromTable and toTable using the metadata registry's foreign keys,
// orbital references, and naming conventions.
func (s *AIService) resolveJoinPath(fromTable, toTable string) (string, string) {
	if s.registry == nil {
		return "id", fmt.Sprintf("%s_id", singularize(fromTable))
	}

	fromFields := s.getFieldsForTable(fromTable)
	toFields := s.getFieldsForTable(toTable)

	// Check if fromTable references toTable
	for _, f := range fromFields {
		if f.Reference != nil && strings.EqualFold(f.Reference.Model, toTable) {
			attr := f.Reference.Attribute
			if attr == "" {
				attr = "id"
			}
			return f.ColumnName, attr
		}
		if f.IsOrbitalReference && f.OrbitalReferenceModelID != nil && strings.EqualFold(*f.OrbitalReferenceModelID, toTable) {
			attr := "id"
			if f.OrbitalReferenceFieldID != nil && *f.OrbitalReferenceFieldID != "" {
				attr = *f.OrbitalReferenceFieldID
			}
			return f.ColumnName, attr
		}
		colLower := strings.ToLower(f.ColumnName)
		if strings.HasSuffix(colLower, "_id") {
			stem := strings.TrimSuffix(colLower, "_id")
			if strings.EqualFold(stem, singularize(toTable)) || strings.EqualFold(pluralize(stem), toTable) || (stem == "emp" && strings.HasPrefix(toTable, "employee")) {
				return f.ColumnName, "id"
			}
		}
	}

	// Check if toTable references fromTable
	for _, f := range toFields {
		if f.Reference != nil && strings.EqualFold(f.Reference.Model, fromTable) {
			attr := f.Reference.Attribute
			if attr == "" {
				attr = "id"
			}
			return attr, f.ColumnName
		}
		if f.IsOrbitalReference && f.OrbitalReferenceModelID != nil && strings.EqualFold(*f.OrbitalReferenceModelID, fromTable) {
			attr := "id"
			if f.OrbitalReferenceFieldID != nil && *f.OrbitalReferenceFieldID != "" {
				attr = *f.OrbitalReferenceFieldID
			}
			return attr, f.ColumnName
		}
		colLower := strings.ToLower(f.ColumnName)
		if strings.HasSuffix(colLower, "_id") {
			stem := strings.TrimSuffix(colLower, "_id")
			if strings.EqualFold(stem, singularize(fromTable)) || strings.EqualFold(pluralize(stem), fromTable) || (stem == "emp" && strings.HasPrefix(fromTable, "employee")) {
				return "id", f.ColumnName
			}
		}
	}

	return "id", fmt.Sprintf("%s_id", singularize(fromTable))
}
