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
func (s *DataSetService) Execute(ctx context.Context, referenceName string, runtimeParams map[string]any) ([]map[string]any, error) {
	return s.ExecuteWithUserToken(ctx, referenceName, runtimeParams, nil)
}

// ExecuteWithUserToken resolves dataset by reference name, binds parameters safely
// (supporting KTON user tokens, dynamic CD date expressions, and type coercions), and runs
// the dataset via Procedure, Function, or Direct Query.
//
// Purpose:
//   Executes a dataset using full parameter precedence:
//     1. Checks if each parameter is provided in `runtimeParams` (payload). If yes, uses it.
//     2. If not provided or empty, falls back to `defaultValue`.
//     3. Resolves dynamic token macros (`KTON|<key>`) from `userToken`.
//     4. Resolves dynamic date macros (`CD|<offset>|<mode>`) using user's timezone.
//     5. Dispatches execution according to `SaveMode`:
//          - PROCEDURE: executes `CALL sp_<name>(...)`.
//          - FUNCTION: executes `SELECT fn_<name>(...)` or `SELECT * FROM fn_<name>(...)`.
//          - QUERY: performs placeholder substitution via CreateFilterParams and runs raw query.
//
// Where it is used:
//   - Called by the HTTP handler `POST /api/datasets/{referenceName}/execute`.
//   - Used by application services executing multi-tenant or role-scoped queries.
//
// When can it be used:
//   - At runtime whenever data needs to be retrieved from a saved dataset with runtime arguments and session tokens.
func (s *DataSetService) ExecuteWithUserToken(ctx context.Context, referenceName string, runtimeParams map[string]any, userToken any) ([]map[string]any, error) {
	ds, err := s.repo.FindByReferenceName(ctx, referenceName)
	if err != nil {
		return nil, err
	}

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
				if userToken != nil {
					user, _ = domain.StructToMap(userToken)
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
	var execReq execution.ExecutionRequest

	switch ds.SaveMode {
	case domain.SaveModeProcedure:
		execReq = execution.ExecutionRequest{
			Operation: operation.OpProcedure,
			Target:    fmt.Sprintf("sp_%s", cleanName),
			Arguments: boundArgs,
		}
	case domain.SaveModeFunction:
		execReq = execution.ExecutionRequest{
			Operation: operation.OpFunction,
			Target:    fmt.Sprintf("fn_%s", cleanName),
			Arguments: boundArgs,
		}
	default: // SaveModeQuery
		queryToRun := ds.Pipeline
		if ds.ReferencePipeline != "" {
			queryToRun = domain.CreateFilterParams(effectiveParams, ds.ReferencePipeline, userToken)
		}
		if queryToRun == "" {
			queryToRun = ds.Pipeline
		}
		execReq = execution.ExecutionRequest{
			Operation: operation.OpQuery,
			Target:    queryToRun,
			Arguments: boundArgs,
		}
	}

	res, err := s.adapter.Execute(ctx, execReq)
	if err != nil {
		return nil, domain.WrapError(domain.ErrPipelineExecutionFailed, "dataset execution failed", err)
	}

	if res != nil {
		if resMap, ok := res.Data.(map[string]any); ok {
			if rows, ok := resMap["rows"].([]map[string]any); ok {
				return rows, nil
			}
		} else if rows, ok := res.Data.([]map[string]any); ok {
			return rows, nil
		} else if rawStr, ok := res.Data.(string); ok {
			var parsedRows []map[string]any
			if err := json.Unmarshal([]byte(rawStr), &parsedRows); err == nil {
				return parsedRows, nil
			}
		}
	}

	return []map[string]any{}, nil
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
