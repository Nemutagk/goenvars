package goenvarsaws

import "testing"

func TestLookup(t *testing.T) {
	s := source{"A": "1"}

	if v, ok := s.Lookup("A"); !ok || v != "1" {
		t.Fatalf("Lookup(A) = %q, %v; se esperaba \"1\", true", v, ok)
	}
	if v, ok := s.Lookup("B"); ok || v != "" {
		t.Fatalf("Lookup(B) = %q, %v; se esperaba \"\", false", v, ok)
	}
}

func TestFlatten(t *testing.T) {
	s := flatten(map[string]any{
		"S": "txt", "N": float64(1000000), "B": true, "NULL": nil,
		"OBJ": map[string]any{"a": float64(1)},
	})

	casos := map[string]string{"S": "txt", "N": "1000000", "B": "true", "OBJ": `{"a":1}`}
	for k, want := range casos {
		if got, ok := s.Lookup(k); !ok || got != want {
			t.Errorf("%s = %q, %v; se esperaba %q", k, got, ok, want)
		}
	}
	if _, ok := s.Lookup("NULL"); ok {
		t.Error("null debería tratarse como ausente")
	}
}
