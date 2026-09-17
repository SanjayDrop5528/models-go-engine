// Package service implements dataset lifecycle orchestration including validation, AST planning,
// multi-database query compilation, previewing, saving, and runtime execution with parameter binding.
//
// File: dataset_service.go
// Usage:
//   This file defines the primary application-layer service (DataSetService) for all dynamic dataset operations.
//   It connects the user-facing HTTP endpoints or programmatic clients with the underlying AST planner,
//   domain validator, dialect compilers (PostgreSQL, MySQL, MongoDB), and physical database adapters.
//   It coordinates design-time features (preview without saving, DDL generation for procedures/functions)
//   and runtime features (executing saved datasets with payload/token parameter precedence).
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/SanjayDrop5528/models-go-engine/adapter"
	"github.com/SanjayDrop5528/models-go-engine/dataset/compiler"
	"github.com/SanjayDrop5528/models-go-engine/dataset/domain"
	"github.com/SanjayDrop5528/models-go-engine/dataset/planner"
	"github.com/SanjayDrop5528/models-go-engine/dataset/repository"
	"github.com/SanjayDrop5528/models-go-engine/dataset/resolver"
	"github.com/SanjayDrop5528/models-go-engine/dataset/validator"
	"github.com/SanjayDrop5528/models-go-engine/execution"
	"github.com/SanjayDrop5528/models-go-engine/operation"
)

// PreviewResponse contains column metadata, sample rows, and compiled pipelines for inspection.
type PreviewResponse struct {
	Columns           []domain.SelectedField `json:"columns"`
	Rows              []map[string]any       `json:"rows"`
	Pipeline          string                 `json:"pipeline"`
	ReferencePipeline string                 `json:"reference_pipeline"`
	Parameters        []domain.FilterParam   `json:"parameters"`
	DDLStatement      string                 `json:"ddl_statement,omitempty"`
}

// DataSetService orchestrates design-time Preview & Save, and runtime Execution.
type DataSetService struct {
	repo             repository.DataSetRepository
	validator        *validator.DataSetValidator
	planner          *planner.DataSetPlanner
	functionResolver resolver.FunctionResolver
	modelResolver    resolver.ModelResolver
	adapter          adapter.Adapter
	compilers        map[string]compiler.DataSetCompiler
}

// NewDataSetService creates a new DataSetService instance.
//
// Purpose:
//   Initializes the DataSetService with its required metadata repository, validators,
//   AST planners, resolvers, and target database adapter.
//
// Where it is used:
//   - Initialized in the engine setup layer (models-go-engine/project).
//   - Initialized in API server bootstrapping (models-go-example/examples/*/server.go).
//   - Initialized in integration and unit tests.
//
// When can it be used:
//   - At application startup when assembling the engine and configuring dataset capabilities.
func NewDataSetService(
	repo repository.DataSetRepository,
	mr resolver.ModelResolver,
	fr resolver.FieldResolver,
	fnr resolver.FunctionResolver,
	adp adapter.Adapter,
) *DataSetService {
	val := validator.NewDataSetValidator(mr, fr, fnr)
	pln := planner.NewPlanner(fnr)

	return &DataSetService{
		repo:             repo,
		validator:        val,
		planner:          pln,
		functionResolver: fnr,
		modelResolver:    mr,
		adapter:          adp,
		compilers:        make(map[string]compiler.DataSetCompiler),
	}
}

// RegisterCompiler registers a custom compiler for a specific driver.
//
// Purpose:
//   Adds or overrides a database-specific compiler implementation (e.g., "postgres", "mysql", "mongodb").
//
// Where it is used:
//   - Called during adapter registration or project startup to hook in database-specific compilers.
//   - Called in test setups to provide mock compilers.
//
// When can it be used:
//   - Before invoking Preview, Save, or Execute, to ensure compilation support for target database drivers.
func (s *DataSetService) RegisterCompiler(driver string, c compiler.DataSetCompiler) *DataSetService {
	s.compilers[strings.ToLower(driver)] = c
	return s
}

// SetAdapter sets or updates the underlying execution adapter.
//
// Purpose:
//   Attaches a physical database adapter (PostgreSQL, MySQL, MongoDB, Memory) to the dataset service
//   for running preview queries, applying DDL routines, and executing queries.
//
// Where it is used:
//   - Called by Engine/Project when an adapter is initialized or swapped.
//   - Called in test harnesses to attach real or mock adapters.
//
// When can it be used:
//   - Anytime the dataset service needs to be bound to an active database connection.
func (s *DataSetService) SetAdapter(adp adapter.Adapter) *DataSetService {
	s.adapter = adp
	return s
}

// Preview compiles and executes the dataset without saving it to database metadata.
//
// Purpose:
//   Validates the incoming dataset definition, plans an AST, compiles the executable query
//   and reference pipeline for the specified driver, runs a live preview query through the adapter,
//   and returns sample rows and compiled DDL without persisting to catalog storage.
//
// Where it is used:
//   - Called by the HTTP handler `POST /api/datasets/preview`.
//   - Used by the Angular UI Dataset Studio during interactive query design.
//
// When can it be used:
//   - During dataset authoring when users configure joins, custom calculations, aggregations,
//     and filter parameters and want immediate visual feedback without saving.
func (s *DataSetService) Preview(ctx context.Context, ds *domain.DataSet) (*PreviewResponse, error) {
	// 1. Validate
	if err := s.validator.Validate(ctx, ds); err != nil {
		return nil, err
	}

	// 2. Build AST
	ast, err := s.planner.BuildAST(ctx, ds)
	if err != nil {
		return nil, err
	}

	// 3. Compile (Check adapter first, then fallback to registered compiler)
	compiled, err := s.compile(ctx, ast, ds)
	if err != nil {
		return nil, err
	}

	// 4. Extract Columns Metadata
	var cols []domain.SelectedField
	for _, p := range ds.SelectedList {
		cols = append(cols, p)
	}
	for _, cc := range ds.CustomColumns {
		cols = append(cols, domain.SelectedField{
			Field:      cc.CustomColumnName,
			HeaderName: cc.CustomLabelName,
			DataType:   cc.Type,
		})
	}

	// 5. Execute preview query against adapter
	var rows []map[string]any
	if s.adapter != nil {
		execReq := execution.ExecutionRequest{
			Operation: operation.OpQuery,
			Target:    compiled.ExecutableQuery,
			Arguments: map[string]any{
				"preview":    true,
				"collection": ds.BaseCollection.Collection,
				"schema":     ds.BaseCollection.Schema,
			},
		}
		res, err := s.adapter.Execute(ctx, execReq)
		if err == nil && res != nil {
			if resMap, ok := res.Data.(map[string]any); ok {
				if resRows, ok := resMap["rows"].([]map[string]any); ok {
					rows = resRows
				}
			} else if rowSlice, ok := res.Data.([]map[string]any); ok {
				rows = rowSlice
			}
		}
	}

	if rows == nil {
		rows = make([]map[string]any, 0)
	}

	return &PreviewResponse{
		Columns:           cols,
		Rows:              rows,
		Pipeline:          compiled.ExecutableQuery,
		ReferencePipeline: compiled.ReferencePipeline,
		Parameters:        compiled.Parameters,
		DDLStatement:      compiled.DDLStatement,
	}, nil
}

// Save validates, compiles pipelines, executes DDL (procedures/functions), and persists dataset metadata.
//
// Purpose:
//   Validates the dataset definition, compiles the pipeline for the target dialect, applies any
//   necessary DDL statements on the database (e.g. `CREATE OR REPLACE PROCEDURE` or `FUNCTION`),
//   and saves the dataset definition to the system metadata catalog (`metadata_catalog.dataset`).
//
// Where it is used:
//   - Called by the HTTP handler `POST /api/datasets`.
//   - Used by the Angular UI Dataset Studio when clicking "Save Dataset".
//
// When can it be used:
//   - When finalizing and publishing a dataset for application consumption or report generation.
func (s *DataSetService) Save(ctx context.Context, ds *domain.DataSet) (*domain.DataSet, error) {
	// 1. Validate
	if err := s.validator.Validate(ctx, ds); err != nil {
		return nil, err
	}

	// 2. Build AST
	ast, err := s.planner.BuildAST(ctx, ds)
	if err != nil {
		return nil, err
	}

	// 3. Compile (Check adapter first, then fallback to registered compiler)
	compiled, err := s.compile(ctx, ast, ds)
	if err != nil {
		return nil, err
	}

	// 4. Execute Procedure/Function DDL if applicable
	if compiled.DDLStatement != "" && s.adapter != nil && (compiled.Driver == "postgres" || compiled.Driver == "mysql") {
		execReq := execution.ExecutionRequest{
			Operation: operation.OpDDL,
			Target:    compiled.DDLStatement,
			Arguments: map[string]any{"save_mode": string(compiled.SaveMode)},
		}
		if _, err := s.adapter.Execute(ctx, execReq); err != nil {
			if compiled.SaveMode == domain.SaveModeFunction {
				return nil, domain.WrapError(domain.ErrFunctionCreationFailed, "failed creating database function", err)
			}
			return nil, domain.WrapError(domain.ErrProcedureCreationFailed, "failed creating database procedure", err)
		}
	}

	// 5. Persist DataSet metadata
	ds.Pipeline = compiled.ExecutableQuery
	ds.ReferencePipeline = compiled.ReferencePipeline
	if ds.Status == "" {
		ds.Status = "ACTIVE"
	}
	ds.UpdatedAt = time.Now()

	if err := s.repo.Save(ctx, ds); err != nil {
		return nil, err
	}

	return ds, nil
}

// Execute resolves dataset by reference name, binds parameters safely, and runs the query.
//
// Purpose:
//   Standard entry point to execute a saved dataset by its unique reference name with runtime arguments.
//   Delegates to ExecuteWithUserToken with a nil userToken.
//
// Where it is used:
//   - Called by programmatic Go callers and simple execution APIs without authentication context.
//
// When can it be used:
//   - When executing datasets that do not depend on session user tokens (KTON) or when default tokens suffice.
// Execute executes a saved dataset by reference name with runtime parameter bindings.
func (s *DataSetService) Execute(ctx context.Context, referenceName string, runtimeParams map[string]any) ([]map[string]any, error) {
	return s.ExecuteWithOptions(ctx, referenceName, &domain.ExecuteRequest{
		FilterParams: runtimeParams,
	})
}

// ExecuteWithUserToken resolves dataset by reference name, binds parameters safely
// (supporting KTON user tokens, dynamic CD date expressions, and type coercions), and runs
// the dataset via Procedure, Function, or Direct Query.
func (s *DataSetService) ExecuteWithUserToken(ctx context.Context, referenceName string, runtimeParams map[string]any, userToken any) ([]map[string]any, error) {
	return s.ExecuteWithOptions(ctx, referenceName, &domain.ExecuteRequest{
		FilterParams: runtimeParams,
		UserToken:    userToken,
	})
}

// ExecuteWithOptions resolves dataset by reference name and executes it using the standardized
// ExecuteRequest supporting dynamic runtime filters, multi-column sorting, pagination (start, limit),
// and parameter substitution (filterParams).
func (s *DataSetService) ExecuteWithOptions(ctx context.Context, referenceName string, req *domain.ExecuteRequest) ([]map[string]any, error) {
	if req == nil {
		req = &domain.ExecuteRequest{}
	}
	ds, err := s.repo.FindByReferenceName(ctx, referenceName)
	if err != nil {
		return nil, err
	}

	runtimeParams := req.NormalizeRuntimeParams()

	// 1. Validate & Coerce Parameters: check payload first, else fallback to default value
	boundArgs := make(map[string]any)
	effectiveParams := make([]domain.FilterParam, len(ds.FilterParams))
	for i, p := range ds.FilterParams {
		var val any
		foundInPayload := false

		// Check if value came from payload (case-insensitive)
		for k, v := range runtimeParams {
			if strings.EqualFold(k, p.ParamName) && v != nil && v != "" {
				val = v
				foundInPayload = true
				break
			}
		}

		// If not in payload, fallback to default value
		if !foundInPayload || val == nil {
			if p.Paramvalue != nil && p.Paramvalue != "" {
				val = p.Paramvalue
			} else {
				val = p.DefaultValue
			}
		}

		if val == nil && p.Required {
			return nil, domain.NewErrorf(domain.ErrMissingRequiredParameter, "required parameter '%s' was not provided", p.ParamName)
		}

		effectiveParams[i] = p
		if val != nil {
			// Resolve dynamic expressions (KTON|key, CD|+0|ST, etc.)
			if strVal, ok := val.(string); ok {
				var user map[string]any
				if req.UserToken != nil {
					user, _ = domain.StructToMap(req.UserToken)
				}
				resolved := domain.ConvertValueToDataType("string", strVal, user)
				if resolved != "unsupported_data_type" && resolved != "" {
					val = resolved
				}
			}

			coerced, err := coerceDataType(val, p.ParamDataType)
			if err != nil {
				return nil, domain.WrapError(domain.ErrInvalidParameterType, fmt.Sprintf("parameter '%s' coercion failed", p.ParamName), err)
			}
			boundArgs[p.ParamName] = coerced
			effectiveParams[i].Paramvalue = coerced
		}
	}

	// 2. Select execution strategy based on SaveMode
	if s.adapter == nil {
		return []map[string]any{}, nil
	}

	cleanName := strings.ReplaceAll(ds.ReferenceName, "-", "_")

	// Determine base executable query with runtime parameters substituted
	baseQuery := ds.Pipeline
	if ds.ReferencePipeline != "" {
		baseQuery = domain.CreateFilterParams(effectiveParams, ds.ReferencePipeline, req.UserToken)
	}
	if baseQuery == "" {
		baseQuery = ds.Pipeline
	}

	// Build wrapped query with runtime filter, sort, and pagination if applicable
	finalQuery := buildWrappedQuery(baseQuery, ds, req)

	var execReq execution.ExecutionRequest
	switch ds.SaveMode {
	case domain.SaveModeProcedure:
		procTarget := fmt.Sprintf("sp_%s", cleanName)
		if ds.BaseCollection.Schema != "" && ds.BaseCollection.Schema != "public" {
			procTarget = fmt.Sprintf("%s.sp_%s", ds.BaseCollection.Schema, cleanName)
		}
		execReq = execution.ExecutionRequest{
			Operation: operation.OpProcedure,
			Target:    procTarget,
			Arguments: boundArgs,
		}
	case domain.SaveModeFunction:
		fnTarget := fmt.Sprintf("fn_%s", cleanName)
		if ds.BaseCollection.Schema != "" && ds.BaseCollection.Schema != "public" {
			fnTarget = fmt.Sprintf("%s.fn_%s", ds.BaseCollection.Schema, cleanName)
		}
		execReq = execution.ExecutionRequest{
			Operation: operation.OpFunction,
			Target:    fnTarget,
			Arguments: boundArgs,
		}
	default: // SaveModeQuery
		execReq = execution.ExecutionRequest{
			Operation: operation.OpQuery,
			Target:    finalQuery,
			Arguments: boundArgs,
		}
	}

	res, err := s.adapter.Execute(ctx, execReq)
	if err != nil {
		if execReq.Operation != operation.OpQuery && finalQuery != "" {
			// Fallback: execute final query directly if stored procedure/function is missing
			qRes, qErr := s.adapter.Execute(ctx, execution.ExecutionRequest{
				Operation: operation.OpQuery,
				Target:    finalQuery,
				Arguments: boundArgs,
			})
			if qErr != nil {
				return nil, domain.WrapError(domain.ErrPipelineExecutionFailed, "dataset execution failed", qErr)
			}
			res = qRes
		} else {
			return nil, domain.WrapError(domain.ErrPipelineExecutionFailed, "dataset execution failed", err)
		}
	}

	var rows []map[string]any
	if res != nil {
		if resMap, ok := res.Data.(map[string]any); ok {
			if r, ok := resMap["rows"].([]map[string]any); ok && len(r) > 0 {
				rows = r
			}
		} else if r, ok := res.Data.([]map[string]any); ok && len(r) > 0 {
			rows = r
		} else if rawStr, ok := res.Data.(string); ok {
			var parsedRows []map[string]any
			if err := json.Unmarshal([]byte(rawStr), &parsedRows); err == nil && len(parsedRows) > 0 {
				rows = parsedRows
			}
		}
	}

	// For procedures or if direct execution returned no tabular rows, execute finalQuery
	if len(rows) == 0 && finalQuery != "" {
		qRes, qErr := s.adapter.Execute(ctx, execution.ExecutionRequest{
			Operation: operation.OpQuery,
			Target:    finalQuery,
			Arguments: boundArgs,
		})
		if qErr == nil && qRes != nil {
			if resMap, ok := qRes.Data.(map[string]any); ok {
				if qRows, ok := resMap["rows"].([]map[string]any); ok {
					rows = qRows
				}
			} else if qRows, ok := qRes.Data.([]map[string]any); ok {
				rows = qRows
			}
		}
	}

	// In-memory safety pagination / slicing if query wasn't wrapped (e.g. non-SQL adapter)
	if len(rows) > 0 && (req.Start > 0 || req.Limit > 0) {
		if req.Limit > 0 && len(rows) > req.Limit {
			start := req.Start
			if start >= len(rows) {
				return []map[string]any{}, nil
			}
			end := start + req.Limit
			if end > len(rows) {
				end = len(rows)
			}
			rows = rows[start:end]
		}
	}

	if rows == nil {
		rows = []map[string]any{}
	}

	return rows, nil
}

// buildWrappedQuery wraps or augments a base SQL or MongoDB query with runtime filtering,
// dynamic sorting, pagination, and ordered filter appending ("first" or "last").
func buildWrappedQuery(baseQuery string, ds *domain.DataSet, req *domain.ExecuteRequest) string {
	trimmed := strings.TrimSpace(baseQuery)
	if trimmed == "" || req == nil {
		return baseQuery
	}

	hasFilter := req.Filter != nil && len(req.Filter) > 0
	hasSort := req.Sort != nil
	hasPaging := req.Start > 0 || req.Limit > 0

	if !hasFilter && !hasSort && !hasPaging && (ds == nil || len(ds.Filter) == 0) {
		return baseQuery
	}

	// 1. MongoDB Aggregation Pipeline Support
	if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
		var stages []map[string]any
		if err := json.Unmarshal([]byte(trimmed), &stages); err == nil {
			if hasFilter {
				matchStage := map[string]any{
					"$match": req.Filter,
				}
				if req.GetAppendFilter() == "first" {
					stages = append([]map[string]any{matchStage}, stages...)
				} else {
					stages = append(stages, matchStage)
				}
			}
			if req.Sort != nil {
				stages = append(stages, map[string]any{"$sort": req.Sort})
			}
			if req.Start > 0 {
				stages = append(stages, map[string]any{"$skip": req.Start})
			}
			if req.Limit > 0 {
				stages = append(stages, map[string]any{"$limit": req.Limit})
			}
			if b, err := json.Marshal(stages); err == nil {
				return string(b)
			}
		}
		return baseQuery
	}

	// 2. SQL SELECT Queries Support
	if strings.HasPrefix(strings.ToUpper(trimmed), "SELECT") {
		cleanBase := strings.TrimSuffix(trimmed, ";")
		var sb strings.Builder
		sb.WriteString("SELECT * FROM (\n")
		sb.WriteString(cleanBase)
		sb.WriteString("\n) AS \"_exec_sub\"")

		var reqConditions []string
		if hasFilter {
			var cols []string
			for col := range req.Filter {
				cols = append(cols, col)
			}
			sort.Strings(cols)
			for _, col := range cols {
				cond := formatSQLFilterCondition("_exec_sub", col, req.Filter[col])
				if cond != "" {
					reqConditions = append(reqConditions, cond)
				}
			}
		}

		var dsConditions []string
		if ds != nil && len(ds.Filter) > 0 {
			var cols []string
			for col := range ds.Filter {
				cols = append(cols, col)
			}
			sort.Strings(cols)
			for _, col := range cols {
				if req != nil && req.Filter != nil {
					if _, exists := req.Filter[col]; exists {
						continue
					}
				}
				cond := formatSQLFilterCondition("_exec_sub", col, ds.Filter[col])
				if cond != "" {
					dsConditions = append(dsConditions, cond)
				}
			}
		}

		var allConditions []string
		if req.GetAppendFilter() == "first" {
			allConditions = append(allConditions, reqConditions...)
			allConditions = append(allConditions, dsConditions...)
		} else {
			allConditions = append(allConditions, dsConditions...)
			allConditions = append(allConditions, reqConditions...)
		}

		if len(allConditions) > 0 {
			sb.WriteString("\nWHERE ")
			sb.WriteString(strings.Join(allConditions, " AND "))
		}

		if hasSort {
			sortClause := formatSQLSortClause("_exec_sub", req.Sort)
			if sortClause != "" {
				sb.WriteString("\nORDER BY ")
				sb.WriteString(sortClause)
			}
		}

		if req.Limit > 0 {
			sb.WriteString(fmt.Sprintf("\nLIMIT %d", req.Limit))
		}
		if req.Start > 0 {
			sb.WriteString(fmt.Sprintf(" OFFSET %d", req.Start))
		}

		sb.WriteString(";")
		return sb.String()
	}

	return baseQuery
}

// ApplyFilterToSQL injects or appends dynamic filter conditions into a SQL WHERE clause directly.
// If appendFilter is "first", new conditions are prepended to existing WHERE conditions:
//
//	WHERE (<new_conditions>) AND (<existing_where>)
//
// If appendFilter is "last" (or default), new conditions are appended to existing WHERE conditions:
//
//	WHERE (<existing_where>) AND (<new_conditions>)
//
// If no existing WHERE clause is present, it adds:
//
//	WHERE <new_conditions>
func ApplyFilterToSQL(baseSQL string, filter map[string]any, appendFilter string) string {
	if len(filter) == 0 {
		return baseSQL
	}
	var cols []string
	for col := range filter {
		cols = append(cols, col)
	}
	sort.Strings(cols)

	var conditions []string
	for _, col := range cols {
		cond := formatSQLFilterCondition("", col, filter[col])
		if cond != "" {
			conditions = append(conditions, cond)
		}
	}
	if len(conditions) == 0 {
		return baseSQL
	}
	newFilterStr := strings.Join(conditions, " AND ")

	whereIdx := findTopLevelWhere(baseSQL)

	if whereIdx != -1 {
		preWhere := baseSQL[:whereIdx]
		afterWhere := baseSQL[whereIdx+5:] // length of "WHERE"

		endIdx := findClauseEnd(afterWhere)
		existingWhere := strings.TrimSpace(afterWhere[:endIdx])
		remainder := afterWhere[endIdx:]

		var combinedWhere string
		if strings.EqualFold(strings.TrimSpace(appendFilter), "first") {
			combinedWhere = fmt.Sprintf("(%s) AND (%s)", newFilterStr, existingWhere)
		} else {
			combinedWhere = fmt.Sprintf("(%s) AND (%s)", existingWhere, newFilterStr)
		}

		return fmt.Sprintf("%sWHERE %s%s", preWhere, combinedWhere, remainder)
	}

	// No WHERE clause; insert before ORDER BY, GROUP BY, etc., or before trailing ';'
	insertIdx := findClauseEnd(baseSQL)
	pre := baseSQL[:insertIdx]
	remainder := baseSQL[insertIdx:]
	cleanPre := strings.TrimRight(pre, " \t\r\n;")
	cleanRem := strings.TrimSpace(remainder)
	if cleanRem != "" {
		return fmt.Sprintf("%s\nWHERE %s\n%s", cleanPre, newFilterStr, cleanRem)
	}
	return fmt.Sprintf("%s\nWHERE %s;", cleanPre, newFilterStr)
}

// findTopLevelWhere finds the index of the top-level WHERE keyword not inside parentheses.
func findTopLevelWhere(sql string) int {
	parenDepth := 0
	inQuote := false
	var quoteChar rune
	runes := []rune(sql)
	n := len(runes)

	for i := 0; i < n; i++ {
		r := runes[i]
		if inQuote {
			if r == quoteChar {
				inQuote = false
			}
			continue
		}
		if r == '\'' || r == '"' {
			inQuote = true
			quoteChar = r
			continue
		}
		if r == '(' {
			parenDepth++
			continue
		}
		if r == ')' {
			if parenDepth > 0 {
				parenDepth--
			}
			continue
		}
		if parenDepth == 0 && (r == 'W' || r == 'w') && i+5 <= n {
			candidate := strings.ToUpper(string(runes[i : i+5]))
			if candidate == "WHERE" {
				beforeOk := i == 0 || isSQLWordBoundary(runes[i-1])
				afterOk := i+5 == n || isSQLWordBoundary(runes[i+5])
				if beforeOk && afterOk {
					return i
				}
			}
		}
	}
	return -1
}

// findClauseEnd finds where WHERE conditions end (start of GROUP BY, HAVING, WINDOW, ORDER BY, LIMIT, OFFSET, UNION).
func findClauseEnd(sql string) int {
	parenDepth := 0
	inQuote := false
	var quoteChar rune
	runes := []rune(sql)
	n := len(runes)

	keywords := []string{"GROUP BY", "HAVING", "WINDOW", "ORDER BY", "LIMIT", "OFFSET", "FETCH", "UNION", "INTERSECT", "EXCEPT"}

	for i := 0; i < n; i++ {
		r := runes[i]
		if inQuote {
			if r == quoteChar {
				inQuote = false
			}
			continue
		}
		if r == '\'' || r == '"' {
			inQuote = true
			quoteChar = r
			continue
		}
		if r == '(' {
			parenDepth++
			continue
		}
		if r == ')' {
			if parenDepth > 0 {
				parenDepth--
			}
			continue
		}
		if parenDepth == 0 {
			if r == ';' {
				return i
			}
			for _, kw := range keywords {
				kwLen := len(kw)
				if i+kwLen <= n {
					candidate := strings.ToUpper(string(runes[i : i+kwLen]))
					if candidate == kw {
						beforeOk := i == 0 || isSQLWordBoundary(runes[i-1])
						afterOk := i+kwLen == n || isSQLWordBoundary(runes[i+kwLen])
						if beforeOk && afterOk {
							return i
						}
					}
				}
			}
		}
	}
	return n
}

func isSQLWordBoundary(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '(' || r == ')' || r == ',' || r == ';'
}

// formatSQLFilterCondition translates a dynamic runtime filter key/value into an SQL condition.
func formatSQLFilterCondition(alias, col string, val any) string {
	safeCol := col
	if strings.Contains(safeCol, ".") {
		parts := strings.Split(safeCol, ".")
		safeCol = parts[len(parts)-1]
	}
	var colExpr string
	if alias != "" {
		colExpr = fmt.Sprintf("\"%s\".\"%s\"", alias, strings.ReplaceAll(safeCol, "\"", ""))
	} else {
		colExpr = fmt.Sprintf("\"%s\"", strings.ReplaceAll(safeCol, "\"", ""))
	}

	if val == nil {
		return fmt.Sprintf("%s IS NULL", colExpr)
	}

	switch v := val.(type) {
	case string:
		return fmt.Sprintf("%s = '%s'", colExpr, strings.ReplaceAll(v, "'", "''"))
	case bool:
		if v {
			return fmt.Sprintf("%s = TRUE", colExpr)
		}
		return fmt.Sprintf("%s = FALSE", colExpr)
	case int, int32, int64, float32, float64:
		return fmt.Sprintf("%s = %v", colExpr, v)
	case map[string]any:
		var parts []string
		for op, operand := range v {
			switch strings.ToLower(op) {
			case "$eq":
				parts = append(parts, fmt.Sprintf("%s = '%v'", colExpr, operand))
			case "$ne":
				parts = append(parts, fmt.Sprintf("%s != '%v'", colExpr, operand))
			case "$gt":
				parts = append(parts, fmt.Sprintf("%s > '%v'", colExpr, operand))
			case "$gte":
				parts = append(parts, fmt.Sprintf("%s >= '%v'", colExpr, operand))
			case "$lt":
				parts = append(parts, fmt.Sprintf("%s < '%v'", colExpr, operand))
			case "$lte":
				parts = append(parts, fmt.Sprintf("%s <= '%v'", colExpr, operand))
			case "$like", "$regex":
				parts = append(parts, fmt.Sprintf("%s LIKE '%v'", colExpr, operand))
			case "$in":
				if arr, ok := operand.([]any); ok {
					var inItems []string
					for _, item := range arr {
						inItems = append(inItems, fmt.Sprintf("'%v'", item))
					}
					parts = append(parts, fmt.Sprintf("%s IN (%s)", colExpr, strings.Join(inItems, ", ")))
				}
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, " AND ")
		}
	}
	return fmt.Sprintf("%s = '%v'", colExpr, val)
}

// formatSQLSortClause formats a dynamic sort request into an SQL ORDER BY clause.
func formatSQLSortClause(alias string, sortVal any) string {
	if sortVal == nil {
		return ""
	}
	switch s := sortVal.(type) {
	case string:
		s = strings.TrimSpace(s)
		if s == "" {
			return ""
		}
		parts := strings.Fields(s)
		if len(parts) == 1 {
			return fmt.Sprintf("\"%s\".\"%s\" ASC", alias, strings.ReplaceAll(parts[0], "\"", ""))
		}
		dir := "ASC"
		if strings.EqualFold(parts[1], "DESC") {
			dir = "DESC"
		}
		return fmt.Sprintf("\"%s\".\"%s\" %s", alias, strings.ReplaceAll(parts[0], "\"", ""), dir)
	case map[string]any:
		var parts []string
		for field, dir := range s {
			dStr := fmt.Sprintf("%v", dir)
			order := "ASC"
			if strings.EqualFold(dStr, "desc") || dStr == "-1" {
				order = "DESC"
			}
			parts = append(parts, fmt.Sprintf("\"%s\".\"%s\" %s", alias, strings.ReplaceAll(field, "\"", ""), order))
		}
		return strings.Join(parts, ", ")
	case []any:
		var parts []string
		for _, item := range s {
			if m, ok := item.(map[string]any); ok {
				f := fmt.Sprintf("%v", m["field"])
				if f == "" {
					f = fmt.Sprintf("%v", m["col"])
				}
				d := fmt.Sprintf("%v", m["dir"])
				if d == "" {
					d = fmt.Sprintf("%v", m["order"])
				}
				order := "ASC"
				if strings.EqualFold(d, "desc") || d == "-1" {
					order = "DESC"
				}
				if f != "" {
					parts = append(parts, fmt.Sprintf("\"%s\".\"%s\" %s", alias, strings.ReplaceAll(f, "\"", ""), order))
				}
			}
		}
		return strings.Join(parts, ", ")
	}
	return ""
}

// coerceDataType parses and converts an arbitrary parameter value into its target Go data type.
//
// Purpose:
//   Ensures parameter values match the declared data type before binding into queries or procedures.
//
// Where it is used:
//   - Called internally by ExecuteWithUserToken for each bounded parameter.
//
// When can it be used:
//   - When raw string or interface values from JSON payloads need to be typed (int, float, bool, date).
func coerceDataType(val any, targetType string) (any, error) {
	if val == nil {
		return nil, nil
	}
	sVal := fmt.Sprintf("%v", val)

	switch strings.ToLower(targetType) {
	case "int", "integer":
		return strconv.Atoi(sVal)
	case "decimal", "float", "numeric":
		return strconv.ParseFloat(sVal, 64)
	case "boolean", "bool":
		return strconv.ParseBool(sVal)
	case "date", "timestamp", "datetime":
		if t, err := time.Parse(time.RFC3339, sVal); err == nil {
			return t, nil
		}
		if t, err := time.Parse("2006-01-02", sVal); err == nil {
			return t, nil
		}
		return sVal, nil
	default:
		return sVal, nil
	}
}

// compile delegates dataset compilation to the adapter's native compiler or a registered dialect compiler.
//
// Purpose:
//   Converts an abstract syntax tree (QueryAST) into executable queries and reference pipelines.
//
// Where it is used:
//   - Called internally by Preview and Save.
//
// When can it be used:
//   - When generating database-specific SQL or MongoDB aggregation pipelines from an AST.
func (s *DataSetService) compile(ctx context.Context, ast *planner.QueryAST, ds *domain.DataSet) (*compiler.CompiledPipeline, error) {
	// 1. Check if adapter provides native dataset compilation
	if dsAdapter, ok := s.adapter.(adapter.DataSetAdapter); ok {
		if comp := dsAdapter.DataSetCompiler(); comp != nil {
			res, err := comp.Compile(ctx, ast, ds)
			if err != nil {
				return nil, err
			}
			if cp, ok := res.(*compiler.CompiledPipeline); ok {
				return cp, nil
			}
		}
	}

	// 2. Fallback to registered compiler
	driver := strings.ToLower(ds.Driver)
	if driver == "" {
		driver = "postgres"
	}
	comp, exists := s.compilers[driver]
	if !exists {
		return nil, domain.NewErrorf(domain.ErrUnsupportedDriver, "no compiler available for driver '%s'", driver)
	}

	return comp.Compile(ctx, ast, ds)
}
