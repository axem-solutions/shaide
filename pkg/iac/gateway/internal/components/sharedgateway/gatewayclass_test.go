package sharedgateway

import "testing"

func TestIsAccepted(t *testing.T) {
	accepted := func(status string) map[string]any {
		return map[string]any{
			"status": map[string]any{
				"conditions": []any{
					map[string]any{"type": "Accepted", "status": status},
				},
			},
		}
	}

	tests := []struct {
		name string
		obj  map[string]any
		want bool
	}{
		{"accepted", accepted("True"), true},
		{"not accepted", accepted("False"), false},
		{"missing status", map[string]any{}, false},
		{
			"unrelated condition",
			map[string]any{"status": map[string]any{"conditions": []any{
				map[string]any{"type": "SupportedVersion", "status": "True"},
			}}},
			false,
		},
		{
			"malformed condition",
			map[string]any{"status": map[string]any{"conditions": []any{"invalid"}}},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isAccepted(tt.obj); got != tt.want {
				t.Fatalf("isAccepted() = %v, want %v", got, tt.want)
			}
		})
	}
}
