package query

import (
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/SanjayDrop5528/models-go-engine/rdbms/schema"
	"github.com/google/uuid"
)

var structModelCache sync.Map

// StructTableModel implements schema.TableModel for Go struct and slice-of-struct models.
type StructTableModel struct {
	table       *schema.Table
	joins       []*schema.RelationJoin
	joinMap     map[string]*schema.RelationJoin
	activeJoins []*schema.RelationJoin
	activeSet   map[string]bool
	modelType   reflect.Type
	isStruct    bool
}

var _ schema.TableModel = (*StructTableModel)(nil)

// Table returns the underlying schema.Table descriptor.
func (s *StructTableModel) Table() *schema.Table {
	return s.table
}

// IsStructModel returns true if this table model was constructed from a Go struct.
func (s *StructTableModel) IsStructModel() bool {
	return s.isStruct
}

// Clone creates an isolated copy of the StructTableModel for query execution.
func (s *StructTableModel) Clone() schema.TableModel {
	joinsCopy := make([]*schema.RelationJoin, len(s.joins))
	copy(joinsCopy, s.joins)

	activeCopy := make([]*schema.RelationJoin, len(s.activeJoins))
	copy(activeCopy, s.activeJoins)

	activeSetCopy := make(map[string]bool, len(s.activeSet))
	for k, v := range s.activeSet {
		activeSetCopy[k] = v
	}

	joinMapCopy := make(map[string]*schema.RelationJoin, len(s.joinMap))
	for k, v := range s.joinMap {
		joinMapCopy[k] = v
	}

	return &StructTableModel{
		table:       s.table,
		joins:       joinsCopy,
		joinMap:     joinMapCopy,
		activeJoins: activeCopy,
		activeSet:   activeSetCopy,
		modelType:   s.modelType,
		isStruct:    s.isStruct,
	}
}

// GetJoins returns only relation joins that were explicitly requested.
func (s *StructTableModel) GetJoins() []*schema.RelationJoin {
	return s.activeJoins
}

// HasActiveJoins reports whether any relations were explicitly activated on this table model.
func (s *StructTableModel) HasActiveJoins() bool {
	return len(s.activeJoins) > 0
}

func (s *StructTableModel) activateJoin(j *schema.RelationJoin) {
	if j == nil || j.Relation == nil {
		return
	}
	name := strings.ToLower(j.Relation.Name)
	if s.activeSet == nil {
		s.activeSet = make(map[string]bool)
	}
	if !s.activeSet[name] {
		s.activeSet[name] = true
		s.activeJoins = append(s.activeJoins, j)
	}
}

// Join returns a relation join by name, supporting dot-delimited paths (e.g. "AllocatedVehicle.Model").
func (s *StructTableModel) Join(name string) *schema.RelationJoin {
	if strings.Contains(name, ".") {
		parts := strings.Split(name, ".")
		parentName := parts[0]
		childPath := strings.Join(parts[1:], ".")

		parentJoin := s.Join(parentName)
		if parentJoin == nil || parentJoin.JoinModel == nil {
			return nil
		}

		childJoin := parentJoin.JoinModel.Join(childPath)
		if childJoin != nil {
			s.activateJoin(parentJoin)
			if childSM, ok := parentJoin.JoinModel.(*StructTableModel); ok {
				childSM.activateJoin(childJoin)
			}
			return childJoin
		}
		return nil
	}

	key := strings.ToLower(name)
	if j, ok := s.joinMap[key]; ok {
		s.activateJoin(j)
		return j
	}
	return nil
}

// BuildTableModelFromStruct introspects a Go struct, pointer to struct, or slice of structs
// and builds a schema.TableModel with its fields and relationships.
func BuildTableModelFromStruct(m any) schema.TableModel {
	if m == nil {
		return nil
	}

	t := reflect.TypeOf(m)
	for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil
	}

	// Avoid introspecting primitive wrapper structs
	if t == reflect.TypeOf(time.Time{}) || t == reflect.TypeOf(uuid.UUID{}) {
		return nil
	}

	if cached, ok := structModelCache.Load(t); ok {
		if tm, ok := cached.(*StructTableModel); ok {
			return tm.Clone()
		}
	}

	tm := buildStructTableModel(t, "", make(map[reflect.Type]bool))
	if tm != nil {
		structModelCache.Store(t, tm)
		return tm.Clone()
	}
	return nil
}

func buildStructTableModel(t reflect.Type, parentAlias string, visited map[reflect.Type]bool) *StructTableModel {
	if visited[t] {
		return nil
	}
	visited[t] = true

	tableName := resolveTableName(t)
	alias := resolveTableAlias(t, tableName, parentAlias)

	tbl := schema.NewTable(tableName)
	tbl.SQLAlias = alias

	tm := &StructTableModel{
		table:     tbl,
		joinMap:   make(map[string]*schema.RelationJoin),
		modelType: t,
		isStruct:  true,
	}

	inspectStructFieldsAndRelations(t, tm, parentAlias, visited)

	return tm
}

func resolveTableName(t reflect.Type) string {
	zero := reflect.New(t)
	if named, ok := zero.Interface().(interface{ TableName() string }); ok {
		return named.TableName()
	}

	// Check model or bun tags on fields (e.g. embedded BaseModel)
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		for _, tagKey := range []string{"model", "bun"} {
			tagVal := field.Tag.Get(tagKey)
			if tagVal != "" {
				for _, part := range strings.Split(tagVal, ",") {
					part = strings.TrimSpace(part)
					if strings.HasPrefix(part, "table:") {
						return strings.TrimPrefix(part, "table:")
					}
				}
			}
		}
	}

	return inferTable(t, "")
}

func resolveTableAlias(t reflect.Type, tableName string, parentAlias string) string {
	if parentAlias != "" {
		return parentAlias
	}

	// Check model or bun tags for explicit alias
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		for _, tagKey := range []string{"model", "bun"} {
			tagVal := field.Tag.Get(tagKey)
			if tagVal != "" {
				for _, part := range strings.Split(tagVal, ",") {
					part = strings.TrimSpace(part)
					if strings.HasPrefix(part, "alias:") {
						return strings.TrimPrefix(part, "alias:")
					}
				}
			}
		}
	}

	raw := tableName
	if dot := strings.LastIndex(raw, "."); dot >= 0 {
		raw = raw[dot+1:]
	}

	parts := strings.Split(raw, "_")
	if len(parts) > 1 {
		var b strings.Builder
		for _, p := range parts {
			if len(p) > 0 {
				b.WriteByte(p[0])
			}
		}
		return b.String()
	}

	return raw
}

func inspectStructFieldsAndRelations(t reflect.Type, tm *StructTableModel, currentAlias string, visited map[reflect.Type]bool) {
	alias := tm.table.SQLAlias

	// Collect field names present on this struct for foreign key matching
	fieldColumnMap := make(map[string]string)
	collectFieldColumnMap(t, fieldColumnMap)

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}

		if field.Anonymous {
			// Embedded struct (e.g. IDModel, CoreModel)
			inspectStructFieldsAndRelations(field.Type, tm, currentAlias, visited)
			continue
		}

		// Check if field is a relation (struct pointer or slice of structs)
		fieldType := field.Type
		isSlice := fieldType.Kind() == reflect.Slice
		if isSlice {
			fieldType = fieldType.Elem()
		}
		for fieldType.Kind() == reflect.Pointer {
			fieldType = fieldType.Elem()
		}

		if fieldType.Kind() == reflect.Struct &&
			fieldType != reflect.TypeOf(time.Time{}) &&
			fieldType != reflect.TypeOf(uuid.UUID{}) {

			relName := field.Name
			var relType schema.RelationType
			if isSlice {
				relType = schema.HasManyRelation
			} else {
				relType = schema.BelongsToRelation
			}

			// Target table alias for joins:
			// e.g. "allocated_driver" or "allocated_vehicle__model"
			targetRelAlias := snakeCase(relName)
			if currentAlias != "" {
				targetRelAlias = currentAlias + "__" + targetRelAlias
			}

			visitedCopy := make(map[reflect.Type]bool, len(visited)+1)
			for k, v := range visited {
				visitedCopy[k] = v
			}

			targetModel := buildStructTableModel(fieldType, targetRelAlias, visitedCopy)
			if targetModel == nil {
				continue
			}

			targetTable := targetModel.Table()
			targetTable.SQLAlias = targetRelAlias

			fk, pk := resolveRelationKeys(t, field, fieldType, relType, fieldColumnMap)

			// Qualify fk with parent alias if nested
			if currentAlias != "" && fk != "" && !strings.Contains(fk, ".") {
				fk = currentAlias + "." + fk
			} else if alias != "" && fk != "" && !strings.Contains(fk, ".") {
				fk = alias + "." + fk
			}

			rel := &schema.Relation{
				Name:       relName,
				Type:       relType,
				JoinTable:  targetTable,
				ForeignKey: fk,
				TargetKey:  pk,
			}

			rj := schema.NewRelationJoin(rel, targetModel)
			tm.joins = append(tm.joins, rj)
			tm.joinMap[strings.ToLower(relName)] = rj
			continue
		}

		// Normal column field
		colName := taggedColumn(field)
		if colName == "-" {
			continue
		}
		if colName == "" {
			colName = snakeCase(field.Name)
		}

		isPK := colName == "id" || field.Name == "ID"
		tm.table.AddField(&schema.Field{
			Name:     field.Name,
			SQLName:  colName,
			DataType: field.Type.String(),
			IsPK:     isPK,
		})
	}
}

func collectFieldColumnMap(t reflect.Type, out map[string]string) {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		if f.Anonymous {
			collectFieldColumnMap(f.Type, out)
			continue
		}
		col := taggedColumn(f)
		if col == "" || col == "-" {
			col = snakeCase(f.Name)
		}
		out[f.Name] = col
		out[strings.ToLower(f.Name)] = col
		out[snakeCase(f.Name)] = col
	}
}

func resolveRelationKeys(sourceType reflect.Type, relField reflect.StructField, targetType reflect.Type, relType schema.RelationType, fieldMap map[string]string) (string, string) {
	// 1. Check explicit tag: bun:"rel:belongs-to,join:fk=pk" or model:"rel:...,join:fk=pk"
	for _, tagKey := range []string{"model", "bun"} {
		tagVal := relField.Tag.Get(tagKey)
		if tagVal != "" {
			for _, part := range strings.Split(tagVal, ",") {
				part = strings.TrimSpace(part)
				if strings.HasPrefix(part, "join:") {
					joinDef := strings.TrimPrefix(part, "join:")
					eq := strings.Split(joinDef, "=")
					if len(eq) == 2 {
						return strings.TrimSpace(eq[0]), strings.TrimSpace(eq[1])
					}
				}
			}
		}
	}

	pk := "id"

	if relType == schema.BelongsToRelation {
		// Look for matching foreign key on source struct:
		// e.g. AllocatedDriver -> AllocatedDriverID / allocated_driver_id
		candidates := []string{
			relField.Name + "ID",
			snakeCase(relField.Name) + "_id",
			relField.Name + "Id",
		}

		for _, c := range candidates {
			if col, ok := fieldMap[c]; ok {
				return col, pk
			}
			if col, ok := fieldMap[strings.ToLower(c)]; ok {
				return col, pk
			}
		}

		return snakeCase(relField.Name) + "_id", pk
	}

	// HasMany relation: look for matching key on target struct:
	// e.g. RegularTrip -> RegularTripID / regular_trip_id / trip_id on target
	targetFieldMap := make(map[string]string)
	collectFieldColumnMap(targetType, targetFieldMap)

	srcName := sourceType.Name()
	targetCandidates := []string{
		srcName + "ID",
		snakeCase(srcName) + "_id",
		"TripID",
		"trip_id",
		"ParentID",
		"parent_id",
	}

	for _, c := range targetCandidates {
		if col, ok := targetFieldMap[c]; ok {
			return col, pk
		}
		if col, ok := targetFieldMap[strings.ToLower(c)]; ok {
			return col, pk
		}
	}

	return snakeCase(srcName) + "_id", pk
}
