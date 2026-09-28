package list

import "testing"

func TestKeyedItemsRoundTrips(t *testing.T) {
	m := New(Options{})
	m.SetKeyedItems([]KeyedItem{{Key: "a", Display: "alpha", Data: 7}})
	got := m.KeyedItems()
	if len(got) != 1 || got[0].Key != "a" || got[0].Display != "alpha" || got[0].Data != 7 {
		t.Fatalf("KeyedItems = %+v", got)
	}
	m.SetItems([]string{"x"})
	if m.KeyedItems() != nil {
		t.Error("anonymous items reported as keyed")
	}
}
