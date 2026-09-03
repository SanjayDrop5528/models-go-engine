package compiler

import (
	"context"
	"github.com/SanjayDrop5528/models-go-engine/dataset/domain"
	"github.com/SanjayDrop5528/models-go-engine/dataset/planner"
)

// CompiledPipeline contains the compilation output for a dataset.
type CompiledPipeline struct {
	ExecutableQuery   string                `json:"executable_query"`   // Pure pipeline/SQL without runtime params
	ReferencePipeline string                `json:"reference_pipeline"` // Parameterized pipeline/SQL with $1/? bindings
	Parameters        []domain.FilterParam  `json:"parameters"`         // Parameter metadata
	DDLStatement      string                `json:"ddl_statement"`      // CREATE PROCEDURE / FUNCTION statement
	SaveMode          domain.SaveMode       `json:"save_mode"`
	Driver            string                `json:"driver"`
}

// DataSetCompiler abstracts converting QueryAST into target database code.
type DataSetCompiler interface {
	Compile(ctx context.Context, ast *planner.QueryAST, ds *domain.DataSet) (*CompiledPipeline, error)
}
