// Package compiler defines the interfaces and compilation result containers
// for converting dataset execution ASTs into database-native queries or routines.
//
// File: compiler.go
// Usage:
//   This file provides the universal DataSetCompiler interface implemented by all database
//   adapters (PostgreSQL, MySQL, MongoDB, and In-Memory). Compilers receive a validated
//   QueryAST and generate both an executable query/routine and a reference query template
//   with standardized parameter tokens ({"paramName":"...","paramDataType":"..."}).
package compiler

import (
	"context"

	"github.com/SanjayDrop5528/models-go-engine/dataset/domain"
	"github.com/SanjayDrop5528/models-go-engine/dataset/planner"
)

// CompiledPipeline contains the compilation output for a dataset.
type CompiledPipeline struct {
	ExecutableQuery   string               `json:"executable_query"`   // Pure pipeline/SQL without runtime params
	ReferencePipeline string               `json:"reference_pipeline"` // Parameterized pipeline/SQL with $1/? bindings
	Parameters        []domain.FilterParam `json:"parameters"`         // Parameter metadata
	DDLStatement      string               `json:"ddl_statement"`      // CREATE PROCEDURE / FUNCTION statement
	SaveMode          domain.SaveMode      `json:"save_mode"`
	Driver            string               `json:"driver"`
}

// DataSetCompiler abstracts converting QueryAST into target database code.
//
// Purpose:
//   Defines the database-specific compilation contract: transforming a vendor-agnostic
//   QueryAST tree into executable SQL, MongoDB aggregation stages, or in-memory pipelines.
//
// Where it is used:
//   - Implemented by PostgresDataSetCompiler (models-go-postgres)
//   - Implemented by MySQLDataSetCompiler (models-go-mysql)
//   - Implemented by MongoDataSetCompiler (models-go-mongodb)
//   - Implemented by MemoryDataSetCompiler (models-go-memory)
//   - Consumed by DataSetService in Preview, Save, and Execute workflows.
//
// When can it be used:
//   - When preparing a dataset for live execution or saving as a persistent database procedure.
type DataSetCompiler interface {
	// Compile transforms a planned QueryAST into target database executable queries or routines.
	Compile(ctx context.Context, ast *planner.QueryAST, ds *domain.DataSet) (*CompiledPipeline, error)
}

