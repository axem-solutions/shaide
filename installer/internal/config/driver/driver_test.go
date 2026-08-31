package driver

import "testing"

func TestGPUNodeCapabilityCompatible(t *testing.T) {
	tests := []struct {
		name         string
		driver       Version
		runtime      Version
		requiredCUDA Version
		want         bool
	}{
		{
			name:         "CUDA 12 minor compatibility accepts driver 570",
			driver:       Version{Major: 570, Minor: 211, Revision: 1},
			runtime:      Version{Major: 12, Minor: 8},
			requiredCUDA: Version{Major: 12, Minor: 9},
			want:         true,
		},
		{
			name:         "CUDA 12 accepts exact minimum driver",
			driver:       Version{Major: 525, Minor: 60, Revision: 13},
			runtime:      Version{Major: 12},
			requiredCUDA: Version{Major: 12, Minor: 9},
			want:         true,
		},
		{
			name:         "CUDA 12 rejects driver below minimum",
			driver:       Version{Major: 525, Minor: 60, Revision: 12},
			runtime:      Version{Major: 12, Minor: 9},
			requiredCUDA: Version{Major: 12, Minor: 9},
			want:         false,
		},
		{
			name:         "CUDA 13 rejects driver 570",
			driver:       Version{Major: 570, Minor: 211, Revision: 1},
			runtime:      Version{Major: 12, Minor: 8},
			requiredCUDA: Version{Major: 13},
			want:         false,
		},
		{
			name:         "CUDA 13 accepts exact minimum driver",
			driver:       Version{Major: 580, Minor: 65, Revision: 6},
			runtime:      Version{Major: 13},
			requiredCUDA: Version{Major: 13},
			want:         true,
		},
		{
			name:         "unknown CUDA family uses conservative runtime comparison",
			driver:       Version{Major: 999},
			runtime:      Version{Major: 13, Minor: 1},
			requiredCUDA: Version{Major: 14},
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			capability := GPUNodeCapability{
				DriverVersion:      tt.driver,
				CUDARuntimeVersion: tt.runtime,
			}

			if got := capability.Compatible(tt.requiredCUDA); got != tt.want {
				t.Fatalf("Compatible(%s) = %t, want %t", tt.requiredCUDA, got, tt.want)
			}
		})
	}
}
