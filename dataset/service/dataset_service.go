// Package service implements dataset lifecycle orchestration including validation, AST planning,
// multi-database query compilation, previewing, saving, and runtime execution with parameter binding.
//
// File: dataset_service.go
// Usage:
//
//	This file defines the primary application-layer service (DataSetService) for all dynamic dataset operations.
//	It connects the user-facing HTTP endpoints or programmatic clients with the underlying AST planner,
//	domain validator, dialect compilers (PostgreSQL, MySQL, MongoDB), and physical database adapters.
//	It coordinates design-time features (preview without saving, DDL generation for procedures/functions)
//	and runtime features (executing saved datasets with payload/token parameter precedence).
package service

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
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

// DecryptFunc allows callers or shared libraries (e.g., FleetOne's utils.DecryptField)
// to provide custom or DBMS-specific decryption algorithms (PGP sym decrypt, KMS, etc.).
type DecryptFunc func(ctx context.Context, encrypted, key string) (string, error)

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
	mu               sync.RWMutex
	repo             repository.DataSetRepository
	validator        *validator.DataSetValidator
	planner          *planner.DataSetPlanner
	functionResolver resolver.FunctionResolver
	modelResolver    resolver.ModelResolver
	adapter          adapter.Adapter
	compilers        map[string]compiler.DataSetCompiler
	decryptionKey    string
	decryptFunc      DecryptFunc
}

// NewDataSetService creates a new DataSetService instance.
//
// Purpose:
//
//	Initializes the DataSetService with its required metadata repository, validators,
//	AST planners, resolvers, and target database adapter.
//
// Where it is used:
//   - Initialized in the engine setup layer (models-go-engine/project).
//   - Initialized in API server bootstrapping (models-go-example/examples/*/server.go).
//   - Initialized in integration and unit tests.
//
// When can it be used:
//   - At application startup when assembling the engine and configuring dataset capabilities.
//
// Parameters:
//   - repo (repository.DataSetRepository): Input value used by the operation.
//   - mr (resolver.ModelResolver): Input value used by the operation.
//   - fr (resolver.FieldResolver): Input value used by the operation.
//   - fnr (resolver.FunctionResolver): Input value used by the operation.
//   - adp (adapter.Adapter): Input value used by the operation.
//
// Returns:
//   - result1 (*DataSetService): Returned value.
//
// Business Rules:
//   - Follows the invariants documented by NewDataSetService and its owning package.
//
// Side Effects:
//   - Does not intentionally mutate caller-owned input; configured dependencies may still be consulted.
//
// Errors:
//   - This function does not return an error.
//
// Note:
//   - Callers should preserve the documented ownership and lifecycle of supplied values.
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
//
//	Adds or overrides a database-specific compiler implementation (e.g., "postgres", "mysql", "mongodb").
//
// Where it is used:
//   - Called during adapter registration or project startup to hook in database-specific compilers.
//   - Called in test setups to provide mock compilers.
//
// When can it be used:
//   - Before invoking Preview, Save, or Execute, to ensure compilation support for target database drivers.
//
// Parameters:
//   - driver (string): Input value used by the operation.
//   - c (compiler.DataSetCompiler): Input value used by the operation.
//
// Returns:
//   - result1 (*DataSetService): Returned value.
//
// Business Rules:
//   - Follows the invariants documented by RegisterCompiler and its owning package.
//
// Side Effects:
//   - May update receiver state, persistent storage, or configured external dependencies.
//
// Errors:
//   - This function does not return an error.
//
// Note:
//   - Callers should preserve the documented ownership and lifecycle of supplied values.
func (s *DataSetService) RegisterCompiler(driver string, c compiler.DataSetCompiler) *DataSetService {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.compilers[strings.ToLower(driver)] = c
	return s
}

// SetDecryptionKey sets the secret decryption key used for decrypting sensitive dataset fields.
func (s *DataSetService) SetDecryptionKey(key string) *DataSetService {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.decryptionKey = key
	return s
}

// DecryptionKey returns the configured decryption key, falling back to environment variables.
func (s *DataSetService) DecryptionKey() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.decryptionKey != "" {
		return s.decryptionKey
	}
	if key := os.Getenv("DB_ENCRYPTION_KEY"); key != "" {
		return key
	}
	return os.Getenv("DB_CONFIG_ENCRYPTION_KEY")
}

// SetDecryptFunc configures a custom decryption callback (e.g. bridging shared-lib's utils.DecryptField).
func (s *DataSetService) SetDecryptFunc(fn DecryptFunc) *DataSetService {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.decryptFunc = fn
	return s
}

// SetAdapter sets or updates the underlying execution adapter.
//
// Purpose:
//
//	Attaches a physical database adapter (PostgreSQL, MySQL, MongoDB, Memory) to the dataset service
//	for running preview queries, applying DDL routines, and executing queries.
//
// Where it is used:
//   - Called by Engine/Project when an adapter is initialized or swapped.
//   - Called in test harnesses to attach real or mock adapters.
//
// When can it be used:
//   - Anytime the dataset service needs to be bound to an active database connection.
//
// Parameters:
//   - adp (adapter.Adapter): Input value used by the operation.
//
// Returns:
//   - result1 (*DataSetService): Returned value.
//
// Business Rules:
//   - Follows the invariants documented by SetAdapter and its owning package.
//
// Side Effects:
//   - May update receiver state, persistent storage, or configured external dependencies.
//
// Errors:
//   - This function does not return an error.
//
// Note:
//   - Callers should preserve the documented ownership and lifecycle of supplied values.
func (s *DataSetService) SetAdapter(adp adapter.Adapter) *DataSetService {
	s.adapter = adp
	return s
}

// Preview compiles and executes the dataset without saving it to database metadata.
//
// Purpose:
//
//	Validates the incoming dataset definition, plans an AST, compiles the executable query
//	and reference pipeline for the specified driver, runs a live preview query through the adapter,
//	and returns sample rows and compiled DDL without persisting to catalog storage.
//
// Where it is used:
//   - Called by the HTTP handler `POST /api/datasets/preview`.
//   - Used by the Angular UI Dataset Studio during interactive query design.
//
// When can it be used:
//   - During dataset authoring when users configure joins, custom calculations, aggregations,
//     and filter parameters and want immediate visual feedback without saving.
//
// Parameters:
//   - ctx (context.Context): Input value used by the operation.
//   - ds (*domain.DataSet): Input value used by the operation.
//
// Returns:
//   - result1 (*PreviewResponse): Returned value.
//   - result2 (error): Returned value.
//
// Business Rules:
//   - Follows the invariants documented by Preview and its owning package.
//
// Side Effects:
//   - Does not intentionally mutate caller-owned input; configured dependencies may still be consulted.
//
// Errors:
//   - Returns validation, configuration, context, or underlying dependency errors.
//
// Note:
//   - The context controls cancellation and request lifetime.
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
		alreadyPresent := false
		for _, c := range cols {
			if strings.EqualFold(c.Field, cc.CustomColumnName) {
				alreadyPresent = true
				break
			}
		}
		if !alreadyPresent {
			cols = append(cols, domain.SelectedField{
				Field:      cc.CustomColumnName,
				HeaderName: cc.CustomLabelName,
				DataType:   cc.Type,
			})
		}
	}

	// 5. Execute preview query against adapter
	var rows []map[string]any
	if s.adapter != nil {
		execReq := execution.ExecutionRequest{
			Operation: operation.OpQuery,
			Target:    compiled.ExecutableQuery,
			Options: map[string]any{
				"preview":    true,
				"collection": ds.BaseCollection.Collection,
				"schema":     ds.BaseCollection.Schema,
			},
		}
		if s.adapter.Name() == "mongodb" {
			execReq.Arguments = map[string]any{
				"collection": ds.BaseCollection.Collection,
			}
		} else if len(compiled.Parameters) > 0 {
			boundArgs := make(map[string]any)
			for _, p := range compiled.Parameters {
				if p.DefaultValue != nil {
					boundArgs[p.ParamName] = p.DefaultValue
				}
			}
			if len(boundArgs) > 0 {
				execReq.Arguments = boundArgs
			}
		}

		res, err := s.adapter.Execute(ctx, execReq)
		if err != nil {
			log.Printf("[DataSetService] Preview query execution failed: %v", err)
		} else if res != nil {
			if resMap, ok := res.Data.(map[string]any); ok {
				if resRows, ok := resMap["rows"].([]map[string]any); ok {
					rows = resRows
				}
			} else if rowSlice, ok := res.Data.([]map[string]any); ok {
				rows = rowSlice
			} else if sliceAny, ok := res.Data.([]any); ok {
				for _, item := range sliceAny {
					if m, ok := item.(map[string]any); ok {
						rows = append(rows, m)
					}
				}
			}
		}
	}

	if rows == nil {
		rows = make([]map[string]any, 0)
	}
	rows = s.normalizeAndDecryptDatasetRows(ctx, rows)
	if len(ds.SelectedList) > 0 {
		rows = projectDatasetRows(rows, ds)
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
//
//	Validates the dataset definition, compiles the pipeline for the target dialect, applies any
//	necessary DDL statements on the database (e.g. `CREATE OR REPLACE PROCEDURE` or `FUNCTION`),
//	and saves the dataset definition to the system metadata catalog (`metadata_catalog.dataset`).
//
// Where it is used:
//   - Called by the HTTP handler `POST /api/datasets`.
//   - Used by the Angular UI Dataset Studio when clicking "Save Dataset".
//
// When can it be used:
//   - When finalizing and publishing a dataset for application consumption or report generation.
//
// Parameters:
//   - ctx (context.Context): Input value used by the operation.
//   - ds (*domain.DataSet): Input value used by the operation.
//
// Returns:
//   - result1 (*domain.DataSet): Returned value.
//   - result2 (error): Returned value.
//
// Business Rules:
//   - Follows the invariants documented by Save and its owning package.
//
// Side Effects:
//   - May update receiver state, persistent storage, or configured external dependencies.
//
// Errors:
//   - Returns validation, configuration, context, or underlying dependency errors.
//
// Note:
//   - The context controls cancellation and request lifetime.
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
//
//	Standard entry point to execute a saved dataset by its unique reference name with runtime arguments.
//	Delegates to ExecuteWithUserToken with a nil userToken.
//
// Where it is used:
//   - Called by programmatic Go callers and simple execution APIs without authentication context.
//
// When can it be used:
//   - When executing datasets that do not depend on session user tokens (KTON) or when default tokens suffice.
//
// Execute executes a saved dataset by reference name with runtime parameter bindings.
//
// Parameters:
//   - ctx (context.Context): Input value used by the operation.
//   - referenceName (string): Input value used by the operation.
//   - runtimeParams (map[string]any): Input value used by the operation.
//
// Returns:
//   - result1 ([]map[string]any): Returned value.
//   - result2 (error): Returned value.
//
// Business Rules:
//   - Follows the invariants documented by Execute and its owning package.
//
// Side Effects:
//   - May update receiver state, persistent storage, or configured external dependencies.
//
// Errors:
//   - Returns validation, configuration, context, or underlying dependency errors.
//
// Note:
//   - The context controls cancellation and request lifetime.
func (s *DataSetService) Execute(ctx context.Context, referenceName string, runtimeParams map[string]any) ([]map[string]any, error) {
	return s.ExecuteWithOptions(ctx, referenceName, &domain.ExecuteRequest{
		FilterParams: runtimeParams,
	})
}

// ExecuteWithUserToken resolves dataset by reference name, binds parameters safely
// (supporting KTON user tokens, dynamic CD date expressions, and type coercions), and runs
// the dataset via Procedure, Function, or Direct Query.
//
// Parameters:
//   - ctx (context.Context): Input value used by the operation.
//   - referenceName (string): Input value used by the operation.
//   - runtimeParams (map[string]any): Input value used by the operation.
//   - userToken (any): Input value used by the operation.
//
// Returns:
//   - result1 ([]map[string]any): Returned value.
//   - result2 (error): Returned value.
//
// Business Rules:
//   - Follows the invariants documented by ExecuteWithUserToken and its owning package.
//
// Side Effects:
//   - May update receiver state, persistent storage, or configured external dependencies.
//
// Errors:
//   - Returns validation, configuration, context, or underlying dependency errors.
//
// Note:
//   - The context controls cancellation and request lifetime.
func (s *DataSetService) ExecuteWithUserToken(ctx context.Context, referenceName string, runtimeParams map[string]any, userToken any) ([]map[string]any, error) {
	return s.ExecuteWithOptions(ctx, referenceName, &domain.ExecuteRequest{
		FilterParams: runtimeParams,
		UserToken:    userToken,
	})
}

// ExecuteWithOptions resolves dataset by reference name and executes it using the standardized
// ExecuteRequest supporting dynamic runtime filters, multi-column sorting, pagination (start, limit),
// and parameter substitution (filterParams).
//
// Parameters:
//   - ctx (context.Context): Input value used by the operation.
//   - referenceName (string): Input value used by the operation.
//   - req (*domain.ExecuteRequest): Input value used by the operation.
//
// Returns:
//   - result1 ([]map[string]any): Returned value.
//   - result2 (error): Returned value.
//
// Business Rules:
//   - Follows the invariants documented by ExecuteWithOptions and its owning package.
//
// Side Effects:
//   - May update receiver state, persistent storage, or configured external dependencies.
//
// Errors:
//   - Returns validation, configuration, context, or underlying dependency errors.
//
// Note:
//   - The context controls cancellation and request lifetime.
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
	hasRuntimeFilterParams := false
	for i, p := range ds.FilterParams {
		var val any
		foundInPayload := false

		// Check if value came from payload (case-insensitive)
		for k, v := range runtimeParams {
			if strings.EqualFold(k, p.ParamName) && v != nil && v != "" {
				val = v
				foundInPayload = true
				hasRuntimeFilterParams = true
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

	// QUERY uses its saved default pipeline until the caller supplies filter
	// parameters. Runtime values use the saved reference template.
	baseQuery := ds.Pipeline
	if ds.SaveMode != domain.SaveModeQuery && ds.ReferencePipeline != "" {
		baseQuery = ds.ReferencePipeline
	}
	if ds.SaveMode == domain.SaveModeQuery && hasRuntimeFilterParams && ds.ReferencePipeline != "" {
		baseQuery = ds.ReferencePipeline
	}
	if baseQuery == "" {
		baseQuery = ds.ReferencePipeline
	}
	if strings.EqualFold(ds.Driver, "postgres") || strings.EqualFold(ds.Driver, "mysql") || ds.Driver == "" {
		// Resolve dynamic defaults in the saved pipeline on every call. The
		// reference pipeline gets the already-resolved effective values below.
		for i, p := range ds.FilterParams {
			if macro, ok := p.DefaultValue.(string); ok && (macro == "CD" || strings.HasPrefix(macro, "CD|") || strings.HasPrefix(macro, "KTON|")) {
				literal, literalErr := domain.SQLParameterLiteral(effectiveParams[i].Paramvalue, p.ParamDataType)
				if literalErr != nil {
					return nil, domain.WrapError(domain.ErrInvalidParameterType, "dataset parameter substitution failed", literalErr)
				}
				baseQuery = strings.ReplaceAll(baseQuery, "'"+strings.ReplaceAll(macro, "'", "''")+"'", literal)
			}
		}
		baseQuery, err = domain.RenderSQLFilterParams(effectiveParams, baseQuery)
		if err != nil {
			return nil, domain.WrapError(domain.ErrInvalidParameterType, "dataset parameter substitution failed", err)
		}
		if ds.SaveMode != domain.SaveModeQuery {
			baseQuery, err = domain.RenderSQLRoutineParams(effectiveParams, baseQuery)
			if err != nil {
				return nil, domain.WrapError(domain.ErrInvalidParameterType, "dataset routine parameter substitution failed", err)
			}
		}
	} else if ds.ReferencePipeline != "" && hasRuntimeFilterParams {
		baseQuery = domain.CreateFilterParams(effectiveParams, ds.ReferencePipeline, req.UserToken)
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
	if len(rows) == 0 && finalQuery != "" && execReq.Operation != operation.OpQuery {
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
	rows = s.normalizeAndDecryptDatasetRows(ctx, rows)
	if len(ds.SelectedList) > 0 {
		rows = projectDatasetRows(rows, ds)
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
	hasSort := req.Sort != nil && len(req.Sort) > 0
	hasDsSort := ds != nil && len(ds.Sort) > 0
	hasPaging := req.Start > 0 || req.Limit > 0

	if !hasFilter && !hasSort && !hasDsSort && !hasPaging && (ds == nil || len(ds.Filter) == 0) {
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
			if hasSort {
				stages = append(stages, map[string]any{"$sort": req.Sort})
			} else if hasDsSort {
				stages = append(stages, map[string]any{"$sort": ds.Sort})
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
			reqConditions = extractSQLConditionsFromFilter("_exec_sub", req.Filter)
		}

		var dsConditions []string
		if ds != nil && len(ds.Filter) > 0 {
			dsConditions = extractSQLConditionsFromFilter("_exec_sub", ds.Filter)
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
			if sortClause == "" && hasDsSort {
				sortClause = formatSQLSortClause("_exec_sub", ds.Sort)
			}
			if sortClause != "" {
				sb.WriteString("\nORDER BY ")
				sb.WriteString(sortClause)
			}
		} else if hasDsSort {
			sortClause := formatSQLSortClause("_exec_sub", ds.Sort)
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
//
// Parameters:
//   - baseSQL (string): Input value used by the operation.
//   - filter (map[string]any): Input value used by the operation.
//   - appendFilter (string): Input value used by the operation.
//
// Returns:
//   - result1 (string): Returned value.
//
// Business Rules:
//   - Follows the invariants documented by ApplyFilterToSQL and its owning package.
//
// Side Effects:
//   - May update receiver state, persistent storage, or configured external dependencies.
//
// Errors:
//   - This function does not return an error.
//
// Note:
//   - Callers should preserve the documented ownership and lifecycle of supplied values.
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
			if str, ok := item.(string); ok {
				if sc := formatSQLSortClause(alias, str); sc != "" {
					parts = append(parts, sc)
				}
				continue
			}
			if m, ok := item.(map[string]any); ok {
				f := fmt.Sprintf("%v", m["colId"])
				if f == "" || f == "<nil>" {
					f = fmt.Sprintf("%v", m["field"])
				}
				if f == "" || f == "<nil>" {
					f = fmt.Sprintf("%v", m["col"])
				}
				if f == "" || f == "<nil>" {
					f = fmt.Sprintf("%v", m["column"])
				}
				d := fmt.Sprintf("%v", m["sort"])
				if d == "" || d == "<nil>" {
					d = fmt.Sprintf("%v", m["dir"])
				}
				if d == "" || d == "<nil>" {
					d = fmt.Sprintf("%v", m["order"])
				}
				order := "ASC"
				if strings.EqualFold(d, "desc") || d == "-1" {
					order = "DESC"
				}
				if f != "" && f != "<nil>" {
					parts = append(parts, fmt.Sprintf("\"%s\".\"%s\" %s", alias, strings.ReplaceAll(f, "\"", ""), order))
				} else {
					for field, dir := range m {
						dStr := fmt.Sprintf("%v", dir)
						order := "ASC"
						if strings.EqualFold(dStr, "desc") || dStr == "-1" {
							order = "DESC"
						}
						parts = append(parts, fmt.Sprintf("\"%s\".\"%s\" %s", alias, strings.ReplaceAll(field, "\"", ""), order))
					}
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
//
//	Ensures parameter values match the declared data type before binding into queries or procedures.
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
//
//	Converts an abstract syntax tree (QueryAST) into executable queries and reference pipelines.
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

func parseDatasetArray(s string) ([]any, bool) {
	s = strings.TrimSpace(s)
	if len(s) < 2 || s[0] != '{' || s[len(s)-1] != '}' {
		return nil, false
	}
	content := s[1 : len(s)-1]
	if strings.TrimSpace(content) == "" {
		return []any{}, true
	}

	var result []any
	var current strings.Builder
	inQuotes := false
	escaped := false
	hasQuote := false

	for i := 0; i < len(content); i++ {
		ch := content[i]
		if escaped {
			current.WriteByte(ch)
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		if ch == '"' {
			inQuotes = !inQuotes
			hasQuote = true
			continue
		}
		if ch == ',' && !inQuotes {
			elem := current.String()
			if !hasQuote && strings.EqualFold(elem, "NULL") {
				result = append(result, nil)
			} else {
				result = append(result, elem)
			}
			current.Reset()
			hasQuote = false
			continue
		}
		current.WriteByte(ch)
	}

	if inQuotes {
		return nil, false
	}

	elem := current.String()
	if !hasQuote && strings.EqualFold(elem, "NULL") {
		result = append(result, nil)
	} else {
		result = append(result, elem)
	}

	return result, true
}

func normalizeDatasetValue(val any) any {
	switch v := val.(type) {
	case []byte:
		if len(v) == 0 {
			return ""
		}
		var obj map[string]any
		if json.Unmarshal(v, &obj) == nil {
			return obj
		}
		var arr []any
		if json.Unmarshal(v, &arr) == nil {
			return arr
		}
		s := string(v)
		if strings.HasPrefix(strings.TrimSpace(s), "{") && strings.HasSuffix(strings.TrimSpace(s), "}") {
			if pgArr, ok := parseDatasetArray(s); ok {
				return pgArr
			}
		}
		return s
	case string:
		trimmed := strings.TrimSpace(v)
		if len(trimmed) == 0 {
			return v
		}
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			var arr []any
			if json.Unmarshal([]byte(trimmed), &arr) == nil {
				return arr
			}
		}
		if strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}") {
			var obj map[string]any
			if json.Unmarshal([]byte(trimmed), &obj) == nil {
				return obj
			}
			if pgArr, ok := parseDatasetArray(trimmed); ok {
				return pgArr
			}
		}
		return v
	default:
		return val
	}
}

func (s *DataSetService) normalizeAndDecryptDatasetRows(ctx context.Context, rows []map[string]any) []map[string]any {
	key := s.DecryptionKey()
	for _, row := range rows {
		for k, v := range row {
			row[k] = s.decryptAndNormalizeValue(ctx, v, key)
		}
	}
	return rows
}

func (s *DataSetService) decryptAndNormalizeValue(ctx context.Context, val any, key string) any {
	if val == nil {
		return nil
	}
	switch v := val.(type) {
	case string:
		trimmed := strings.TrimSpace(v)
		if len(trimmed) == 0 {
			return v
		}
		if key != "" && isEncryptedString(trimmed) {
			decrypted, err := s.decryptString(ctx, trimmed, key)
			if err == nil {
				trimmed = strings.TrimSpace(decrypted)
				v = trimmed
			}
		}
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			var arr []any
			if json.Unmarshal([]byte(trimmed), &arr) == nil {
				for i, item := range arr {
					arr[i] = s.decryptAndNormalizeValue(ctx, item, key)
				}
				return arr
			}
		}
		if strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}") {
			var obj map[string]any
			if json.Unmarshal([]byte(trimmed), &obj) == nil {
				for k2, v2 := range obj {
					obj[k2] = s.decryptAndNormalizeValue(ctx, v2, key)
				}
				return obj
			}
			if pgArr, ok := parseDatasetArray(trimmed); ok {
				return pgArr
			}
		}
		return v
	case map[string]any:
		for k, item := range v {
			v[k] = s.decryptAndNormalizeValue(ctx, item, key)
		}
		return v
	case []map[string]any:
		for i, item := range v {
			for k, val2 := range item {
				item[k] = s.decryptAndNormalizeValue(ctx, val2, key)
			}
			v[i] = item
		}
		return v
	case []any:
		for i, item := range v {
			v[i] = s.decryptAndNormalizeValue(ctx, item, key)
		}
		return v
	default:
		return normalizeDatasetValue(val)
	}
}

func isEncryptedString(v string) bool {
	return strings.HasPrefix(v, "enc:") || strings.HasPrefix(v, "aes:")
}

func (s *DataSetService) decryptString(ctx context.Context, encrypted, key string) (string, error) {
	if key == "" || !isEncryptedString(encrypted) {
		return encrypted, nil
	}

	// 1. Check custom decryptor callback
	s.mu.RLock()
	customFn := s.decryptFunc
	s.mu.RUnlock()
	if customFn != nil {
		dec, err := customFn(ctx, encrypted, key)
		if err == nil {
			return dec, nil
		}
	}

	// 2. Built-in AES-256-GCM format ("aes:...")
	if strings.HasPrefix(encrypted, "aes:") {
		return decryptAESGCM(encrypted, key)
	}

	// 3. Built-in PostgreSQL PGP format ("enc:...") via SQL DB connection if available
	if strings.HasPrefix(encrypted, "enc:") && s.adapter != nil {
		if sqlDB, ok := s.adapter.NativeClient().(*sql.DB); ok && sqlDB != nil {
			cleanB64 := strings.ReplaceAll(encrypted[4:], " ", "+")
			cleanB64 = strings.ReplaceAll(cleanB64, "\n", "")
			cleanB64 = strings.ReplaceAll(cleanB64, "\r", "")
			cleanB64 = strings.TrimSpace(cleanB64)

			var decrypted string
			query := "SELECT iam.pgp_sym_decrypt(decode($1, 'base64'), $2)"
			err := sqlDB.QueryRowContext(ctx, query, cleanB64, key).Scan(&decrypted)
			if err == nil {
				return decrypted, nil
			}
			queryFallback := "SELECT pgp_sym_decrypt(decode($1, 'base64'), $2)"
			if err2 := sqlDB.QueryRowContext(ctx, queryFallback, cleanB64, key).Scan(&decrypted); err2 == nil {
				return decrypted, nil
			}
		}
	}

	return encrypted, nil
}

func decryptAESGCM(encrypted, key string) (string, error) {
	cleanAES := strings.ReplaceAll(encrypted[4:], " ", "+")
	cleanAES = strings.ReplaceAll(cleanAES, "\n", "")
	cleanAES = strings.ReplaceAll(cleanAES, "\r", "")
	cleanAES = strings.TrimSpace(cleanAES)

	data, err := base64.StdEncoding.DecodeString(cleanAES)
	if err != nil {
		return "", fmt.Errorf("decryptAESGCM: base64 decode: %w", err)
	}

	h := sha256.Sum256([]byte(key))
	aesKey := h[:]

	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return "", fmt.Errorf("decryptAESGCM: new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("decryptAESGCM: new GCM: %w", err)
	}
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("decryptAESGCM: ciphertext too short")
	}
	plainBytes, err := gcm.Open(nil, data[:nonceSize], data[nonceSize:], nil)
	if err != nil {
		return "", fmt.Errorf("decryptAESGCM: decrypt failed: %w", err)
	}
	return string(plainBytes), nil
}

func normalizeDatasetRows(rows []map[string]any) []map[string]any {
	for _, row := range rows {
		for k, v := range row {
			row[k] = normalizeDatasetValue(v)
		}
	}
	return rows
}

// extractSQLConditionsFromFilter translates a []any filter slice into SQL WHERE condition strings.
func extractSQLConditionsFromFilter(alias string, filter []any) []string {
	var conditions []string
	for _, item := range filter {
		if item == nil {
			continue
		}
		if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
			conditions = append(conditions, strings.TrimSpace(s))
			continue
		}
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}

		// Case A: Group condition with "conditions": [...]
		if rawConds, hasConds := m["conditions"]; hasConds {
			if condSlice, isSlice := rawConds.([]any); isSlice && len(condSlice) > 0 {
				sub := extractSQLConditionsFromFilter(alias, condSlice)
				if len(sub) > 0 {
					clause := "AND"
					if c, hasClause := m["clause"].(string); hasClause && strings.EqualFold(c, "OR") {
						clause = "OR"
					}
					conditions = append(conditions, "("+strings.Join(sub, " "+clause+" ")+")")
				}
				continue
			}
		}

		// Case B: Explicit condition object: {"column": "status", "operator": "EQUALS", "value": "active"}
		colRaw, hasCol := m["column"]
		if !hasCol {
			colRaw, hasCol = m["field"]
		}
		if !hasCol {
			colRaw, hasCol = m["colId"]
		}
		if hasCol {
			colStr := fmt.Sprintf("%v", colRaw)
			opStr := "EQUALS"
			if op, hasOp := m["operator"].(string); hasOp && op != "" {
				opStr = op
			} else if op, hasOp := m["type"].(string); hasOp && op != "" {
				opStr = op
			}
			val := m["value"]
			if val == nil {
				val = m["filter"]
			}

			cond := formatSQLFilterWithOperator(alias, colStr, opStr, val)
			if cond != "" {
				conditions = append(conditions, cond)
			}
			continue
		}

		// Case C: Standard key-value dictionary: {"status": "active", ...}
		var cols []string
		for col := range m {
			cols = append(cols, col)
		}
		sort.Strings(cols)
		for _, col := range cols {
			cond := formatSQLFilterCondition(alias, col, m[col])
			if cond != "" {
				conditions = append(conditions, cond)
			}
		}
	}
	return conditions
}

// formatSQLFilterWithOperator formats a single condition with custom operator.
func formatSQLFilterWithOperator(alias, col, op string, val any) string {
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

	upperOp := strings.ToUpper(strings.TrimSpace(op))
	switch upperOp {
	case "EQUALS", "=", "EQ":
		if val == nil {
			return fmt.Sprintf("%s IS NULL", colExpr)
		}
		return fmt.Sprintf("%s = '%v'", colExpr, val)
	case "NOTEQUALS", "!=", "<>", "NEQ":
		if val == nil {
			return fmt.Sprintf("%s IS NOT NULL", colExpr)
		}
		return fmt.Sprintf("%s != '%v'", colExpr, val)
	case "CONTAINS", "LIKE":
		return fmt.Sprintf("%s ILIKE '%%%v%%'", colExpr, val)
	case "NOTCONTAINS", "NOT_LIKE":
		return fmt.Sprintf("%s NOT ILIKE '%%%v%%'", colExpr, val)
	case "STARTSWITH":
		return fmt.Sprintf("%s ILIKE '%v%%'", colExpr, val)
	case "ENDSWITH":
		return fmt.Sprintf("%s ILIKE '%%%v'", colExpr, val)
	case "GREATERTHAN", ">", "GT":
		return fmt.Sprintf("%s > %v", colExpr, val)
	case "GREATERTHANOREQUAL", ">=", "GTE":
		return fmt.Sprintf("%s >= %v", colExpr, val)
	case "LESSTHAN", "<", "LT":
		return fmt.Sprintf("%s < %v", colExpr, val)
	case "LESSTHANOREQUAL", "<=", "LTE":
		return fmt.Sprintf("%s <= %v", colExpr, val)
	case "ISNULL", "BLANK":
		return fmt.Sprintf("%s IS NULL", colExpr)
	case "ISNOTNULL", "NOTBLANK":
		return fmt.Sprintf("%s IS NOT NULL", colExpr)
	case "IN":
		if arr, ok := val.([]any); ok {
			var inItems []string
			for _, item := range arr {
				inItems = append(inItems, fmt.Sprintf("'%v'", item))
			}
			return fmt.Sprintf("%s IN (%s)", colExpr, strings.Join(inItems, ", "))
		}
		return fmt.Sprintf("%s = '%v'", colExpr, val)
	case "NOTIN", "NOT_IN":
		if arr, ok := val.([]any); ok {
			var inItems []string
			for _, item := range arr {
				inItems = append(inItems, fmt.Sprintf("'%v'", item))
			}
			return fmt.Sprintf("%s NOT IN (%s)", colExpr, strings.Join(inItems, ", "))
		}
		return fmt.Sprintf("%s != '%v'", colExpr, val)
	default:
		return formatSQLFilterCondition(alias, col, val)
	}
}

// projNode represents a node in the field projection tree for pruning unselected fields.
type projNode struct {
	includeAll bool
	children   map[string]*projNode
	alias      string
}

func newProjNode() *projNode {
	return &projNode{
		children: make(map[string]*projNode),
	}
}

// buildProjectionTree constructs an AST of selected field paths from ds.SelectedList.
func buildProjectionTree(ds *domain.DataSet) *projNode {
	root := newProjNode()
	if ds == nil || len(ds.SelectedList) == 0 {
		return root
	}

	baseColl := strings.ToLower(ds.BaseCollection.Collection)

	for _, sel := range ds.SelectedList {
		field := strings.TrimSpace(sel.Field)
		if field == "" {
			field = strings.TrimSpace(sel.HeaderName)
		}
		if field == "" {
			continue
		}

		parts := strings.Split(field, ".")
		// If path starts with base collection name (e.g. "vehicles.status"),
		// index both "vehicles.status" and "status" so either flat or nested match succeeds.
		if len(parts) > 1 && baseColl != "" && strings.EqualFold(parts[0], baseColl) {
			addPathToTree(root, parts, sel.HeaderName)
			addPathToTree(root, parts[1:], sel.HeaderName)
		} else {
			addPathToTree(root, parts, sel.HeaderName)
		}

		// Also index by HeaderName if provided
		if sel.HeaderName != "" && !strings.EqualFold(sel.HeaderName, field) {
			addPathToTree(root, []string{sel.HeaderName}, sel.HeaderName)
			// SQL JSON projections may already use the selected alias as the
			// nested key. Keep that path as well as the original field path;
			// otherwise a successful relationship becomes an empty object.
			if len(parts) > 1 {
				aliasedParts := append([]string(nil), parts[:len(parts)-1]...)
				aliasedParts = append(aliasedParts, sel.HeaderName)
				addPathToTree(root, aliasedParts, sel.HeaderName)
				if baseColl != "" && strings.EqualFold(parts[0], baseColl) && len(aliasedParts) > 2 {
					addPathToTree(root, aliasedParts[1:], sel.HeaderName)
				}
			}
		}
	}

	return root
}

func addPathToTree(root *projNode, parts []string, alias string) {
	curr := root
	for i, part := range parts {
		key := strings.ToLower(part)
		child, exists := curr.children[key]
		if !exists {
			child = newProjNode()
			curr.children[key] = child
		}
		if i == len(parts)-1 {
			child.includeAll = true
			if alias != "" {
				child.alias = alias
			}
		}
		curr = child
	}
}

// projectDatasetRows prunes unselected fields from tabular rows according to ds.SelectedList.
// If a selected field targets a nested object (e.g. "driver.name") or an array of objects
// (e.g. "childarr.id", "childarr.name"), only the selected fields within the object or array
// are returned, stripping away unselected fields.
func projectDatasetRows(rows []map[string]any, ds *domain.DataSet) []map[string]any {
	if len(rows) == 0 || ds == nil || len(ds.SelectedList) == 0 {
		return rows
	}

	tree := buildProjectionTree(ds)
	if len(tree.children) == 0 {
		return rows
	}

	result := make([]map[string]any, len(rows))
	for i, row := range rows {
		result[i] = projectMap(row, tree)
	}
	return result
}

func projectMap(m map[string]any, node *projNode) map[string]any {
	if m == nil {
		return nil
	}
	if node.includeAll && len(node.children) == 0 {
		return m
	}

	result := make(map[string]any)
	for k, v := range m {
		childNode, ok := node.children[strings.ToLower(k)]
		if !ok {
			continue
		}

		outKey := k
		if childNode.alias != "" && len(childNode.children) == 0 && !strings.Contains(childNode.alias, ".") {
			outKey = childNode.alias
		}

		if childNode.includeAll && len(childNode.children) == 0 {
			result[outKey] = v
			continue
		}

		result[outKey] = projectValue(v, childNode)
	}
	return result
}

func projectValue(val any, node *projNode) any {
	if val == nil {
		return nil
	}
	switch v := val.(type) {
	case map[string]any:
		return projectMap(v, node)
	case []map[string]any:
		res := make([]map[string]any, len(v))
		for i, item := range v {
			res[i] = projectMap(item, node)
		}
		return res
	case []any:
		res := make([]any, len(v))
		for i, item := range v {
			if itemMap, ok := item.(map[string]any); ok {
				res[i] = projectMap(itemMap, node)
			} else {
				res[i] = item
			}
		}
		return res
	default:
		return v
	}
}

