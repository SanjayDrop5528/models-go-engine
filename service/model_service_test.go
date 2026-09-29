package service

import (
	"context"
	"testing"

	"github.com/SanjayDrop5528/models-go-engine/model"
	"github.com/SanjayDrop5528/models-go-engine/registry"
)

func TestGetActiveSynthesizesDistinctInverseRelations(t *testing.T) {
	reg := registry.NewModelRegistry()
	address := &model.Model{ID: "address", Name: "Address", StorageName: "addresses"}
	order := &model.Model{
		ID: "order", Name: "Order", StorageName: "orders",
		Relations: []model.Relation{
			{Name: "BillingAddress", Type: model.RelManyToOne, TargetModel: "address", ForeignKey: "billing_address_id", TargetKey: "id", LoadWithChildren: true},
			{Name: "ShippingAddress", Type: model.RelManyToOne, TargetModel: "address", ForeignKey: "shipping_address_id", TargetKey: "id"},
		},
	}
	if _, err := reg.SetActive(address.ID, address); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.SetActive(order.ID, order); err != nil {
		t.Fatal(err)
	}

	service := NewModelService(reg)
	active, err := service.GetActive(context.Background(), address.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(active.Relations) != 2 {
		t.Fatalf("expected two inverse relations, got %+v", active.Relations)
	}
	byName := map[string]model.Relation{}
	for _, relation := range active.Relations {
		byName[relation.Name] = relation
	}
	if byName["BillingAddressOrders"].ForeignKey != "billing_address_id" || !byName["BillingAddressOrders"].LoadWithChildren {
		t.Fatalf("billing inverse relation is incorrect: %+v", byName["BillingAddressOrders"])
	}
	if byName["ShippingAddressOrders"].ForeignKey != "shipping_address_id" {
		t.Fatalf("shipping inverse relation is incorrect: %+v", byName["ShippingAddressOrders"])
	}
}
