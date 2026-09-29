package crud

import (
	"bytes"
	"context"
	"log"
	"strings"
	"testing"

	"github.com/SanjayDrop5528/models-go-engine/adapter"
	"github.com/SanjayDrop5528/models-go-engine/model"
	"github.com/SanjayDrop5528/models-go-engine/query"
)

type testModelResolver map[string]*model.Model

func (r testModelResolver) GetActive(_ context.Context, id string) (*model.Model, error) {
	return r[id], nil
}

func TestWithAutomaticRelations(t *testing.T) {
	m := &model.Model{Relations: []model.Relation{
		{Name: "Customer", LoadWithChildren: true},
		{Name: "OrderProducts", LoadWithChildren: false},
	}}

	t.Run("automatically includes enabled model relation", func(t *testing.T) {
		q := withAutomaticRelations(m, query.New())
		if len(q.RelationSpecs) != 1 || q.RelationSpecs[0].Name != "Customer" || !q.RelationSpecs[0].LoadWithChildren {
			t.Fatalf("expected Customer to load automatically, got %+v", q.RelationSpecs)
		}
	})

	t.Run("keeps disabled relation lazy", func(t *testing.T) {
		q := withAutomaticRelations(m, query.New())
		for _, relation := range q.Relations {
			if relation == "OrderProducts" {
				t.Fatal("OrderProducts must stay lazy when load_with_children is false")
			}
		}
	})

	t.Run("explicit request loads a disabled relation", func(t *testing.T) {
		q := withAutomaticRelations(m, query.New().Relation("OrderProducts"))
		found := false
		for _, relation := range q.Relations {
			if relation == "OrderProducts" {
				found = true
			}
		}
		if !found {
			t.Fatal("explicit OrderProducts request was removed")
		}
	})

	t.Run("does not duplicate explicitly requested automatic relation", func(t *testing.T) {
		q := withAutomaticRelations(m, query.New().Relation("Customer"))
		count := 0
		for _, relation := range q.Relations {
			if relation == "Customer" {
				count++
			}
		}
		if count != 1 {
			t.Fatalf("expected one Customer relation, got %d", count)
		}
	})
}

func TestEngineHydratesDeclaredObjectAndArrayRelations(t *testing.T) {
	ctx := context.Background()
	registry := adapter.NewRegistry()
	store := adapter.NewMockAdapter()
	registry.Register("mock", store)

	customer := &model.Model{ID: "customer", Name: "Customer", StorageName: "customers", Database: "mock", PrimaryKey: &model.PrimaryKey{Columns: []string{"id"}}}
	orderProduct := &model.Model{ID: "order_product", Name: "OrderProduct", StorageName: "order_products", Database: "mock", PrimaryKey: &model.PrimaryKey{Columns: []string{"id"}}}
	order := &model.Model{
		ID: "order", Name: "Order", StorageName: "orders", Database: "mock", PrimaryKey: &model.PrimaryKey{Columns: []string{"id"}},
		Relations: []model.Relation{
			{Name: "Customer", Type: model.RelManyToOne, TargetModel: "customer", ForeignKey: "customer_id", TargetKey: "id", LoadWithChildren: true},
			{Name: "OrderProducts", Type: model.RelOneToMany, TargetModel: "order_product", ForeignKey: "order_id", TargetKey: "id", LoadWithChildren: true},
		},
	}

	_, _ = store.Create(ctx, customer.Ref(), map[string]any{"id": "customer-1", "name": "Ada"})
	_, _ = store.Create(ctx, order.Ref(), map[string]any{"id": "order-1", "customer_id": "customer-1"})
	_, _ = store.Create(ctx, orderProduct.Ref(), map[string]any{"id": "line-1", "order_id": "order-1", "quantity": 2})
	_, _ = store.Create(ctx, orderProduct.Ref(), map[string]any{"id": "line-2", "order_id": "order-1", "quantity": 1})

	engine := NewEngine(registry)
	engine.SetModelResolver(testModelResolver{"customer": customer, "order_product": orderProduct})
	rows, _, err := engine.Find(ctx, order, query.New())
	if err != nil {
		t.Fatalf("Find failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one order, got %d", len(rows))
	}
	customerValue, ok := rows[0]["Customer"].(map[string]any)
	if !ok || customerValue["name"] != "Ada" {
		t.Fatalf("expected hydrated Customer object, got %+v", rows[0]["Customer"])
	}
	products, ok := rows[0]["OrderProducts"].([]map[string]any)
	if !ok || len(products) != 2 {
		t.Fatalf("expected two hydrated OrderProducts, got %T %+v", rows[0]["OrderProducts"], rows[0]["OrderProducts"])
	}
}

func TestEngineHydratesNestedRelationsAndPreservesProjection(t *testing.T) {
	ctx := context.Background()
	registry := adapter.NewRegistry()
	store := adapter.NewMockAdapter()
	registry.Register("mock", store)

	country := &model.Model{ID: "country", Name: "Country", StorageName: "countries", Database: "mock", PrimaryKey: &model.PrimaryKey{Columns: []string{"id"}}}
	manager := &model.Model{ID: "manager", Name: "Manager", StorageName: "managers", Database: "mock", PrimaryKey: &model.PrimaryKey{Columns: []string{"id"}}}
	customer := &model.Model{
		ID: "customer", Name: "Customer", StorageName: "customers", Database: "mock", PrimaryKey: &model.PrimaryKey{Columns: []string{"id"}},
		Relations: []model.Relation{
			{Name: "Country", Type: model.RelManyToOne, TargetModel: "country", ForeignKey: "country_id", TargetKey: "id", LoadWithChildren: true},
			{Name: "Manager", Type: model.RelManyToOne, TargetModel: "manager", ForeignKey: "manager_id", TargetKey: "id"},
		},
	}
	order := &model.Model{
		ID: "order", Name: "Order", StorageName: "orders", Database: "mock", PrimaryKey: &model.PrimaryKey{Columns: []string{"id"}},
		Relations: []model.Relation{{Name: "Customer", Type: model.RelManyToOne, TargetModel: "customer", ForeignKey: "customer_id", TargetKey: "id"}},
	}

	_, _ = store.Create(ctx, country.Ref(), map[string]any{"id": "country-1", "name": "India"})
	_, _ = store.Create(ctx, manager.Ref(), map[string]any{"id": "manager-1", "name": "Grace"})
	_, _ = store.Create(ctx, customer.Ref(), map[string]any{"id": "customer-1", "name": "Ada", "country_id": "country-1", "manager_id": "manager-1"})
	_, _ = store.Create(ctx, order.Ref(), map[string]any{"id": "order-1", "customer_id": "customer-1", "secret": "hidden"})

	engine := NewEngine(registry)
	engine.SetModelResolver(testModelResolver{"country": country, "manager": manager, "customer": customer})
	q := query.New().Column("id").Relation("Customer.Country").Relation("Customer.Manager")
	rows, _, err := engine.Find(ctx, order, q)
	if err != nil {
		t.Fatalf("Find failed: %v", err)
	}
	if _, leaked := rows[0]["customer_id"]; leaked {
		t.Fatal("internal hydration key customer_id leaked through the requested projection")
	}
	customerValue := rows[0]["Customer"].(map[string]any)
	if customerValue["Country"].(map[string]any)["name"] != "India" {
		t.Fatalf("expected nested Country, got %+v", customerValue)
	}
	if customerValue["Manager"].(map[string]any)["name"] != "Grace" {
		t.Fatalf("expected second nested relation under the same root, got %+v", customerValue)
	}

	autoRows, _, err := engine.Find(ctx, order, query.New().Relation("Customer"))
	if err != nil {
		t.Fatalf("automatic nested Find failed: %v", err)
	}
	autoCustomer := autoRows[0]["Customer"].(map[string]any)
	if autoCustomer["Country"].(map[string]any)["name"] != "India" {
		t.Fatalf("expected load_with_children to discover Country automatically, got %+v", autoCustomer)
	}
	if _, loaded := autoCustomer["Manager"]; loaded {
		t.Fatalf("lazy Manager relation should not auto-load, got %+v", autoCustomer["Manager"])
	}
}

func TestEngineHydratesManyToManyRelation(t *testing.T) {
	ctx := context.Background()
	registry := adapter.NewRegistry()
	store := adapter.NewMockAdapter()
	registry.Register("mock", store)

	tag := &model.Model{ID: "tag", Name: "Tag", StorageName: "tags", Database: "mock", PrimaryKey: &model.PrimaryKey{Columns: []string{"id"}}}
	productTag := &model.Model{ID: "product_tag", Name: "ProductTag", StorageName: "product_tags", Database: "mock", PrimaryKey: &model.PrimaryKey{Columns: []string{"id"}}}
	product := &model.Model{
		ID: "product", Name: "Product", StorageName: "products", Database: "mock", PrimaryKey: &model.PrimaryKey{Columns: []string{"id"}},
		Relations: []model.Relation{{
			Name: "Tags", Type: model.RelManyToMany, TargetModel: "tag", ForeignKey: "id", TargetKey: "id",
			JunctionModel: "product_tag", JunctionSourceKey: "product_id", JunctionTargetKey: "tag_id", LoadWithChildren: true,
		}},
	}

	_, _ = store.Create(ctx, product.Ref(), map[string]any{"id": "product-1", "name": "Keyboard"})
	_, _ = store.Create(ctx, tag.Ref(), map[string]any{"id": "tag-1", "name": "Hardware"})
	_, _ = store.Create(ctx, tag.Ref(), map[string]any{"id": "tag-2", "name": "Office"})
	_, _ = store.Create(ctx, productTag.Ref(), map[string]any{"id": "link-1", "product_id": "product-1", "tag_id": "tag-1"})
	_, _ = store.Create(ctx, productTag.Ref(), map[string]any{"id": "link-2", "product_id": "product-1", "tag_id": "tag-2"})

	engine := NewEngine(registry)
	engine.SetModelResolver(testModelResolver{"tag": tag, "product_tag": productTag})
	rows, _, err := engine.Find(ctx, product, query.New())
	if err != nil {
		t.Fatalf("Find failed: %v", err)
	}
	tags, ok := rows[0]["Tags"].([]map[string]any)
	if !ok || len(tags) != 2 {
		t.Fatalf("expected two many-to-many tags, got %T %+v", rows[0]["Tags"], rows[0]["Tags"])
	}
}

func TestAutomaticRelationDebugLogging(t *testing.T) {
	previousWriter := log.Writer()
	previousFlags := log.Flags()
	t.Cleanup(func() {
		log.SetOutput(previousWriter)
		log.SetFlags(previousFlags)
	})

	var output bytes.Buffer
	log.SetOutput(&output)
	log.SetFlags(0)
	m := &model.Model{ID: "order", Relations: []model.Relation{{Name: "Customer", LoadWithChildren: true}}}

	withAutomaticRelations(m, query.New())
	if output.Len() != 0 {
		t.Fatalf("debug=false must not emit query logs, got %q", output.String())
	}

	q := query.New()
	q.Debug = true
	withAutomaticRelations(m, q)
	if !strings.Contains(output.String(), "decision=auto-load") || !strings.Contains(output.String(), "relation=Customer") || !strings.Contains(output.String(), "[query-") {
		t.Fatalf("debug=true must log automatic relation resolution, got %q", output.String())
	}
}
