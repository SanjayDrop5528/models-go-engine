package service

import (
	"context"
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

// NewDataSetService creates a new DataSetService.
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
func (s *DataSetService) RegisterCompiler(driver string, c compiler.DataSetCompiler) *DataSetService {
	s.compilers[strings.ToLower(driver)] = c
	return s
}

// Preview compiles and executes the dataset without saving it to database metadata.
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
			Arguments: map[string]any{"preview": true},
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
	}, nil
}

// Save validates, compiles pipelines, executes DDL (procedures/functions), and persists dataset metadata.
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
func (s *DataSetService) Execute(ctx context.Context, referenceName string, runtimeParams map[string]any) ([]map[string]any, error) {
	ds, err := s.repo.FindByReferenceName(ctx, referenceName)
	if err != nil {
		return nil, err
	}

	// 1. Validate & Coerce Parameters
	boundArgs := make(map[string]any)
	for _, p := range ds.FilterParams {
		val, provided := runtimeParams[p.ParamName]
		if !provided || val == nil {
			if p.Required && p.DefaultValue == nil {
				return nil, domain.NewErrorf(domain.ErrMissingRequiredParameter, "required parameter '%s' was not provided", p.ParamName)
			}
			val = p.DefaultValue
		}

		if val != nil {
			coerced, err := coerceDataType(val, p.ParamDataType)
			if err != nil {
				return nil, domain.WrapError(domain.ErrInvalidParameterType, fmt.Sprintf("parameter '%s' coercion failed", p.ParamName), err)
			}
			boundArgs[p.ParamName] = coerced
		}
	}

	// 2. Select Pipeline (ReferencePipeline if parameters present, else Pipeline)
	queryToRun := ds.Pipeline
	if len(runtimeParams) > 0 && ds.ReferencePipeline != "" {
		queryToRun = ds.ReferencePipeline
	}

	// 3. Execute via Adapter
	if s.adapter == nil {
		return []map[string]any{}, nil
	}

	execReq := execution.ExecutionRequest{
		Operation: operation.OpQuery,
		Target:    queryToRun,
		Arguments: boundArgs,
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
		}
	}

	return []map[string]any{}, nil
}

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
