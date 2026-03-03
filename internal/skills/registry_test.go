package skills

import "testing"

func TestRegistryOrderingIsDeterministic(t *testing.T) {
	r := NewRegistry()
	r.Register(&Skill{Name: "zeta", Description: "z", Parameters: map[string]any{}})
	r.Register(&Skill{Name: "alpha", Description: "a", Parameters: map[string]any{}})
	r.Register(&Skill{Name: "beta", Description: "b", Parameters: map[string]any{}})

	names := r.Names()
	want := []string{"alpha", "beta", "zeta"}
	if len(names) != len(want) {
		t.Fatalf("len(names)=%d, want=%d", len(names), len(want))
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("names[%d]=%q, want=%q", i, names[i], want[i])
		}
	}

	tools := r.List()
	for i := range want {
		if tools[i]["name"] != want[i] {
			t.Fatalf("tools[%d].name=%v, want=%q", i, tools[i]["name"], want[i])
		}
	}
}
