# models-go-engine

> **Core Query AST Planner, Validation & Execution Engine for DataModel-Adapter**

`models-go-engine` contains the core domain models, AST query planner, validator, function resolver, repository interfaces, and execution pipeline for multi-database operations.

---

## 🏛️ Core Package Architecture

```
models-go-engine/
├── adapter/        # Core Database Adapter & Transaction Interfaces
├── dataset/
│   ├── compiler/   # DataSetCompiler interface & Pipeline definitions
│   ├── domain/     # DataSet, JoinCollection, SelectedField, FilterParam models
│   ├── planner/    # QueryAST Builder & Relational Planner
│   ├── repository/ # Repository interfaces (In-Memory & SQL backends)
│   ├── resolver/   # Model, Field & Function Resolver interfaces
│   ├── service/    # DataSetService (Preview, Save, Execute)
│   └── validator/  # DataSet & Field Validator
├── execution/      # ExecutionRequest & ExecutionResult definitions
├── operation/      # CRUD & DDL Operation Constants (OpCreateTable, OpSelect, etc.)
└── plan/           # SchemaPlan, OperationDiff & Migration Plan models
```

---

## 🛠️ Key Exported Functions & Methods Reference

### 1. `service.DataSetService` ([`dataset/service/dataset_service.go`](./dataset/service/dataset_service.go))

Orchestrates design-time preview/save operations and runtime execution.

| Function / Method | Signature | Description |
| :--- | :--- | :--- |
| `NewDataSetService` | `(repo, mr, fr, fnr, adapter) *DataSetService` | Creates a new dataset engine service instance. |
| `RegisterCompiler` | `(driver string, compiler DataSetCompiler)` | Registers a dialect compiler (e.g. `postgres`, `mysql`). |
| `Preview` | `(ctx context.Context, ds *domain.DataSet) (*PreviewResponse, error)` | Validates, builds AST, compiles SQL, and executes sample rows without saving. |
| `Save` | `(ctx context.Context, ds *domain.DataSet) (*domain.DataSet, error)` | Validates, compiles DDL (Procedures/Functions), executes DDL against target DB, and persists metadata. |
| `Execute` | `(ctx context.Context, refName string, runtimeParams map[string]any) ([]map[string]any, error)` | Executes a saved dataset by reference name with runtime parameter bindings. |

---

### 2. `planner.DataSetPlanner` ([`dataset/planner/planner.go`](./dataset/planner/planner.go))

Transforms domain `DataSet` rules into a clean `QueryAST`.

| Function / Method | Signature | Description |
| :--- | :--- | :--- |
| `NewPlanner` | `(fnResolver resolver.FunctionResolver) *DataSetPlanner` | Creates a new AST planner. |
| `BuildAST` | `(ctx context.Context, ds *domain.DataSet) (*QueryAST, error)` | Compiles base collection, joins, filters, custom columns, group by fields, and projections into `QueryAST`. |

---

### 3. `validator.DataSetValidator` ([`dataset/validator/validator.go`](./dataset/validator/validator.go))

Validates dataset configuration against target model schemas.

| Function / Method | Signature | Description |
| :--- | :--- | :--- |
| `NewDataSetValidator` | `(mr ModelResolver, fr FieldResolver, fnr FunctionResolver)` | Creates a new dataset validator. |
| `Validate` | `(ctx context.Context, ds *domain.DataSet) error` | Validates table existence, column existence, join keys, and parameter requirements. |

---

## 💡 Code Usage Example

```go
package main

import (
	"context"
	"fmt"

	"github.com/SanjayDrop5528/models-go-engine/dataset/domain"
	"github.com/SanjayDrop5528/models-go-engine/dataset/planner"
)

func main() {
	ctx := context.Background()

	ds := &domain.DataSet{
		Name:          "Sales Report",
		ReferenceName: "sales_report",
		BaseCollection: domain.BaseCollection{
			Collection: "orders",
		},
		SelectedList: []domain.SelectedField{
			{Field: "orders.id", HeaderName: "id", DataType: "int"},
			{Field: "orders.total", HeaderName: "total", DataType: "decimal"},
		},
	}

	p := planner.NewPlanner(nil)
	ast, err := p.BuildAST(ctx, ds)
	if err != nil {
		panic(err)
	}

	fmt.Println("Base Table:", ast.BaseTable.Table)
	fmt.Println("Projections Count:", len(ast.Projections))
}
```
