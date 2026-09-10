package resources

import (
	"strings"
	"testing"
)

// Half the machine is the point of the default: the installer shares the host
// with a desktop, and its container sits outside the slice systemd-oomd
// manages, so an unbounded transfer starves everything else instead of itself.
func TestDetectDefaultsToHalf(t *testing.T) {
	budget, err := Detect(Limits{})
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}

	if budget.DetectedCPUs <= 0 {
		t.Fatalf("detected %d CPUs, want a positive count", budget.DetectedCPUs)
	}

	want := budget.DetectedCPUs / 2
	if want < MinCPUs {
		want = MinCPUs
	}
	if budget.CPUs != want {
		t.Errorf("CPUs = %d, want %d (half of %d)", budget.CPUs, want, budget.DetectedCPUs)
	}

	if budget.CPUs > budget.DetectedCPUs {
		t.Errorf("budget %d exceeds what was detected, %d", budget.CPUs, budget.DetectedCPUs)
	}
}

func TestDetectHonoursOverrides(t *testing.T) {
	full, err := Detect(Limits{CPUPercent: 100, MemoryPercent: 100})
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}

	half, err := Detect(Limits{CPUPercent: 50, MemoryPercent: 50})
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}

	if full.CPUs < half.CPUs {
		t.Errorf("100%% gave %d CPUs, less than 50%% at %d", full.CPUs, half.CPUs)
	}
	if full.CPUs != full.DetectedCPUs {
		t.Errorf("100%% gave %d CPUs, want all %d", full.CPUs, full.DetectedCPUs)
	}
}

// A percentage of a small machine must still leave enough to make progress;
// zero workers would stall the transfer entirely.
func TestDetectFloorsAtSomethingUsable(t *testing.T) {
	budget, err := Detect(Limits{CPUPercent: 1, MemoryPercent: 1})
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}

	if budget.CPUs < MinCPUs {
		t.Errorf("CPUs = %d, want at least %d", budget.CPUs, MinCPUs)
	}
	if budget.Memory < MinMemoryBytes {
		t.Errorf("Memory = %d, want at least %d", budget.Memory, MinMemoryBytes)
	}
}

func TestDetectRejectsNonsensePercentages(t *testing.T) {
	for _, limits := range []Limits{
		{CPUPercent: 101},
		{MemoryPercent: 101},
		{CPUPercent: -1},
		{MemoryPercent: -5},
	} {
		if _, err := Detect(limits); err == nil {
			t.Errorf("Detect(%+v) succeeded, want a rejection", limits)
		}
	}
}

// The budget is logged, so it has to say what it decided and what it decided
// from - a number with no denominator tells an operator nothing.
func TestBudgetStringNamesBothSides(t *testing.T) {
	budget := Budget{CPUs: 4, DetectedCPUs: 8, Memory: 8 << 30, DetectedMemory: 16 << 30}

	got := budget.String()
	for _, want := range []string{"4", "8", "GB"} {
		if !strings.Contains(got, want) {
			t.Errorf("String() = %q, want it to mention %q", got, want)
		}
	}
}
