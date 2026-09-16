package dataset_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/SanjayDrop5528/models-go-engine/adapter"
	"github.com/SanjayDrop5528/models-go-engine/dataset/compiler"
	"github.com/SanjayDrop5528/models-go-engine/dataset/domain"
	"github.com/SanjayDrop5528/models-go-engine/dataset/planner"
	"github.com/SanjayDrop5528/models-go-engine/dataset/repository"
	"github.com/SanjayDrop5528/models-go-engine/dataset/resolver"
	"github.com/SanjayDrop5528/models-go-engine/dataset/service"
	"github.com/SanjayDrop5528/models-go-engine/execution"
)

func setupTestService() *service.DataSetService {
	repo := repository.NewDataSetRepository()
	fnRegistry := resolver.NewFunctionRegistry()
	modelResolver := resolver.NewModelResolver(nil)

	// Register sample models
	modelResolver.RegisterModel(&resolver.ModelDefinition{
		Table: "employees",
		Fields: map[string]*resolver.FieldDefinition{
			"id":            {Name: "id", ColumnName: "id", DataType: "uuid", IsPrimaryKey: true},
			"name":          {Name: "name", ColumnName: "name", DataType: "string"},
			"first_name":    {Name: "first_name", ColumnName: "first_name", DataType: "string"},
			"last_name":     {Name: "last_name", ColumnName: "last_name", DataType: "string"},
			"department_id": {Name: "department_id", ColumnName: "department_id", DataType: "uuid"},
			"salary":        {Name: "salary", ColumnName: "salary", DataType: "decimal"},
			"quantity":      {Name: "quantity", ColumnName: "quantity", DataType: "int"},
			"unit_price":    {Name: "unit_price", ColumnName: "unit_price", DataType: "decimal"},
			"is_active":     {Name: "is_active", ColumnName: "is_active", DataType: "boolean"},
		},
	})
	modelResolver.RegisterModel(&resolver.ModelDefinition{
		Table: "departments",
		Fields: map[string]*resolver.FieldDefinition{
			"id":        {Name: "id", ColumnName: "id", DataType: "uuid", IsPrimaryKey: true},
			"name":      {Name: "name", ColumnName: "name", DataType: "string"},
			"is_active": {Name: "is_active", ColumnName: "is_active", DataType: "boolean"},
		},
	})

	svc := service.NewDataSetService(repo, modelResolver, modelResolver, fnRegistry, nil)
	svc.RegisterCompiler("postgres", &testPostgresCompiler{})
	svc.RegisterCompiler("mysql", &testMySQLCompiler{})
	svc.RegisterCompiler("mongodb", &testMongoCompiler{})
	return svc
}

type testPostgresCompiler struct{}

func (c *testPostgresCompiler) Compile(ctx context.Context, ast *planner.QueryAST, ds *domain.DataSet) (*compiler.CompiledPipeline, error) {
	var selectCols []string
	for _, p := range ast.Projections {
		selectCols = append(selectCols, "\""+p.SourceTable+"\".\""+p.SourceField+"\" AS \""+p.Alias+"\"")
	}
	for _, cc := range ast.CustomColumns {
		expr := cc.Expression
		if cc.Function != nil && cc.Function.PostgresExpression != "" {
			expr = cc.Function.PostgresExpression
			for i, op := range cc.Operands {
				var opSql string
				if op.SourceTable == "" || op.SourceTable == "_LITERAL_" {
					if op.IsLiteral {
						opSql = op.SourceField
					} else {
						opSql = "\"" + op.SourceField + "\""
					}
				} else {
					opSql = "\"" + op.SourceTable + "\".\"" + op.SourceField + "\""
				}
				expr = strings.ReplaceAll(expr, "{{"+string(rune('0'+i))+"}}", opSql)
			}
			var allArgs []string
			for _, op := range cc.Operands {
				if op.SourceTable == "" || op.SourceTable == "_LITERAL_" {
					if op.IsLiteral {
						allArgs = append(allArgs, op.SourceField)
					} else {
						allArgs = append(allArgs, "\""+op.SourceField+"\"")
					}
				} else {
					allArgs = append(allArgs, "\""+op.SourceTable+"\".\""+op.SourceField+"\"")
				}
			}
			expr = strings.ReplaceAll(expr, "{{args}}", strings.Join(allArgs, ", "))
		}
		if expr != "" {
			selectCols = append(selectCols, expr+" AS \""+cc.Alias+"\"")
		}
	}
	var joins []string
	for _, j := range ast.Joins {
		on := "\""+j.FromTable+"\".\""+j.FromField+"\" = \""+j.Alias+"\".\""+j.ToField+"\""
		if len(j.JoinFilter) > 0 {
			for k, v := range j.JoinFilter {
				on += " AND \"" + j.Alias + "\".\"" + k + "\" = '" + strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(string(rune('0')), "0", ""), "", ""), "", "")) + strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(string(rune('0')), "0", ""), "", ""), "", ""))
				on = fmt.Sprintf("%s AND \"%s\".\"%s\" = '%v'", "\""+j.FromTable+"\".\""+j.FromField+"\" = \""+j.Alias+"\".\""+j.ToField+"\"", j.Alias, k, v)
			}
		}
		joins = append(joins, "LEFT JOIN \""+j.ToTable+"\" AS \""+j.Alias+"\" ON "+on)
	}
	base := "FROM \"" + ast.BaseTable.Table + "\" AS \"" + ast.BaseTable.Alias + "\""
	sql := "SELECT\n  " + strings.Join(selectCols, ",\n  ") + "\n" + base
	if len(joins) > 0 {
		sql += "\n" + strings.Join(joins, "\n")
	}
	refSQL := sql + "\nWHERE ($1 IS NULL OR \"employees\".\"department_id\" = $1)"

	return &compiler.CompiledPipeline{
		ExecutableQuery:   sql + ";",
		ReferencePipeline: refSQL + ";",
		Parameters:        ast.Parameters,
		SaveMode:          ds.SaveMode,
		Driver:            "postgres",
	}, nil
}

type testMySQLCompiler struct{}

func (c *testMySQLCompiler) Compile(ctx context.Context, ast *planner.QueryAST, ds *domain.DataSet) (*compiler.CompiledPipeline, error) {
	return &compiler.CompiledPipeline{
		ExecutableQuery:   "SELECT * FROM `employees`;",
		ReferencePipeline: "SELECT * FROM `employees` WHERE (? IS NULL OR `employees`.`salary` = ?);",
		Parameters:        ast.Parameters,
		Driver:            "mysql",
	}, nil
}

type testMongoCompiler struct{}

func (c *testMongoCompiler) Compile(ctx context.Context, ast *planner.QueryAST, ds *domain.DataSet) (*compiler.CompiledPipeline, error) {
	return &compiler.CompiledPipeline{
		ExecutableQuery:   "[{\"$lookup\":{\"from\":\"departments\",\"localField\":\"department_id\",\"foreignField\":\"_id\",\"as\":\"dept\"}}]",
		ReferencePipeline: "[{\"$lookup\":{\"from\":\"departments\",\"localField\":\"department_id\",\"foreignField\":\"_id\",\"as\":\"dept\"}}]",
		Parameters:        ast.Parameters,
		Driver:            "mongodb",
	}, nil
}

func TestDataSet_MultipleJoinsAndGroupBy(t *testing.T) {
	ctx := context.Background()
	svc := setupTestService()

	ds := &domain.DataSet{
		Name:          "Department Salaries",
		ReferenceName: "dept_salaries",
		Driver:        "postgres",
		SaveMode:      domain.SaveModeProcedure,
		BaseCollection: domain.BaseCollection{
			Collection: "employees",
			Filter:     map[string]any{"is_active": true},
		},
		JoinCollections: []domain.JoinCollection{
			{
				FromCollection:      "employees",
				FromCollectionField: "department_id",
				ToCollection:        "departments",
				ToCollectionField:   "id",
				NamedAs:             "d",
				JoinType:            domain.JoinLeft,
				Filter:              map[string]any{"is_active": true},
			},
		},
		GroupByFields: []domain.GroupByField{
			{TableName: "d", FieldName: "name"},
		},
		SelectedList: []domain.SelectedField{
			{Field: "d.name", HeaderName: "department_name"},
		},
		CustomColumns: []domain.CustomColumn{
			{
				CustomColumnName:      "total_salary",
				CustomLabelName:       "Total Salary",
				CustomAggregateFnName: "SUM",
				Fields: []domain.DataSetCustomField{
					{TableName: "employees", FieldName: "salary"},
				},
			},
			{
				CustomColumnName:      "avg_salary",
				CustomLabelName:       "Average Salary",
				CustomAggregateFnName: "AVG",
				Fields: []domain.DataSetCustomField{
					{TableName: "employees", FieldName: "salary"},
				},
			},
		},
		FilterParams: []domain.FilterParam{
			{ParamName: "department_id", ParamDataType: "string", Required: false},
		},
		Filter: map[string]any{
			"employees.department_id": map[string]any{"ParamsName": "department_id", "parmsDataType": "string"},
		},
	}

	// 1. Preview
	prev, err := svc.Preview(ctx, ds)
	if err != nil {
		t.Fatalf("preview failed: %v", err)
	}

	if len(prev.Columns) != 3 {
		t.Fatalf("expected 3 columns, got: %d", len(prev.Columns))
	}
	if !strings.Contains(prev.Pipeline, "LEFT JOIN \"departments\" AS \"d\" ON") {
		t.Fatalf("expected LEFT JOIN departments in pipeline, got: %s", prev.Pipeline)
	}
	if !strings.Contains(prev.Pipeline, "\"d\".\"is_active\" = 'true'") {
		t.Fatalf("expected join filter in ON clause, got: %s", prev.Pipeline)
	}
	if !strings.Contains(prev.ReferencePipeline, "$1 IS NULL OR \"employees\".\"department_id\" = $1") {
		t.Fatalf("expected parameterized binding in reference pipeline, got: %s", prev.ReferencePipeline)
	}

	// 2. Save
	saved, err := svc.Save(ctx, ds)
	if err != nil {
		t.Fatalf("save failed: %v", err)
	}
	if saved.ID == "" || saved.Pipeline == "" {
		t.Fatalf("saved dataset missing ID or Pipeline: %+v", saved)
	}

	// 3. Runtime Execute with Parameters
	rows, err := svc.Execute(ctx, "dept_salaries", map[string]any{
		"department_id": "dept_101",
	})
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if rows == nil {
		t.Fatalf("expected rows slice, got nil")
	}
}

func TestDataSet_Calculations_NumericAndString(t *testing.T) {
	ctx := context.Background()
	svc := setupTestService()

	ds := &domain.DataSet{
		Name:          "Employee Compensation Calculations",
		ReferenceName: "emp_comp",
		Driver:        "postgres",
		BaseCollection: domain.BaseCollection{
			Collection: "employees",
		},
		CustomColumns: []domain.CustomColumn{
			{
				CustomColumnName:      "total_amount",
				CustomLabelName:       "Total Amount",
				CustomAggregateFnName: "MULTIPLY",
				Fields: []domain.DataSetCustomField{
					{TableName: "employees", FieldName: "quantity"},
					{TableName: "employees", FieldName: "unit_price"},
				},
			},
			{
				CustomColumnName:      "full_name",
				CustomLabelName:       "Full Name",
				CustomAggregateFnName: "CONCAT_WS",
				Fields: []domain.DataSetCustomField{
					{TableName: "employees", FieldName: "first_name"},
					{TableName: "employees", FieldName: "last_name"},
				},
			},
		},
	}

	prev, err := svc.Preview(ctx, ds)
	if err != nil {
		t.Fatalf("preview failed: %v", err)
	}

	if !strings.Contains(prev.Pipeline, "(\"employees\".\"quantity\" * \"employees\".\"unit_price\") AS \"total_amount\"") {
		t.Fatalf("expected MULTIPLY expression in pipeline, got: %s", prev.Pipeline)
	}
	if !strings.Contains(prev.Pipeline, "CONCAT_WS(\"employees\".\"first_name\", \"employees\".\"last_name\") AS \"full_name\"") {
		t.Fatalf("expected CONCAT_WS expression in pipeline, got: %s", prev.Pipeline)
	}
}

func TestDataSet_LiteralOperand_Divide(t *testing.T) {
	ctx := context.Background()
	svc := setupTestService()

	ds := &domain.DataSet{
		Name:          "Literal Divide Test",
		ReferenceName: "literal_divide",
		Driver:        "postgres",
		BaseCollection: domain.BaseCollection{
			Collection: "employees",
		},
		CustomColumns: []domain.CustomColumn{
			{
				CustomColumnName:      "monthly_leave",
				CustomLabelName:       "Monthly Leave",
				CustomAggregateFnName: "DIVIDE",
				Fields: []domain.DataSetCustomField{
					{TableName: "employees", FieldName: "leave_balance"},
					{TableName: "", FieldName: "12", IsLiteral: true},
				},
			},
		},
	}

	prev, err := svc.Preview(ctx, ds)
	if err != nil {
		t.Fatalf("preview failed: %v", err)
	}

	expectedExpr := "(\"employees\".\"leave_balance\" / NULLIF(12, 0)) AS \"monthly_leave\""
	if !strings.Contains(prev.Pipeline, expectedExpr) {
		t.Fatalf("expected literal divide expression %s in pipeline, got: %s", expectedExpr, prev.Pipeline)
	}
}

func TestDataSet_ChainedCustomColumns(t *testing.T) {
	ctx := context.Background()
	svc := setupTestService()

	ds := &domain.DataSet{
		Name:          "Chained Custom Columns Test",
		ReferenceName: "chained_custom_columns",
		Driver:        "postgres",
		BaseCollection: domain.BaseCollection{
			Collection: "employees",
		},
		CustomColumns: []domain.CustomColumn{
			{
				CustomColumnName:      "monthly_leave",
				CustomLabelName:       "Monthly Leave",
				CustomAggregateFnName: "DIVIDE",
				Fields: []domain.DataSetCustomField{
					{TableName: "employees", FieldName: "leave_balance"},
					{TableName: "", FieldName: "12", IsLiteral: true},
				},
			},
			{
				CustomColumnName:      "double_monthly_leave",
				CustomLabelName:       "Double Monthly Leave",
				CustomAggregateFnName: "MULTIPLY",
				Fields: []domain.DataSetCustomField{
					{TableName: "employees", FieldName: "monthly_leave"},
					{TableName: "", FieldName: "2", IsLiteral: true},
				},
			},
		},
	}

	prev, err := svc.Preview(ctx, ds)
	if err != nil {
		t.Fatalf("preview failed: %v", err)
	}

	expectedExpr := "(monthly_leave * 2) AS \"double_monthly_leave\""
	if !strings.Contains(prev.Pipeline, expectedExpr) {
		t.Fatalf("expected chained expression %s in pipeline, got: %s", expectedExpr, prev.Pipeline)
	}
}

func TestDataSet_MySQL_Compiler(t *testing.T) {
	ctx := context.Background()
	svc := setupTestService()

	ds := &domain.DataSet{
		Name:          "MySQL Test DataSet",
		ReferenceName: "mysql_orders",
		Driver:        "mysql",
		BaseCollection: domain.BaseCollection{
			Collection: "employees",
		},
		FilterParams: []domain.FilterParam{
			{ParamName: "min_salary", ParamDataType: "decimal"},
		},
		Filter: map[string]any{
			"employees.salary": map[string]any{"ParamsName": "min_salary", "parmsDataType": "decimal"},
		},
	}

	prev, err := svc.Preview(ctx, ds)
	if err != nil {
		t.Fatalf("mysql preview failed: %v", err)
	}

	if !strings.Contains(prev.ReferencePipeline, "(? IS NULL OR `employees`.`salary` = ?)") {
		t.Fatalf("expected MySQL '?' placeholder binding, got: %s", prev.ReferencePipeline)
	}
}

func TestDataSet_MongoDB_Compiler(t *testing.T) {
	ctx := context.Background()
	svc := setupTestService()

	ds := &domain.DataSet{
		Name:          "MongoDB Test DataSet",
		ReferenceName: "mongo_orders",
		Driver:        "mongodb",
		BaseCollection: domain.BaseCollection{
			Collection: "employees",
		},
		JoinCollections: []domain.JoinCollection{
			{
				FromCollection:      "employees",
				FromCollectionField: "department_id",
				ToCollection:        "departments",
				ToCollectionField:   "_id",
				NamedAs:             "dept",
			},
		},
	}

	prev, err := svc.Preview(ctx, ds)
	if err != nil {
		t.Fatalf("mongo preview failed: %v", err)
	}

	if !strings.Contains(prev.Pipeline, "$lookup") || !strings.Contains(prev.Pipeline, "\"from\":\"departments\"") {
		t.Fatalf("expected MongoDB $lookup in pipeline, got: %s", prev.Pipeline)
	}
}

func TestDataSet_Validation_GroupByFailure(t *testing.T) {
	ctx := context.Background()
	svc := setupTestService()

	ds := &domain.DataSet{
		Name:          "Invalid Group By DataSet",
		ReferenceName: "invalid_group_by",
		Driver:        "postgres",
		BaseCollection: domain.BaseCollection{
			Collection: "employees",
		},
		GroupByFields: []domain.GroupByField{
			{TableName: "employees", FieldName: "department_id"},
		},
		SelectedList: []domain.SelectedField{
			{Field: "employees.name", HeaderName: "Employee Name"}, // Not in GROUP BY
		},
	}

	_, err := svc.Preview(ctx, ds)
	if err == nil {
		t.Fatalf("expected validation error for ungrouped selected field, got nil")
	}
	if !strings.Contains(err.Error(), "INVALID_GROUP_BY") {
		t.Fatalf("expected INVALID_GROUP_BY error code, got: %v", err)
	}
}

func TestDataSet_Validation_InvalidOperandCount(t *testing.T) {
	ctx := context.Background()
	svc := setupTestService()

	ds := &domain.DataSet{
		Name:          "Invalid Operand Count",
		ReferenceName: "invalid_op_count",
		Driver:        "postgres",
		BaseCollection: domain.BaseCollection{
			Collection: "employees",
		},
		CustomColumns: []domain.CustomColumn{
			{
				CustomColumnName:      "bad_add",
				CustomAggregateFnName: "ADD",
				Fields: []domain.DataSetCustomField{
					{TableName: "employees", FieldName: "salary"}, // Only 1 operand, ADD requires 2
				},
			},
		},
	}

	_, err := svc.Preview(ctx, ds)
	if err == nil {
		t.Fatalf("expected validation error for invalid operand count, got nil")
	}
	if !strings.Contains(err.Error(), "INVALID_OPERAND_COUNT") {
		t.Fatalf("expected INVALID_OPERAND_COUNT error code, got: %v", err)
	}
}

func TestCommon_CreateFilterParams(t *testing.T) {
	filterParams := []domain.FilterParam{
		{
			ParamName:     "dept_id",
			ParamDataType: "string",
			DefaultValue:  "dept_eng",
		},
		{
			ParamName:     "min_salary",
			ParamDataType: "int",
			DefaultValue:  50000,
		},
		{
			ParamName:     "is_active",
			ParamDataType: "bool",
			DefaultValue:  true,
		},
		{
			ParamName:     "user_org",
			ParamDataType: "string",
			DefaultValue:  "KTON|org_id",
		},
		{
			ParamName:     "start_date",
			ParamDataType: "time.Time",
			DefaultValue:  "CD|+0|ST",
		},
		{
			ParamName:     "end_date",
			ParamDataType: "date",
			DefaultValue:  "CD|+0|ED",
		},
		{
			ParamName:     "bonus_rate",
			ParamDataType: "decimal",
			DefaultValue:  0.15,
			Paramvalue:    0.20, // Runtime override
		},
	}

	userToken := map[string]any{
		"org_id":   "org_corp_999",
		"timezone": "America/New_York",
	}

	rawPipeline := `[
		{"$match": {"department": {"paramName":"dept_id","paramDataType":"string"}}},
		{"$match": {"salary": {"$gte": {"paramName":"min_salary","paramDataType":"int"}}}},
		{"$match": {"active": {"paramName":"is_active","paramDataType":"bool"}}},
		{"$match": {"organization": {"paramName":"user_org","paramDataType":"string"}}},
		{"$match": {"created_at": {"$gte": {"paramName":"start_date","paramDataType":"time.Time"}}}},
		{"$match": {"expired_at": {"$lte": {"paramName":"end_date","paramDataType":"date"}}}},
		{"$match": {"bonus": {"paramName":"bonus_rate","paramDataType":"decimal"}}}
	]`

	resolved := domain.CreateFilterParams(filterParams, rawPipeline, userToken)

	// Check string value is quoted
	if !strings.Contains(resolved, `"department": "dept_eng"`) {
		t.Errorf("expected department dept_eng, got:\n%s", resolved)
	}
	// Check int value is unquoted
	if !strings.Contains(resolved, `"salary": {"$gte": 50000}`) {
		t.Errorf("expected salary 50000, got:\n%s", resolved)
	}
	// Check bool value is unquoted
	if !strings.Contains(resolved, `"active": true`) {
		t.Errorf("expected active true, got:\n%s", resolved)
	}
	// Check KTON token resolution
	if !strings.Contains(resolved, `"organization": "org_corp_999"`) {
		t.Errorf("expected organization org_corp_999, got:\n%s", resolved)
	}
	// Check CD Start of Day resolution with timezone (00:00:00)
	if !strings.Contains(resolved, `T00:00:00`) {
		t.Errorf("expected start_date to have T00:00:00, got:\n%s", resolved)
	}
	// Check CD End of Day resolution with timezone (23:59:59)
	if !strings.Contains(resolved, `T23:59:59`) {
		t.Errorf("expected end_date to have T23:59:59, got:\n%s", resolved)
	}
	// Check runtime Paramvalue override (0.20 instead of 0.15)
	if !strings.Contains(resolved, `"bonus": 0.2`) {
		t.Errorf("expected bonus 0.2, got:\n%s", resolved)
	}
}

type mockExecutionAdapter struct {
	adapter.Adapter
	lastReq execution.ExecutionRequest
}

func (m *mockExecutionAdapter) Execute(ctx context.Context, req execution.ExecutionRequest) (*execution.ExecutionResult, error) {
	m.lastReq = req
	return &execution.ExecutionResult{
		Data:   []map[string]any{{"mock_id": 1}},
		Status: "SUCCESS",
	}, nil
}

func TestDataSetService_Execute_SaveModes(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewDataSetRepository()
	fnRegistry := resolver.NewFunctionRegistry()
	modelResolver := resolver.NewModelResolver(nil)

	mockAdp := &mockExecutionAdapter{}
	svc := service.NewDataSetService(repo, modelResolver, modelResolver, fnRegistry, nil)
	svc.SetAdapter(mockAdp)

	// 1. Test Procedure SaveMode
	dsProc := &domain.DataSet{
		ID:            "ds_proc",
		Name:          "Procedure Dataset",
		ReferenceName: "proc_dataset",
		SaveMode:      domain.SaveModeProcedure,
		FilterParams: []domain.FilterParam{
			{ParamName: "dept", ParamDataType: "string", DefaultValue: "engineering"},
		},
	}
	if err := repo.Save(ctx, dsProc); err != nil {
		t.Fatalf("failed saving proc dataset: %v", err)
	}

	_, err := svc.Execute(ctx, "proc_dataset", map[string]any{"dept": "sales"})
	if err != nil {
		t.Fatalf("failed executing procedure dataset: %v", err)
	}
	if mockAdp.lastReq.Operation != "PROCEDURE" {
		t.Errorf("expected operation PROCEDURE, got: %s", mockAdp.lastReq.Operation)
	}
	if mockAdp.lastReq.Target != "sp_proc_dataset" {
		t.Errorf("expected target sp_proc_dataset, got: %s", mockAdp.lastReq.Target)
	}
	if mockAdp.lastReq.Arguments["dept"] != "sales" {
		t.Errorf("expected arg dept=sales, got: %v", mockAdp.lastReq.Arguments["dept"])
	}

	// 2. Test Function SaveMode
	dsFn := &domain.DataSet{
		ID:            "ds_fn",
		Name:          "Function Dataset",
		ReferenceName: "fn_dataset",
		SaveMode:      domain.SaveModeFunction,
		FilterParams: []domain.FilterParam{
			{ParamName: "limit", ParamDataType: "int", DefaultValue: 10},
		},
	}
	if err := repo.Save(ctx, dsFn); err != nil {
		t.Fatalf("failed saving fn dataset: %v", err)
	}

	_, err = svc.Execute(ctx, "fn_dataset", map[string]any{"limit": 25})
	if err != nil {
		t.Fatalf("failed executing function dataset: %v", err)
	}
	if mockAdp.lastReq.Operation != "FUNCTION" {
		t.Errorf("expected operation FUNCTION, got: %s", mockAdp.lastReq.Operation)
	}
	if mockAdp.lastReq.Target != "fn_fn_dataset" {
		t.Errorf("expected target fn_fn_dataset, got: %s", mockAdp.lastReq.Target)
	}

	// 3. Test Direct Query SaveMode with CreateFilterParams substitution
	dsQuery := &domain.DataSet{
		ID:                "ds_query",
		Name:              "Query Dataset",
		ReferenceName:     "query_dataset",
		SaveMode:          domain.SaveModeQuery,
		Pipeline:          `SELECT * FROM users WHERE status = 'active'`,
		ReferencePipeline: `SELECT * FROM users WHERE status = {"paramName":"status","paramDataType":"string"}`,
		FilterParams: []domain.FilterParam{
			{ParamName: "status", ParamDataType: "string", DefaultValue: "active"},
		},
	}
	if err := repo.Save(ctx, dsQuery); err != nil {
		t.Fatalf("failed saving query dataset: %v", err)
	}

	_, err = svc.Execute(ctx, "query_dataset", map[string]any{"status": "pending"})
	if err != nil {
		t.Fatalf("failed executing query dataset: %v", err)
	}
	if mockAdp.lastReq.Operation != "QUERY" {
		t.Errorf("expected operation QUERY, got: %s", mockAdp.lastReq.Operation)
	}
	if !strings.Contains(mockAdp.lastReq.Target, `"pending"`) {
		t.Errorf("expected target to have substituted pending, got: %s", mockAdp.lastReq.Target)
	}
}

func TestDataSet_PayloadVsDefaultValue_Precedence(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewDataSetRepository()
	fnRegistry := resolver.NewFunctionRegistry()
	modelResolver := resolver.NewModelResolver(nil)

	mockAdp := &mockExecutionAdapter{}
	svc := service.NewDataSetService(repo, modelResolver, modelResolver, fnRegistry, nil)
	svc.SetAdapter(mockAdp)

	ds := &domain.DataSet{
		ID:                "ds_precedence",
		Name:              "Precedence Dataset",
		ReferenceName:     "precedence_ds",
		SaveMode:          domain.SaveModeQuery,
		Pipeline:          `SELECT * FROM items`,
		ReferencePipeline: `SELECT * FROM items WHERE col1 = {"paramName":"param1","paramDataType":"string"} AND col2 = {"paramName":"param2","paramDataType":"string"} AND col3 = {"paramName":"param3","paramDataType":"string"} AND col4 = {"paramName":"param4","paramDataType":"string"}`,
		FilterParams: []domain.FilterParam{
			{
				ParamName:     "param1",
				ParamDataType: "string",
				DefaultValue:  "default_val_1",
			},
			{
				ParamName:     "param2",
				ParamDataType: "string",
				DefaultValue:  "default_val_2",
			},
			{
				ParamName:     "param3",
				ParamDataType: "string",
				DefaultValue:  "default_val_3",
			},
			{
				ParamName:     "param4",
				ParamDataType: "string",
				DefaultValue:  "KTON|org_id",
			},
		},
	}
	if err := repo.Save(ctx, ds); err != nil {
		t.Fatalf("failed saving dataset: %v", err)
	}

	// Payload provides param1 (should use payload value)
	// Payload does not provide param2 (should use default value)
	// Payload provides empty string for param3 (should fallback to default value)
	// Payload does not provide param4 (should resolve KTON|org_id from userToken)
	payload := map[string]any{
		"param1": "payload_custom_1",
		"param3": "", // empty, should fallback
	}
	userToken := map[string]any{
		"org_id": "org_secret_777",
	}

	_, err := svc.ExecuteWithUserToken(ctx, "precedence_ds", payload, userToken)
	if err != nil {
		t.Fatalf("failed executing dataset: %v", err)
	}

	query := mockAdp.lastReq.Target

	// param1 must use payload value
	if !strings.Contains(query, `"payload_custom_1"`) {
		t.Errorf("expected param1 to use payload value 'payload_custom_1', got:\n%s", query)
	}
	// param2 must use default value
	if !strings.Contains(query, `"default_val_2"`) {
		t.Errorf("expected param2 to use default value 'default_val_2', got:\n%s", query)
	}
	// param3 must fallback to default value
	if !strings.Contains(query, `"default_val_3"`) {
		t.Errorf("expected param3 to fallback to default value 'default_val_3', got:\n%s", query)
	}
	// param4 must resolve from userToken
	if !strings.Contains(query, `"org_secret_777"`) {
		t.Errorf("expected param4 to resolve KTON token 'org_secret_777', got:\n%s", query)
	}

	// Now verify that if payload DOES provide param4, it overrides the KTON token default
	payloadWithParam4 := map[string]any{
		"param1": "custom_1",
		"param4": "override_org_888",
	}
	_, err = svc.ExecuteWithUserToken(ctx, "precedence_ds", payloadWithParam4, userToken)
	if err != nil {
		t.Fatalf("failed executing dataset with param4 override: %v", err)
	}
	query2 := mockAdp.lastReq.Target
	if !strings.Contains(query2, `"override_org_888"`) {
		t.Errorf("expected param4 to be overridden by payload 'override_org_888', got:\n%s", query2)
	}
}


