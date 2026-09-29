// Package crud provides unified dynamic CRUD operations across relational and document database adapters.
//
// File: crud.go
// Usage:
//
//	Implements the core CRUD Engine providing validated Create, Find, FindOne, Update, Patch,
//	and Delete actions with automatic input sanitization and orbital reference verification.
package crud

import (
	"context"
	"errors"
	"fmt"
	"github.com/SanjayDrop5528/models-go-engine/adapter"
	"github.com/SanjayDrop5528/models-go-engine/mapping"
	"github.com/SanjayDrop5528/models-go-engine/model"
	"github.com/SanjayDrop5528/models-go-engine/query"
	"github.com/SanjayDrop5528/models-go-engine/validation"
	"log"
	"strings"
	"time"
)

// OrbitalValidator provides orbital reference validation capabilities across models.
type OrbitalValidator interface {
	ValidateOrbitalReferences(ctx context.Context, modelID string, data map[string]any) error
}

type RelationModelResolver interface {
	GetActive(ctx context.Context, id string) (*model.Model, error)
}

// Engine orchestrates dynamic data operations through adapters with strict model validation.
type Engine struct {
	adapters         *adapter.Registry
	orbitalValidator OrbitalValidator
	modelResolver    RelationModelResolver
}

// NewEngine creates a new CRUD engine.
//
// Purpose:
//
//	Instantiates the CRUD engine with access to the adapter registry.
//
// Where it is used:
//   - Initialized during server bootstrap and used by HTTP CRUD controllers and test suites.
//
// When can it be used:
//   - Call when initializing runtime data manipulation services.
func NewEngine(adapters *adapter.Registry) *Engine {
	return &Engine{
		adapters: adapters,
	}
}

// SetOrbitalValidator sets the orbital reference validator instance.
//
// Purpose:
//
//	Configures cross-model reference validation to verify foreign keys and orbital links before writing data.
//
// Where it is used:
//   - Wired during dependency injection in server startup or complex project configurations.
//
// When can it be used:
//   - Call when enabling cross-model relationship integrity checks during CRUD execution.
func (e *Engine) SetOrbitalValidator(v OrbitalValidator) {
	e.orbitalValidator = v
}

func (e *Engine) SetModelResolver(resolver RelationModelResolver) {
	e.modelResolver = resolver
}

// Create validates, coerces, and inserts a new dynamic record.
//
// Purpose:
//
//	Performs model validation, orbital checks, input sanitization/ID generation, and executes adapter insertion.
//
// Where it is used:
//   - Called by POST /api/data/:model endpoints and data seeding routines.
//
// When can it be used:
//   - Call whenever creating a new record in a registered model table or collection.
func (e *Engine) Create(ctx context.Context, m *model.Model, data map[string]any) (map[string]any, error) {
	if m == nil {
		return nil, errors.New("model cannot be nil")
	}

	// 1. Validate raw payload FIRST before coercion
	if err := validation.ValidateData(m, data); err != nil {
		return nil, err
	}

	if e.orbitalValidator != nil {
		if err := e.orbitalValidator.ValidateOrbitalReferences(ctx, m.Name, data); err != nil {
			return nil, err
		}
	}

	// 2. Sanitize and coerce input for adapter
	sanitized, err := mapping.SanitizeInput(m, data)
	if err != nil {
		return nil, fmt.Errorf("data sanitization failed: %w", err)
	}

	adp, err := e.adapters.Get(m.Database)
	if err != nil {
		return nil, err
	}

	return adp.Create(ctx, m.Ref(), sanitized)
}

// Find executes a query against the adapter.
//
// Purpose:
//
//	Routes structured queries (with filters, projections, sorting, pagination) to the appropriate database adapter.
//
// Where it is used:
//   - Called by GET /api/data/:model search endpoints and reporting handlers.
//
// When can it be used:
//   - Call when retrieving a list of matching records along with total count.
func (e *Engine) Find(ctx context.Context, m *model.Model, q query.Query) ([]map[string]any, int64, error) {
	if m == nil {
		return nil, 0, errors.New("model cannot be nil")
	}

	adp, err := e.adapters.Get(m.Database)
	if err != nil {
		return nil, 0, err
	}

	q = q.EnsureDebugTrace()
	q = withAutomaticRelations(m, q)
	q = e.expandAutomaticChildren(ctx, m, q)
	if q.Debug {
		log.Printf("[Query Debug][%s][Engine] phase=dispatch operation=Find model=%s table=%s relations=%v relation_specs=%v filters=%v fields=%v sorts=%v pagination=%+v", q.DebugTraceID, m.ID, m.Ref().StorageName, q.Relations, q.DebugRelationSpecs(q.RelationSpecs), q.DebugFilters(q.Filters), q.Fields, q.Sorts, q.Pagination)
	}
	adapterQuery, hiddenFields := withHydrationFields(q, m)
	adapterQuery = withoutEngineHydratedRelations(adapterQuery, m, adp)
	rows, total, err := adp.Find(ctx, m.Ref(), adapterQuery)
	if err != nil {
		return nil, 0, err
	}
	if err := hydrateDeclaredRelations(ctx, adp, e.modelResolver, m, rows, q); err != nil {
		return nil, 0, err
	}
	removeHydrationFields(rows, hiddenFields)
	return rows, total, nil
}

// FindOne gets a record by primary key identifier.
//
// Purpose:
//
//	Retrieves a single record matching the given primary key from the target database adapter.
//
// Where it is used:
//   - Called by GET /api/data/:model/:id endpoints.
//
// When can it be used:
//   - Call when fetching an individual entity by ID.
func (e *Engine) FindOne(ctx context.Context, m *model.Model, id any) (map[string]any, error) {
	return e.FindOneWithQuery(ctx, m, id, query.NewQuery())
}

// findOneQueryAdapter is optional so existing third-party adapters remain
// source-compatible while adapters with relation support can load relations on
// a primary-key lookup.
type findOneQueryAdapter interface {
	FindOneWithQuery(context.Context, model.ModelRef, any, query.Query) (map[string]any, error)
}

// FindOneWithQuery retrieves one record and applies projections or requested
// orbital relations. Adapters without this optional capability fall back to a
// normal Find query using the model's primary key.
func (e *Engine) FindOneWithQuery(ctx context.Context, m *model.Model, id any, q query.Query) (map[string]any, error) {
	if m == nil {
		return nil, errors.New("model cannot be nil")
	}

	adp, err := e.adapters.Get(m.Database)
	if err != nil {
		return nil, err
	}

	ref := m.Ref()
	q = q.EnsureDebugTrace()
	q = withAutomaticRelations(m, q)
	q = e.expandAutomaticChildren(ctx, m, q)
	if q.Debug {
		log.Printf("[Query Debug][%s][Engine] phase=dispatch operation=FindOne model=%s table=%s primary_key=%s id=%v relations=%v relation_specs=%v fields=%v", q.DebugTraceID, m.ID, ref.StorageName, ref.PrimaryKey, q.DebugValue(id, 1), q.Relations, q.DebugRelationSpecs(q.RelationSpecs), q.Fields)
	}
	if !q.Debug && len(q.Relations) == 0 && len(q.RelationSpecs) == 0 && len(q.Fields) == 0 && len(q.ExcludedColumns) == 0 &&
		len(q.Filters) == 0 && len(q.RawWheres) == 0 && len(q.WhereGroups) == 0 && len(q.Joins) == 0 {
		return adp.FindOne(ctx, ref, id)
	}
	if queryAdapter, ok := adp.(findOneQueryAdapter); ok {
		adapterQuery, hiddenFields := withHydrationFields(q, m)
		row, err := queryAdapter.FindOneWithQuery(ctx, ref, id, withoutEngineHydratedRelations(adapterQuery, m, adp))
		if err != nil {
			return nil, err
		}
		if err := hydrateDeclaredRelations(ctx, adp, e.modelResolver, m, []map[string]any{row}, q); err != nil {
			return nil, err
		}
		removeHydrationFields([]map[string]any{row}, hiddenFields)
		return row, nil
	}

	primaryKey := ref.PrimaryKey
	if primaryKey == "" {
		primaryKey = "id"
	}
	adapterQuery, hiddenFields := withHydrationFields(q, m)
	adapterQuery = withoutEngineHydratedRelations(adapterQuery, m, adp)
	adapterQuery = adapterQuery.Where(primaryKey, query.OpEq, id).LimitOffset(1, 0)
	rows, _, err := adp.Find(ctx, ref, adapterQuery)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("record '%v' not found", id)
	}
	if err := hydrateDeclaredRelations(ctx, adp, e.modelResolver, m, rows, q); err != nil {
		return nil, err
	}
	removeHydrationFields(rows, hiddenFields)
	return rows[0], nil
}

func (e *Engine) expandAutomaticChildren(ctx context.Context, parent *model.Model, q query.Query) query.Query {
	if e.modelResolver == nil || parent == nil {
		return q
	}
	for i := range q.RelationSpecs {
		q.RelationSpecs[i] = e.expandRelationSpec(ctx, parent, q.RelationSpecs[i], map[string]bool{}, 0)
	}
	return q
}

func (e *Engine) expandRelationSpec(ctx context.Context, parent *model.Model, spec query.RelationSpec, visited map[string]bool, depth int) query.RelationSpec {
	if depth >= 16 || !spec.LoadWithChildren || parent == nil {
		return spec
	}
	root := strings.Split(spec.Name, ".")[0]
	var definition *model.Relation
	for i := range parent.Relations {
		if strings.EqualFold(parent.Relations[i].Name, root) {
			definition = &parent.Relations[i]
			break
		}
	}
	if definition == nil {
		return spec
	}
	key := strings.ToLower(parent.ID + ":" + definition.Name + ":" + definition.TargetModel)
	if visited[key] {
		return spec
	}
	nextVisited := make(map[string]bool, len(visited)+1)
	for item := range visited {
		nextVisited[item] = true
	}
	nextVisited[key] = true
	target, err := e.modelResolver.GetActive(ctx, definition.TargetModel)
	if err != nil || target == nil {
		return spec
	}
	for _, child := range target.Relations {
		if !child.LoadWithChildren {
			continue
		}
		childSpec := query.RelationSpec{Name: child.Name, LoadWithChildren: true}
		found := false
		for i := range spec.SubRelations {
			if strings.EqualFold(spec.SubRelations[i].Name, child.Name) {
				spec.SubRelations[i].LoadWithChildren = true
				spec.SubRelations[i] = e.expandRelationSpec(ctx, target, spec.SubRelations[i], nextVisited, depth+1)
				found = true
				break
			}
		}
		if !found {
			spec.SubRelations = append(spec.SubRelations, e.expandRelationSpec(ctx, target, childSpec, nextVisited, depth+1))
		}
	}
	return spec
}

// withHydrationFields preserves relation keys when a caller projects only a
// subset of parent fields. The extra keys are removed after hydration so the
// public response still matches the requested projection.
func withHydrationFields(q query.Query, m *model.Model) (query.Query, []string) {
	if m == nil || len(q.Fields) == 0 {
		return q, nil
	}
	requested := requestedRelationRoots(q)
	present := make(map[string]bool, len(q.Fields))
	for _, field := range q.Fields {
		present[strings.ToLower(field)] = true
	}
	var hidden []string
	for _, relation := range m.Relations {
		if !requested[strings.ToLower(relation.Name)] {
			continue
		}
		key := relation.ForeignKey
		if relation.Inverse || relation.Type == model.RelOneToMany || (relation.Type == model.RelManyToMany && key == "") {
			key = relation.TargetKey
		}
		if key == "" {
			key = "id"
		}
		if !present[strings.ToLower(key)] {
			q.Fields = append(q.Fields, key)
			hidden = append(hidden, key)
			present[strings.ToLower(key)] = true
		}
	}
	return q, hidden
}

func requestedRelationRoots(q query.Query) map[string]bool {
	requested := make(map[string]bool, len(q.Relations)+len(q.RelationSpecs))
	for _, name := range q.Relations {
		root := strings.TrimSpace(strings.Split(name, ".")[0])
		if root != "" {
			requested[strings.ToLower(root)] = true
		}
	}
	for _, spec := range q.RelationSpecs {
		root := strings.TrimSpace(strings.Split(spec.Name, ".")[0])
		if root != "" {
			requested[strings.ToLower(root)] = true
		}
	}
	return requested
}

func removeHydrationFields(rows []map[string]any, fields []string) {
	for _, row := range rows {
		for _, field := range fields {
			delete(row, field)
		}
	}
}

func withoutEngineHydratedRelations(q query.Query, m *model.Model, adp adapter.Adapter) query.Query {
	remove := make(map[string]bool)
	forceEngineHydration, _ := m.Metadata["engine_hydrate_relations"].(bool)
	for _, relation := range m.Relations {
		// PostgreSQL keeps its optimized native object-join path. Declared
		// collection relations use the shared hydrator because generated inverse
		// aliases and M:N junction metadata live on model.Relation.
		if forceEngineHydration || !strings.EqualFold(adp.Name(), "postgres") || relation.Type == model.RelOneToMany || relation.Type == model.RelManyToMany {
			remove[strings.ToLower(relation.Name)] = true
		}
	}
	if len(remove) == 0 {
		return q
	}
	filteredNames := make([]string, 0, len(q.Relations))
	for _, name := range q.Relations {
		root := strings.ToLower(strings.Split(name, ".")[0])
		if !remove[root] {
			filteredNames = append(filteredNames, name)
		}
	}
	q.Relations = filteredNames
	filteredSpecs := make([]query.RelationSpec, 0, len(q.RelationSpecs))
	for _, spec := range q.RelationSpecs {
		root := strings.ToLower(strings.Split(spec.Name, ".")[0])
		if !remove[root] {
			filteredSpecs = append(filteredSpecs, spec)
		}
	}
	q.RelationSpecs = filteredSpecs
	return q
}

func hydrateDeclaredRelations(ctx context.Context, adp adapter.Adapter, resolver RelationModelResolver, m *model.Model, rows []map[string]any, q query.Query) error {
	if len(rows) == 0 || (len(q.Relations) == 0 && len(q.RelationSpecs) == 0) {
		return nil
	}

	requested := make(map[string]query.RelationSpec)
	for _, name := range q.Relations {
		parts := strings.Split(strings.TrimSpace(name), ".")
		root := parts[0]
		if root != "" {
			spec := query.RelationSpec{Name: root, LoadWithChildren: q.LoadWithChildren}
			if len(parts) > 1 {
				spec.SubRelations = []query.RelationSpec{nestedRelationSpec(parts[1:])}
			}
			key := strings.ToLower(root)
			requested[key] = mergeRelationSpec(requested[key], spec)
		}
	}
	for _, spec := range q.RelationSpecs {
		parts := strings.Split(strings.TrimSpace(spec.Name), ".")
		root := parts[0]
		if root != "" {
			spec.Name = root
			if len(parts) > 1 {
				spec.SubRelations = append(spec.SubRelations, nestedRelationSpec(parts[1:]))
			}
			key := strings.ToLower(root)
			requested[key] = mergeRelationSpec(requested[key], spec)
		}
	}

	definitions := make(map[string]model.Relation, len(m.Relations))
	for _, relation := range m.Relations {
		definitions[strings.ToLower(strings.TrimSpace(relation.Name))] = relation
	}

	for key, spec := range requested {
		relationStarted := time.Now()
		relation, ok := definitions[key]
		if !ok {
			// PostgreSQL resolves reverse and metadata-only relations natively.
			if strings.EqualFold(adp.Name(), "postgres") {
				continue
			}
			return fmt.Errorf("relation '%s' is not declared on model '%s' and adapter '%s' cannot infer it", spec.Name, m.ID, adp.Name())
		}
		targetKey := relation.TargetKey
		if targetKey == "" {
			targetKey = "id"
		}
		targetRef := model.NewModelRef(relation.TargetModel, relation.TargetModel, relation.TargetModel, targetKey)
		var targetModel *model.Model
		if resolver != nil {
			resolvedModel, err := resolver.GetActive(ctx, relation.TargetModel)
			if err != nil {
				return fmt.Errorf("resolving target model '%s' for relation '%s': %w", relation.TargetModel, relation.Name, err)
			}
			if resolvedModel == nil {
				return fmt.Errorf("resolving target model '%s' for relation '%s': resolver returned nil", relation.TargetModel, relation.Name)
			}
			targetModel = resolvedModel
			targetRef = targetModel.Ref()
		}
		if !strings.EqualFold(adp.Name(), "postgres") && (len(spec.Conditions) > 0 || len(spec.On) > 0) {
			return fmt.Errorf("relation '%s' uses SQL conditions that adapter '%s' cannot safely apply", relation.Name, adp.Name())
		}
		if q.Debug {
			log.Printf("[Query Debug][%s][Engine] phase=relation-hydration-start relation=%s cardinality=%s parent_model=%s target_model=%s foreign_key=%s target_key=%s junction_model=%s fields=%v order=%v nested=%d adapter=%s",
				q.DebugTraceID, relation.Name, relation.Type, m.ID, relation.TargetModel, relation.ForeignKey, targetKey, relation.JunctionModel, spec.Fields, spec.Order, len(spec.SubRelations), adp.Name())
		}

		nestedQuery := relationChildrenQuery(targetModel, spec, q)
		hydratedRows := 0
		hydratedChildren := 0

		for _, row := range rows {
			if _, alreadyLoaded := row[relation.Name]; alreadyLoaded {
				continue
			}
			childQuery := query.NewQuery()
			childQuery.Pagination.Limit = 0
			childQuery.Debug = q.Debug
			childQuery.DebugTraceID = q.DebugTraceID
			childQuery.Fields = append(childQuery.Fields, spec.Fields...)
			childQuery.Sorts = append(childQuery.Sorts, spec.Order...)
			var hiddenChildFields []string
			if targetModel != nil {
				childQuery, hiddenChildFields = withHydrationFieldsFor(childQuery, targetModel, nestedQuery)
			}

			switch relation.Type {
			case model.RelOneToOne, model.RelManyToOne:
				parentField := relation.ForeignKey
				childField := targetKey
				if relation.Inverse {
					parentField = targetKey
					childField = relation.ForeignKey
				}
				value := row[parentField]
				if value == nil {
					row[relation.Name] = nil
					hydratedRows++
					continue
				}
				childQuery = childQuery.Where(childField, query.OpEq, value).LimitOffset(1, 0)
				children, _, err := adp.Find(ctx, targetRef, childQuery)
				if err != nil {
					return fmt.Errorf("loading object relation '%s': %w", relation.Name, err)
				}
				if len(children) == 0 {
					row[relation.Name] = nil
				} else {
					if err := hydrateNestedRelationSpecs(ctx, adp, resolver, targetModel, children, nestedQuery); err != nil {
						return err
					}
					removeHydrationFields(children, hiddenChildFields)
					row[relation.Name] = children[0]
					hydratedChildren++
				}
				hydratedRows++

			case model.RelOneToMany:
				value := row[targetKey]
				if value == nil {
					row[relation.Name] = []map[string]any{}
					continue
				}
				childQuery = childQuery.Where(relation.ForeignKey, query.OpEq, value)
				children, _, err := adp.Find(ctx, targetRef, childQuery)
				if err != nil {
					return fmt.Errorf("loading array relation '%s': %w", relation.Name, err)
				}
				if children == nil {
					children = []map[string]any{}
				}
				if err := hydrateNestedRelationSpecs(ctx, adp, resolver, targetModel, children, nestedQuery); err != nil {
					return err
				}
				removeHydrationFields(children, hiddenChildFields)
				row[relation.Name] = children
				hydratedRows++
				hydratedChildren += len(children)

			case model.RelManyToMany:
				if relation.JunctionModel == "" || relation.JunctionSourceKey == "" || relation.JunctionTargetKey == "" {
					return fmt.Errorf("many-to-many relation '%s' requires junction_model, junction_source_key, and junction_target_key", relation.Name)
				}
				parentValue := row[relation.ForeignKey]
				if parentValue == nil {
					parentValue = row[targetKey]
				}
				junctionRef := model.NewModelRef(relation.JunctionModel, relation.JunctionModel, relation.JunctionModel, "id")
				if resolver != nil {
					junctionModel, err := resolver.GetActive(ctx, relation.JunctionModel)
					if err != nil {
						return fmt.Errorf("resolving junction model '%s' for relation '%s': %w", relation.JunctionModel, relation.Name, err)
					}
					if junctionModel == nil {
						return fmt.Errorf("resolving junction model '%s' for relation '%s': resolver returned nil", relation.JunctionModel, relation.Name)
					}
					junctionRef = junctionModel.Ref()
				}
				junctionQuery := query.New().Where(relation.JunctionSourceKey, query.OpEq, parentValue)
				junctionQuery.Pagination.Limit = 0
				links, _, err := adp.Find(ctx, junctionRef, junctionQuery)
				if err != nil {
					return fmt.Errorf("loading junction for relation '%s': %w", relation.Name, err)
				}
				ids := make([]any, 0, len(links))
				for _, link := range links {
					if value := link[relation.JunctionTargetKey]; value != nil {
						ids = append(ids, value)
					}
				}
				if len(ids) == 0 {
					row[relation.Name] = []map[string]any{}
					continue
				}
				children, _, err := adp.Find(ctx, targetRef, childQuery.Where(targetKey, query.OpIn, ids))
				if err != nil {
					return fmt.Errorf("loading many-to-many relation '%s': %w", relation.Name, err)
				}
				if err := hydrateNestedRelationSpecs(ctx, adp, resolver, targetModel, children, nestedQuery); err != nil {
					return err
				}
				removeHydrationFields(children, hiddenChildFields)
				row[relation.Name] = children
				hydratedRows++
				hydratedChildren += len(children)
			default:
				return fmt.Errorf("relation '%s' has unsupported cardinality '%s'", relation.Name, relation.Type)
			}
		}
		if q.Debug {
			log.Printf("[Query Debug][%s][Engine] phase=relation-hydration-complete relation=%s parent_rows=%d hydrated_rows=%d child_rows=%d duration=%s",
				q.DebugTraceID, relation.Name, len(rows), hydratedRows, hydratedChildren, time.Since(relationStarted))
		}
	}
	return nil
}

func mergeRelationSpec(existing, incoming query.RelationSpec) query.RelationSpec {
	if existing.Name == "" {
		return incoming
	}
	existing.LoadWithChildren = existing.LoadWithChildren || incoming.LoadWithChildren
	if len(incoming.Fields) > 0 {
		existing.Fields = append(existing.Fields, incoming.Fields...)
	}
	existing.Conditions = append(existing.Conditions, incoming.Conditions...)
	existing.On = append(existing.On, incoming.On...)
	existing.Order = append(existing.Order, incoming.Order...)
	for _, child := range incoming.SubRelations {
		matched := false
		for i := range existing.SubRelations {
			if strings.EqualFold(existing.SubRelations[i].Name, child.Name) {
				existing.SubRelations[i] = mergeRelationSpec(existing.SubRelations[i], child)
				matched = true
				break
			}
		}
		if !matched {
			existing.SubRelations = append(existing.SubRelations, child)
		}
	}
	return existing
}

func nestedRelationSpec(parts []string) query.RelationSpec {
	spec := query.RelationSpec{Name: parts[0], LoadWithChildren: true}
	if len(parts) > 1 {
		spec.SubRelations = []query.RelationSpec{nestedRelationSpec(parts[1:])}
	}
	return spec
}

func relationChildrenQuery(targetModel *model.Model, parent query.RelationSpec, root query.Query) query.Query {
	nestedQuery := query.NewQuery()
	nestedQuery.Debug = root.Debug
	nestedQuery.DebugTraceID = root.DebugTraceID
	nestedQuery.DebugIncludeArgs = root.DebugIncludeArgs
	nestedQuery.SlowQueryThresholdMS = root.SlowQueryThresholdMS
	nestedQuery.RelationSpecs = append(nestedQuery.RelationSpecs, parent.SubRelations...)
	if targetModel != nil && parent.LoadWithChildren {
		nestedQuery = withAutomaticRelations(targetModel, nestedQuery)
	}
	return nestedQuery
}

func withHydrationFieldsFor(child query.Query, targetModel *model.Model, relations query.Query) (query.Query, []string) {
	relations.Fields = child.Fields
	enriched, hidden := withHydrationFields(relations, targetModel)
	child.Fields = enriched.Fields
	return child, hidden
}

func hydrateNestedRelationSpecs(ctx context.Context, adp adapter.Adapter, resolver RelationModelResolver, targetModel *model.Model, rows []map[string]any, nestedQuery query.Query) error {
	if targetModel == nil || len(rows) == 0 || (len(nestedQuery.Relations) == 0 && len(nestedQuery.RelationSpecs) == 0) {
		return nil
	}
	return hydrateDeclaredRelations(ctx, adp, resolver, targetModel, rows, nestedQuery)
}

// withAutomaticRelations adds only model relations explicitly configured for
// automatic loading. Request-provided relations remain authoritative and work
// even when the model relation has LoadWithChildren disabled.
func withAutomaticRelations(m *model.Model, q query.Query) query.Query {
	q = q.EnsureDebugTrace()
	requested := make(map[string]bool, len(q.Relations)+len(q.RelationSpecs))
	for _, name := range q.Relations {
		requested[strings.ToLower(strings.TrimSpace(name))] = true
	}
	for _, spec := range q.RelationSpecs {
		requested[strings.ToLower(strings.TrimSpace(spec.Name))] = true
	}

	for _, relation := range m.Relations {
		name := strings.TrimSpace(relation.Name)
		if name == "" {
			continue
		}
		if requested[strings.ToLower(name)] {
			if q.Debug {
				log.Printf("[Query Debug][%s][Engine] phase=relation-decision relation=%s decision=explicit-request cardinality=%s", q.DebugTraceID, name, relation.Type)
			}
			continue
		}
		if !relation.LoadWithChildren {
			if q.Debug {
				log.Printf("[Query Debug][%s][Engine] phase=relation-decision relation=%s decision=lazy load_with_children=false cardinality=%s", q.DebugTraceID, name, relation.Type)
			}
			continue
		}
		q = q.RelationWithOpts(name, query.RelationOpts{LoadWithChildren: true})
		if q.Debug {
			log.Printf("[Query Debug][%s][Engine] phase=relation-decision relation=%s decision=auto-load model=%s load_with_children=true cardinality=%s source_key=%s target_model=%s target_key=%s", q.DebugTraceID, name, m.ID, relation.Type, relation.ForeignKey, relation.TargetModel, relation.TargetKey)
		}
		requested[strings.ToLower(name)] = true
	}
	return q
}

// Update updates fields in an existing record by payload keys.
//
// Purpose:
//
//	Verifies record existence, validates partial payload, coerces field values, and executes adapter update.
//
// Where it is used:
//   - Called by PUT /api/data/:model/:id endpoints.
//
// When can it be used:
//   - Call when replacing or updating fields on an existing record.
func (e *Engine) Update(ctx context.Context, m *model.Model, id any, data map[string]any) (map[string]any, error) {
	if m == nil {
		return nil, errors.New("model cannot be nil")
	}

	adp, err := e.adapters.Get(m.Database)
	if err != nil {
		return nil, err
	}

	// 1. Verify that the target record exists in the database
	_, err = adp.FindOne(ctx, m.Ref(), id)
	if err != nil {
		return nil, fmt.Errorf("record '%v' not found in model '%s'", id, m.Name)
	}

	// 2. Validate raw payload FIRST before coercion
	if err := validation.ValidatePartialData(m, data); err != nil {
		return nil, err
	}

	if e.orbitalValidator != nil {
		if err := e.orbitalValidator.ValidateOrbitalReferences(ctx, m.Name, data); err != nil {
			return nil, err
		}
	}

	// 3. Sanitize and coerce input for adapter
	sanitized, err := mapping.SanitizePartialInput(m, data)
	if err != nil {
		return nil, fmt.Errorf("data sanitization failed: %w", err)
	}

	// 4. Update adapter with payload given
	return adp.Update(ctx, m.Ref(), id, sanitized)
}

// Patch partially updates fields in an existing record by payload keys.
//
// Purpose:
//
//	Aliases Update for partial modification workflows.
//
// Where it is used:
//   - Called by PATCH /api/data/:model/:id endpoints.
//
// When can it be used:
//   - Call when partially modifying specific columns or attributes of an existing record.
func (e *Engine) Patch(ctx context.Context, m *model.Model, id any, data map[string]any) (map[string]any, error) {
	return e.Update(ctx, m, id, data)
}

// Delete removes a record by primary key identifier.
//
// Purpose:
//
//	Dispatches record removal by primary key to the target database adapter.
//
// Where it is used:
//   - Called by DELETE /api/data/:model/:id endpoints.
//
// When can it be used:
//   - Call when deleting an existing record from storage.
func (e *Engine) Delete(ctx context.Context, m *model.Model, id any) error {
	if m == nil {
		return errors.New("model cannot be nil")
	}

	adp, err := e.adapters.Get(m.Database)
	if err != nil {
		return err
	}

	return adp.Delete(ctx, m.Ref(), id)
}
