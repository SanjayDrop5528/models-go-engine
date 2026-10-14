# models-go-engine

> **Core Query AST Planner, Validation & Execution Engine for DataModel-Adapter**

`models-go-engine` contains the core domain models, AST query planner, validator, function resolver, repository interfaces, and execution pipeline for multi-database operations.

## Prerequisites

- Go 1.20 or newer.
- A Go workspace that can resolve this module and its adapter dependencies.
- A model, field, and function resolver when using dataset validation.

## Usage Steps

1. From this directory, run `go test ./...`.
2. Create a `domain.DataSet` with a base collection, selected fields, joins, filters, and parameters.
3. Validate it with `dataset/validator`.
4. Build a database-independent `QueryAST` with `dataset/planner`.
5. Register a dialect compiler and call `Preview`, `Save`, or `Execute` through `dataset/service`.

## Core Functionality

- Dataset validation and parameter checking.
- Dialect-independent query AST planning.
- Ordered joins, projections, grouping, filtering, and custom functions.
- Preview, persistence, and runtime execution orchestration.
- Common adapter, execution, operation, and schema-plan contracts.

## Using the Validation Engine Alone

The validation package is independent of PostgreSQL, MySQL, MongoDB, and the dataset service. Use it directly when an application only needs model, payload, partial-update, query, or schema-plan validation.

```go
package main

import (
    "fmt"

    "github.com/SanjayDrop5528/models-go-engine/model"
    "github.com/SanjayDrop5528/models-go-engine/validation"
)

func main() {
    employee := &model.Model{
        Name:        "employees",
        StorageName: "employees",
        StorageType: model.StorageRelational,
        PrimaryKey:  &model.PrimaryKey{Columns: []string{"id"}},
        Attributes: []model.Attribute{
            {Name: "id", Type: model.TypeUUID, IsPrimaryKey: true},
            {
                Name: "email",
                Type: model.TypeEmail,
                Validation: &model.RuleSet{Required: true},
            },
        },
    }

    validator := validation.NewValidationEngine()
    if err := validator.ValidateModel(employee); err != nil {
        panic(err)
    }

    payload := map[string]any{
        "id":    "123e4567-e89b-12d3-a456-426614174000",
        "email": "employee@example.com",
    }
    if err := validator.ValidateData(employee, payload); err != nil {
        fmt.Println("invalid payload:", err)
        return
    }

    fmt.Println("payload is valid")
}
```

For partial updates, call `ValidateUpdateData`. The stateless helpers `validation.ValidateModel`, `validation.ValidateData`, `validation.ValidatePartialData`, and `validation.ValidatePlan` can also be called without constructing an engine. An adapter is only needed for `ValidateOrbitalReferences`, where the validator must check that referenced records exist.

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
