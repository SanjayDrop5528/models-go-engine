package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
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

	// 1. Layer 1: Discover all relevant meta-tables matching the user intent and their relationships
	discovered := s.DiscoverRelatedTables(req.Prompt, req.CurrentDataSet)

	// 2. Layer 2: Formulate enriched schema context containing those tables, all their attributes, and relationships
	schemaContext, discoveredTableNames := s.BuildEnrichedSchemaContext(discovered)

	// 3. Build system instructions with enriched Layer 2 context
	systemPrompt := s.buildSystemPrompt(schemaContext, driver)

	// 3. Resolve conversation ID from request, current dataset, or engine memory
	convID := strings.TrimSpace(req.ConversationID)
	if convID == "" && req.CurrentDataSet != nil {
		convID = strings.TrimSpace(req.CurrentDataSet.ConversationID)
	}
	if convID == "" {
		convID = s.GetActiveConversationID()
	}

	userPrompt := req.Prompt
	if req.CurrentDataSet != nil && req.CurrentDataSet.BaseCollection.Collection != "" {
		dsJSON, _ := json.MarshalIndent(req.CurrentDataSet, "", "  ")
		userPrompt = fmt.Sprintf("Current DataSet State:\n%s\n\nUser Refinement Request:\n%s", string(dsJSON), req.Prompt)
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

	// Resolve schema from ModelRegistry if not specified
	baseSchema := plan.Schema
	if s.registry != nil {
		cfg, err := s.registry.GetModelConfig(baseTable)
		if err == nil && cfg != nil {
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

	// 1. Convert Joins
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
			if cfg, err := s.registry.GetModelConfig(toTbl); err == nil && cfg != nil && cfg.Schema != "" {
				schema = cfg.Schema
			}
		}

		ds.JoinCollections = append(ds.JoinCollections, domain.JoinCollection{
			Schema:              schema,
			FromCollection:      fromTbl,
			FromCollectionField: j.FromField,
			ToCollection:        toTbl,
			ToCollectionField:   j.ToField,
			NamedAs:             namedAs,
			JoinType:            jt,
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
		switch op {
		case "!=", "<>":
			filterMap[fieldKey] = map[string]any{"$ne": f.Value}
		case ">":
			filterMap[fieldKey] = map[string]any{"$gt": f.Value}
		case ">=":
			filterMap[fieldKey] = map[string]any{"$gte": f.Value}
		case "<":
			filterMap[fieldKey] = map[string]any{"$lt": f.Value}
		case "<=":
			filterMap[fieldKey] = map[string]any{"$lte": f.Value}
		case "like", "contains":
			filterMap[fieldKey] = map[string]any{"$regex": f.Value}
		case "in":
			filterMap[fieldKey] = map[string]any{"$in": f.Value}
		case "is_null":
			filterMap[fieldKey] = nil
		default:
			filterMap[fieldKey] = f.Value
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
		ds.GroupByFields = append(ds.GroupByFields, domain.GroupByField{
			TableName: tbl,
			FieldName: g.Field,
			Name:      g.Field,
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
			ds.SelectedList = append(ds.SelectedList, domain.SelectedField{
				Field:      fmt.Sprintf("%s.%s", g.TableName, g.FieldName),
				HeaderName: g.FieldName,
				DataType:   "string",
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

// DiscoverRelatedTables (Layer 1): Scans the model catalog, scores candidates against user keywords,
// and traverses relational links (FKs, Orbital References, name conventions) to find all connected meta-tables.
func (s *AIService) DiscoverRelatedTables(prompt string, currentDS *domain.DataSet) []*DiscoveredTableMeta {
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
	for _, cfg := range configs {
		tLower := strings.ToLower(cfg.Table)
		cfgByTable[tLower] = cfg
		cfgByTable[strings.ToLower(cfg.ID)] = cfg

		fields := s.registry.ListDataModels(cfg.ID)
		if len(fields) == 0 {
			fields = s.registry.ListDataModels(cfg.Table)
		}
		fieldsByTable[tLower] = fields
	}

	// 1. Score tables based on prompt relevance
	type scoredTable struct {
		cfg   *model.ModelConfig
		score int
	}
	var scoredList []scoredTable

	for _, cfg := range configs {
		tLower := strings.ToLower(cfg.Table)
		score := 0

		// Current dataset base table receives strong anchor weight
		if currentDS != nil && (strings.EqualFold(cfg.Table, currentDS.BaseCollection.Collection) || strings.EqualFold(cfg.ID, currentDS.BaseCollection.Collection)) {
			score += 150
		}

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

	// Sort scored list descending
	sort.Slice(scoredList, func(i, j int) bool {
		return scoredList[i].score > scoredList[j].score
	})

	// Select top candidate tables (up to 5 initial seeds)
	selectedSet := make(map[string]*model.ModelConfig)
	scores := make(map[string]int)

	if len(scoredList) > 0 {
		limit := 5
		if len(scoredList) < limit {
			limit = len(scoredList)
		}
		for i := 0; i < limit; i++ {
			tbl := strings.ToLower(scoredList[i].cfg.Table)
			selectedSet[tbl] = scoredList[i].cfg
			scores[tbl] = scoredList[i].score
		}
	} else {
		// Fallback: take top 3 active configs
		for i, cfg := range configs {
			if i >= 3 {
				break
			}
			tbl := strings.ToLower(cfg.Table)
			selectedSet[tbl] = cfg
			scores[tbl] = 1
		}
	}

	// 2. Relational Graph Traversal: Expand with related tables (incoming & outgoing foreign keys)
	// Check outgoing references from initial candidate tables
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

	// Check incoming references from other tables that mention prompt keywords
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
				} else if refCfg, exists := cfgByTable[stem]; exists {
					targetTbl = strings.ToLower(refCfg.Table)
				}
			}
			if targetTbl != "" {
				if _, isTargetSelected := selectedSet[targetTbl]; isTargetSelected {
					// Check if this referencing table shares prompt keywords
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

	// 3. Assemble DiscoveredTableMeta with full attributes & relationships
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
				if refCfg, exists := cfgByTable[pluralize(stem)]; exists {
					col.IsForeignKey = true
					col.RefTable = refCfg.Table
					col.RefColumn = "id"
				} else if refCfg, exists := cfgByTable[stem]; exists {
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
    { "from_table": "<table 1>", "from_field": "<col 1>", "to_table": "<table 2>", "to_field": "<col 2>", "join_type": "LEFT|INNER" }
  ],
  "filters": [
    { "table": "<table name>", "field": "<column>", "operator": "=|!=|>|<|like|in|is_null", "value": <literal value> }
  ],
  "aggregations": [
    { "name": "<alias e.g. total_capacity>", "function": "SUM|AVG|COUNT|MIN|MAX", "table": "<table name>", "field": "<column>", "group_by_key": "<paired dimension column>" }
  ],
  "group_by": [
    { "table": "<table name>", "field": "<column>" }
  ],
  "filter_params": [
    { "name": "<param_name>", "data_type": "string|int|date" }
  ],
  "driver": "%s",
  "save_mode": "PROCEDURE",
  "explanation": "<Short human-friendly summary of what query was generated>"
}

RULES:
1. ONLY use tables and columns present in the AVAILABLE DATABASE TABLES & FIELDS schema above.
2. If computing totals, averages, or counts across groups, add them to "aggregations" AND include the grouping columns in "group_by".
3. When joining tables, choose foreign key matches from the schema (e.g. stores.id = store_spares.store_id).
4. For multi-turn conversations, preserve existing selections and filters unless the user explicitly asks to replace or remove them.
5. Provide a clear, concise "explanation" summarizing the action taken.
6. Return ONLY the raw JSON object.`, driver, schemaContext, driver)
}

func extractAIQueryPlan(content string) (*AIQueryPlan, error) {
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
	clean = strings.TrimSpace(clean)

	var plan AIQueryPlan
	if err := json.Unmarshal([]byte(clean), &plan); err != nil {
		return nil, fmt.Errorf("invalid json: %w (content: %s)", err, clean)
	}

	return &plan, nil
}
