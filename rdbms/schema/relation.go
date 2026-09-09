package schema

import (
	"context"
	"fmt"
)

// RelationJoin represents a joined relation in a SelectQuery.
type RelationJoin struct {
	Relation                   *Relation
	JoinModel                  TableModel
	Columns                    []QueryWithArgs
	AdditionalJoinOnConditions []QueryWithArgs
	Apply                      any
	LoadWithChildren           bool // Controls whether child/nested relations and their columns are loaded
}

func NewRelationJoin(rel *Relation, joinModel TableModel) *RelationJoin {
	return &RelationJoin{
		Relation:         rel,
		JoinModel:        joinModel,
		LoadWithChildren: true, // Default to true, can be configured via RelationOpts
	}
}

func (j *RelationJoin) ApplyTo(q any) {
	// Custom apply hook if provided
}

func (j *RelationJoin) AppendAlias(gen QueryGen, b []byte) []byte {
	if j.Relation != nil && j.Relation.JoinTable != nil {
		alias := j.Relation.JoinTable.SQLAlias
		if alias == "" {
			alias = j.Relation.JoinTable.SQLName
		}
		if gen != nil {
			return gen.Dialect().AppendIdent(b, alias)
		}
		return append(b, alias...)
	}
	return b
}

func (j *RelationJoin) AppendAliasColumn(gen QueryGen, b []byte, col string) []byte {
	alias := ""
	if j.Relation != nil {
		alias = j.Relation.Name + "__" + col
	} else {
		alias = col
	}
	if gen != nil {
		return gen.Dialect().AppendIdent(b, alias)
	}
	return append(b, alias...)
}

func (j *RelationJoin) AppendHasOneJoin(gen QueryGen, b []byte, q any) ([]byte, error) {
	if j.Relation == nil || j.Relation.JoinTable == nil {
		return b, nil
	}

	b = append(b, "LEFT JOIN "...)
	tbl := j.Relation.JoinTable
	if gen != nil {
		b = gen.Dialect().AppendIdent(b, tbl.SQLName)
		if tbl.SQLAlias != "" && tbl.SQLAlias != tbl.SQLName {
			b = append(b, " AS "...)
			b = gen.Dialect().AppendIdent(b, tbl.SQLAlias)
		}
	} else {
		b = append(b, tbl.SQLName...)
	}

	b = append(b, " ON "...)
	fk := j.Relation.ForeignKey
	if fk == "" {
		fk = "id"
	}
	pk := j.Relation.TargetKey
	if pk == "" {
		pk = "id"
	}

	joinAlias := tbl.SQLAlias
	if joinAlias == "" {
		joinAlias = tbl.SQLName
	}

	if gen != nil {
		b = gen.Dialect().AppendIdent(b, fmt.Sprintf("%s.%s", joinAlias, pk))
		b = append(b, " = "...)
		// Parent key reference
		b = append(b, fmt.Sprintf("%s", fk)...)
	} else {
		b = append(b, fmt.Sprintf("%s.%s = %s", joinAlias, pk, fk)...)
	}

	for _, cond := range j.AdditionalJoinOnConditions {
		b = append(b, " AND ("...)
		var err error
		b, err = cond.AppendQuery(gen, b)
		if err != nil {
			return nil, err
		}
		b = append(b, ')')
	}

	return b, nil
}

func (j *RelationJoin) SelectMany(ctx context.Context, q any) error {
	// Hook for subquery selection when executing HasMany
	return nil
}

func (j *RelationJoin) SelectM2M(ctx context.Context, q any) error {
	// Hook for subquery selection when executing ManyToMany
	return nil
}
