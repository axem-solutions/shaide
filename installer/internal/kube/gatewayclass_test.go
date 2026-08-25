package kube

import "testing"

func TestIsGatewayClassAccepted(t *testing.T) {
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
		{"accepted true", accepted("True"), true},
		{"accepted false", accepted("False"), false},
		{"accepted unknown", accepted("Unknown"), false},
		{"no status at all", map[string]any{}, false},
		{
			name: "status without conditions",
			obj:  map[string]any{"status": map[string]any{}},
			want: false,
		},
		{
			// Only a non-Accepted condition present: a class that reports
			// something else must not be treated as usable.
			name: "unrelated condition only",
			obj: map[string]any{
				"status": map[string]any{
					"conditions": []any{
						map[string]any{"type": "SupportedVersion", "status": "True"},
					},
				},
			},
			want: false,
		},
		{
			// Accepted alongside other conditions, in any order.
			name: "accepted among several conditions",
			obj: map[string]any{
				"status": map[string]any{
					"conditions": []any{
						map[string]any{"type": "SupportedVersion", "status": "True"},
						map[string]any{"type": "Accepted", "status": "True"},
					},
				},
			},
			want: true,
		},
		{
			// Malformed entries must not panic.
			name: "conditions contain a non-map entry",
			obj: map[string]any{
				"status": map[string]any{
					"conditions": []any{"nonsense"},
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isGatewayClassAccepted(tt.obj); got != tt.want {
				t.Errorf("isGatewayClassAccepted() = %v, want %v", got, tt.want)
			}
		})
	}
}
