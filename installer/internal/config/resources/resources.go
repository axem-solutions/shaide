// Package resources sizes the work the installer asks of the machine it runs
// on.
//
// Transfers scale with concurrency, and left unbounded they take whatever the
// host has. That is fine on a build server and ruinous on a laptop, where the
// installer shares the machine with a desktop: the container lives outside the
// slice systemd-oomd manages, so it never gets killed itself and starves
// everything else instead.
//
// The budget is therefore a share of what is available rather than a fixed
// number, so a bigger machine is used harder without any tuning.
package resources

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
)

const (
	// DefaultPercent leaves half the machine for everything else.
	DefaultPercent = 50

	// MinCPUs keeps a budget usable on a small machine: below one worker a
	// transfer cannot make progress at all.
	MinCPUs = 1

	// MinMemoryBytes floors the memory share so a percentage of a small
	// container does not round down to nothing.
	MinMemoryBytes = 512 << 20
)

// Budget is the share of the machine a transfer may use.
type Budget struct {
	CPUs   int
	Memory int64

	// Detected records what the budget was derived from, for logging. It is
	// the cgroup limit when one is set, otherwise what the host reports.
	DetectedCPUs   int
	DetectedMemory int64
	FromCgroup     bool
}

func (b Budget) String() string {
	return fmt.Sprintf(
		"%d of %d CPUs, %s of %s memory",
		b.CPUs, b.DetectedCPUs, formatBytes(b.Memory), formatBytes(b.DetectedMemory),
	)
}

// Limits are the operator's overrides. Zero values take the defaults.
type Limits struct {
	CPUPercent    int
	MemoryPercent int
}

func (l Limits) validate() error {
	for name, percent := range map[string]int{
		"CPU":    l.CPUPercent,
		"memory": l.MemoryPercent,
	} {
		if percent < 0 || percent > 100 {
			return fmt.Errorf("%s percent must be between 1 and 100, got %d", name, percent)
		}
	}

	return nil
}

// Detect resolves the budget from the limits the process actually runs under.
//
// A container is told its cgroup limits but not by runtime.NumCPU, which
// reports every host CPU regardless of the quota. Sizing thread pools from that
// number oversubscribes the quota: the workers run flat out until the period's
// budget is spent and are then throttled together, which reads as a pegged CPU
// while doing less work.
func Detect(limits Limits) (Budget, error) {
	if err := limits.validate(); err != nil {
		return Budget{}, err
	}

	cpus, cpusFromCgroup := detectCPUs()
	memory, memoryFromCgroup := detectMemory()

	budget := Budget{
		DetectedCPUs:   cpus,
		DetectedMemory: memory,
		FromCgroup:     cpusFromCgroup || memoryFromCgroup,
	}

	budget.CPUs = share(cpus, percentOr(limits.CPUPercent), MinCPUs)
	budget.Memory = int64(share(int(memory>>20), percentOr(limits.MemoryPercent), int(MinMemoryBytes>>20))) << 20

	return budget, nil
}

func percentOr(percent int) int {
	if percent <= 0 {
		return DefaultPercent
	}

	return percent
}

func share(total, percent, minimum int) int {
	if total <= 0 {
		return minimum
	}

	value := total * percent / 100
	if value < minimum {
		return minimum
	}

	return value
}

// detectCPUs prefers the cgroup v2 quota, which is what --cpus sets, over the
// host CPU count that runtime.NumCPU reports inside a container.
func detectCPUs() (int, bool) {
	quota, period, ok := readCPUMax()
	if ok && period > 0 {
		cpus := int(quota / period)
		if quota%period != 0 {
			cpus++
		}
		if cpus > 0 {
			return cpus, true
		}
	}

	return runtime.NumCPU(), false
}

func readCPUMax() (int64, int64, bool) {
	content, err := os.ReadFile("/sys/fs/cgroup/cpu.max")
	if err != nil {
		return 0, 0, false
	}

	fields := strings.Fields(string(content))
	if len(fields) != 2 || fields[0] == "max" {
		return 0, 0, false
	}

	quota, quotaErr := strconv.ParseInt(fields[0], 10, 64)
	period, periodErr := strconv.ParseInt(fields[1], 10, 64)
	if quotaErr != nil || periodErr != nil {
		return 0, 0, false
	}

	return quota, period, true
}

// detectMemory prefers the cgroup v2 limit, which is what --memory sets.
func detectMemory() (int64, bool) {
	if limit, ok := readMemoryMax(); ok {
		return limit, true
	}

	return readMemTotal(), false
}

func readMemoryMax() (int64, bool) {
	content, err := os.ReadFile("/sys/fs/cgroup/memory.max")
	if err != nil {
		return 0, false
	}

	value := strings.TrimSpace(string(content))
	if value == "max" {
		return 0, false
	}

	limit, err := strconv.ParseInt(value, 10, 64)
	if err != nil || limit <= 0 {
		return 0, false
	}

	return limit, true
}

func readMemTotal() int64 {
	content, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}

	for _, line := range strings.Split(string(content), "\n") {
		if !strings.HasPrefix(line, "MemTotal:") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 {
			return 0
		}

		kilobytes, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			return 0
		}

		return kilobytes << 10
	}

	return 0
}

func formatBytes(bytes int64) string {
	const unit = 1 << 10

	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}

	value := float64(bytes)
	units := []string{"KB", "MB", "GB", "TB"}

	for _, suffix := range units {
		value /= unit
		if value < unit {
			return fmt.Sprintf("%.1f %s", value, suffix)
		}
	}

	return fmt.Sprintf("%.1f PB", value/unit)
}
