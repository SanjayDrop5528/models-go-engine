package resolver

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/SanjayDrop5528/models-go-engine/dataset/domain"
)

// InMemFunctionRegistry provides a thread-safe, database-driven function registry implementation.
type InMemFunctionRegistry struct {
	mu        sync.RWMutex
	functions map[string]*domain.FunctionDefinition
}

// NewFunctionRegistry creates and seeds a function registry with all standard categories.
func NewFunctionRegistry() *InMemFunctionRegistry {
	r := &InMemFunctionRegistry{
		functions: make(map[string]*domain.FunctionDefinition),
	}
	r.seedDefaultFunctions()
	return r
}

// ResolveFunction finds a function by its name or reference name (case-insensitive).
func (r *InMemFunctionRegistry) ResolveFunction(ctx context.Context, name string) (*domain.FunctionDefinition, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	norm := strings.ToUpper(strings.TrimSpace(name))
	fn, exists := r.functions[norm]
	if !exists {
		return nil, domain.NewErrorf(domain.ErrFunctionNotFound, "function '%s' is not registered in dataset_functions", name)
	}
	if fn.Status != "ACTIVE" {
		return nil, domain.NewErrorf(domain.ErrFunctionInactive, "function '%s' is registered but currently %s", name, fn.Status)
	}
	return fn, nil
}

// RegisterFunction stores or updates a function in the registry.
func (r *InMemFunctionRegistry) RegisterFunction(ctx context.Context, fn *domain.FunctionDefinition) error {
	if fn == nil || fn.Name == "" {
		return domain.NewError(domain.ErrFunctionNotFound, "invalid function definition: name is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	if fn.Status == "" {
		fn.Status = "ACTIVE"
	}
	if fn.CreatedAt.IsZero() {
		fn.CreatedAt = time.Now()
	}
	fn.UpdatedAt = time.Now()

	r.functions[strings.ToUpper(fn.Name)] = fn
	if fn.ReferenceName != "" {
		r.functions[strings.ToUpper(fn.ReferenceName)] = fn
	}
	return nil
}

// ListFunctions returns all functions in a category, or all functions if category is empty.
func (r *InMemFunctionRegistry) ListFunctions(ctx context.Context, category domain.FunctionCategory) ([]*domain.FunctionDefinition, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var list []*domain.FunctionDefinition
	seen := make(map[string]bool)

	for _, fn := range r.functions {
		if !seen[fn.Name] {
			seen[fn.Name] = true
			if category == "" || fn.Category == category {
				list = append(list, fn)
			}
		}
	}
	return list, nil
}

func (r *InMemFunctionRegistry) seedDefaultFunctions() {
	now := time.Now()

	add := func(name string, cat domain.FunctionCategory, minOp, maxOp int, isAgg bool, pg, mysql, mongo, desc string) {
		fn := &domain.FunctionDefinition{
			ID:                 fmt.Sprintf("fn_%s", strings.ToLower(name)),
			Name:               name,
			ReferenceName:      strings.ToLower(name),
			Category:           cat,
			Status:             "ACTIVE",
			MinOperands:        minOp,
			MaxOperands:        maxOp,
			IsAggregate:        isAgg,
			IsDeterministic:    true,
			PostgresExpression: pg,
			MySQLExpression:    mysql,
			MongoDBExpression:  mongo,
			Description:        desc,
			CreatedAt:          now,
			UpdatedAt:          now,
		}
		r.functions[strings.ToUpper(name)] = fn
		r.functions[strings.ToLower(name)] = fn
	}

	// 1. Numeric Functions
	add("ADD", domain.CategoryNumeric, 2, 2, false, "({{0}} + {{1}})", "({{0}} + {{1}})", "{\"$add\": [\"{{0}}\", \"{{1}}\"]}", "Adds two numbers")
	add("SUBTRACT", domain.CategoryNumeric, 2, 2, false, "({{0}} - {{1}})", "({{0}} - {{1}})", "{\"$subtract\": [\"{{0}}\", \"{{1}}\"]}", "Subtracts second number from first")
	add("MULTIPLY", domain.CategoryNumeric, 2, 2, false, "({{0}} * {{1}})", "({{0}} * {{1}})", "{\"$multiply\": [\"{{0}}\", \"{{1}}\"]}", "Multiplies two numbers")
	add("DIVIDE", domain.CategoryNumeric, 2, 2, false, "({{0}} / NULLIF({{1}}, 0))", "({{0}} / NULLIF({{1}}, 0))", "{\"$divide\": [\"{{0}}\", \"{{1}}\"]}", "Divides first number by second")
	add("MODULO", domain.CategoryNumeric, 2, 2, false, "MOD({{0}}, {{1}})", "MOD({{0}}, {{1}})", "{\"$mod\": [\"{{0}}\", \"{{1}}\"]}", "Remainder of division")
	add("POWER", domain.CategoryNumeric, 2, 2, false, "POWER({{0}}, {{1}})", "POW({{0}}, {{1}})", "{\"$pow\": [\"{{0}}\", \"{{1}}\"]}", "Raises first number to power of second")
	add("SQRT", domain.CategoryNumeric, 1, 1, false, "SQRT({{0}})", "SQRT({{0}})", "{\"$sqrt\": \"{{0}}\"}", "Square root")
	add("ABS", domain.CategoryNumeric, 1, 1, false, "ABS({{0}})", "ABS({{0}})", "{\"$abs\": \"{{0}}\"}", "Absolute value")
	add("ROUND", domain.CategoryNumeric, 1, 2, false, "ROUND({{0}}, {{1}})", "ROUND({{0}}, {{1}})", "{\"$round\": [\"{{0}}\", \"{{1}}\"]}", "Rounds number")
	add("CEIL", domain.CategoryNumeric, 1, 1, false, "CEIL({{0}})", "CEIL({{0}})", "{\"$ceil\": \"{{0}}\"}", "Ceiling")
	add("FLOOR", domain.CategoryNumeric, 1, 1, false, "FLOOR({{0}})", "FLOOR({{0}})", "{\"$floor\": \"{{0}}\"}", "Floor")

	// 2. Aggregate Functions
	add("SUM", domain.CategoryAggregate, 1, 1, true, "SUM({{0}})", "SUM({{0}})", "{\"$sum\": \"{{0}}\"}", "Calculates sum")
	add("AVG", domain.CategoryAggregate, 1, 1, true, "AVG({{0}})", "AVG({{0}})", "{\"$avg\": \"{{0}}\"}", "Calculates average")
	add("COUNT", domain.CategoryAggregate, 1, 1, true, "COUNT({{0}})", "COUNT({{0}})", "{\"$sum\": 1}", "Counts records")
	add("COUNT_DISTINCT", domain.CategoryAggregate, 1, 1, true, "COUNT(DISTINCT {{0}})", "COUNT(DISTINCT {{0}})", "{\"$addToSet\": \"{{0}}\"}", "Counts distinct values")
	add("MIN", domain.CategoryAggregate, 1, 1, true, "MIN({{0}})", "MIN({{0}})", "{\"$min\": \"{{0}}\"}", "Calculates minimum")
	add("MAX", domain.CategoryAggregate, 1, 1, true, "MAX({{0}})", "MAX({{0}})", "{\"$max\": \"{{0}}\"}", "Calculates maximum")

	// 3. String Functions
	add("CONCAT", domain.CategoryString, 1, -1, false, "CONCAT({{args}})", "CONCAT({{args}})", "{\"$concat\": [{{args}}]}", "Concatenates strings")
	add("CONCAT_WS", domain.CategoryString, 2, -1, false, "CONCAT_WS({{args}})", "CONCAT_WS({{args}})", "{\"$concat\": [{{args}}]}", "Concatenates with separator")
	add("UPPER", domain.CategoryString, 1, 1, false, "UPPER({{0}})", "UPPER({{0}})", "{\"$toUpper\": \"{{0}}\"}", "Upper case")
	add("LOWER", domain.CategoryString, 1, 1, false, "LOWER({{0}})", "LOWER({{0}})", "{\"$toLower\": \"{{0}}\"}", "Lower case")
	add("TRIM", domain.CategoryString, 1, 1, false, "TRIM({{0}})", "TRIM({{0}})", "{\"$trim\": {\"input\": \"{{0}}\"}}", "Trims whitespace")
	add("LENGTH", domain.CategoryString, 1, 1, false, "LENGTH({{0}})", "LENGTH({{0}})", "{\"$strLenCP\": \"{{0}}\"}", "String length")
	add("SUBSTRING", domain.CategoryString, 2, 3, false, "SUBSTRING({{0}}, {{1}}, {{2}})", "SUBSTRING({{0}}, {{1}}, {{2}})", "{\"$substrCP\": [\"{{0}}\", \"{{1}}\", \"{{2}}\"]}", "Extracts substring")
	add("REPLACE", domain.CategoryString, 3, 3, false, "REPLACE({{0}}, {{1}}, {{2}})", "REPLACE({{0}}, {{1}}, {{2}})", "{\"$replaceAll\": {\"input\": \"{{0}}\", \"find\": \"{{1}}\", \"replacement\": \"{{2}}\"}}", "Replaces occurrences")

	// 4. Date/Time Functions
	add("NOW", domain.CategoryDateTime, 0, 0, false, "NOW()", "NOW()", "$$NOW", "Current timestamp")
	add("CURRENT_DATE", domain.CategoryDateTime, 0, 0, false, "CURRENT_DATE", "CURDATE()", "$$NOW", "Current date")
	add("DATE_ADD", domain.CategoryDateTime, 2, 3, false, "({{0}} + INTERVAL '{{1}} {{2}}')", "DATE_ADD({{0}}, INTERVAL {{1}} {{2}})", "{\"$dateAdd\": {\"startDate\": \"{{0}}\", \"unit\": \"{{2}}\", \"amount\": {{1}}}}", "Adds interval to date")
	add("DATE_DIFF", domain.CategoryDateTime, 2, 2, false, "({{0}}::date - {{1}}::date)", "DATEDIFF({{0}}, {{1}})", "{\"$dateDiff\": {\"startDate\": \"{{1}}\", \"endDate\": \"{{0}}\", \"unit\": \"day\"}}", "Difference in days")
	add("YEAR", domain.CategoryDateTime, 1, 1, false, "EXTRACT(YEAR FROM {{0}})", "YEAR({{0}})", "{\"$year\": \"{{0}}\"}", "Extracts year")
	add("MONTH", domain.CategoryDateTime, 1, 1, false, "EXTRACT(MONTH FROM {{0}})", "MONTH({{0}})", "{\"$month\": \"{{0}}\"}", "Extracts month")
	add("DAY", domain.CategoryDateTime, 1, 1, false, "EXTRACT(DAY FROM {{0}})", "DAY({{0}})", "{\"$dayOfMonth\": \"{{0}}\"}", "Extracts day")

	// 5. Comparison & Conditional
	add("EQUAL", domain.CategoryComparison, 2, 2, false, "({{0}} = {{1}})", "({{0}} = {{1}})", "{\"$eq\": [\"{{0}}\", \"{{1}}\"]}", "Equality test")
	add("NOT_EQUAL", domain.CategoryComparison, 2, 2, false, "({{0}} != {{1}})", "({{0}} != {{1}})", "{\"$ne\": [\"{{0}}\", \"{{1}}\"]}", "Inequality test")
	add("GREATER_THAN", domain.CategoryComparison, 2, 2, false, "({{0}} > {{1}})", "({{0}} > {{1}})", "{\"$gt\": [\"{{0}}\", \"{{1}}\"]}", "Greater than")
	add("LESS_THAN", domain.CategoryComparison, 2, 2, false, "({{0}} < {{1}})", "({{0}} < {{1}})", "{\"$lt\": [\"{{0}}\", \"{{1}}\"]}", "Less than")
	add("COALESCE", domain.CategoryBoolean, 2, -1, false, "COALESCE({{args}})", "COALESCE({{args}})", "{\"$ifNull\": [\"{{0}}\", \"{{1}}\"]}", "Returns first non-null argument")
	add("CASE", domain.CategoryBoolean, 2, -1, false, "CASE WHEN {{0}} THEN {{1}} ELSE {{2}} END", "CASE WHEN {{0}} THEN {{1}} ELSE {{2}} END", "{\"$cond\": {\"if\": \"{{0}}\", \"then\": \"{{1}}\", \"else\": \"{{2}}\"}}", "Conditional evaluation")
	add("IF", domain.CategoryBoolean, 3, 3, false, "CASE WHEN {{0}} THEN {{1}} ELSE {{2}} END", "IF({{0}}, {{1}}, {{2}})", "{\"$cond\": [\"{{0}}\", \"{{1}}\", \"{{2}}\"]}", "Inline conditional")

	// 6. Conditional Aggregates
	add("COUNT_IF", domain.CategoryConditionalAggregate, 1, 1, true, "COUNT(CASE WHEN {{0}} THEN 1 END)", "COUNT(CASE WHEN {{0}} THEN 1 END)", "{\"$sum\": {\"$cond\": [\"{{0}}\", 1, 0]}}", "Counts matching condition")
	add("SUM_IF", domain.CategoryConditionalAggregate, 2, 2, true, "SUM(CASE WHEN {{0}} THEN {{1}} ELSE 0 END)", "SUM(CASE WHEN {{0}} THEN {{1}} ELSE 0 END)", "{\"$sum\": {\"$cond\": [\"{{0}}\", \"{{1}}\", 0]}}", "Sums matching condition")

	// 7. Type Conversion
	add("TO_STRING", domain.CategoryTypeConversion, 1, 1, false, "CAST({{0}} AS TEXT)", "CAST({{0}} AS CHAR)", "{\"$toString\": \"{{0}}\"}", "Cast to string")
	add("TO_INTEGER", domain.CategoryTypeConversion, 1, 1, false, "CAST({{0}} AS INTEGER)", "CAST({{0}} AS SIGNED)", "{\"$toInt\": \"{{0}}\"}", "Cast to integer")
	add("TO_DECIMAL", domain.CategoryTypeConversion, 1, 1, false, "CAST({{0}} AS NUMERIC)", "CAST({{0}} AS DECIMAL(18,4))", "{\"$toDecimal\": \"{{0}}\"}", "Cast to decimal")

	// 8. Financial
	add("PERCENTAGE", domain.CategoryFinancial, 2, 2, false, "(({{0}} / NULLIF({{1}}, 0)) * 100.0)", "(({{0}} / NULLIF({{1}}, 0)) * 100.0)", "{\"$multiply\": [{\"$divide\": [\"{{0}}\", \"{{1}}\"]}, 100]}", "Calculates percentage")
	add("DISCOUNT", domain.CategoryFinancial, 2, 2, false, "({{0}} - ({{0}} * ({{1}} / 100.0)))", "({{0}} - ({{0}} * ({{1}} / 100.0)))", "{\"$subtract\": [\"{{0}}\", {\"$multiply\": [\"{{0}}\", {\"$divide\": [\"{{1}}\", 100]}]}]}", "Calculates discounted price")
}
