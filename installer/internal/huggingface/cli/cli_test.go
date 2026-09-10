package cli

import (
	"strings"
	"testing"

	"github.com/axem-solutions/ai_platform/installer/internal/config/resources"
)

func envValue(env []string, key string) (string, bool) {
	prefix := key + "="
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			return strings.TrimPrefix(entry, prefix), true
		}
	}

	return "", false
}

func runner(budget resources.Budget, highPerformance bool) Runner {
	return New(Config{
		Path:            "hf",
		CacheDir:        "/var/shaide-installer/model-cache",
		Token:           "hf_token",
		XetDirName:      "xet",
		DefaultRevision: "main",
		Budget:          budget,
		HighPerformance: highPerformance,
	})
}

// Without caps Xet sizes itself from the host, which inside a container is not
// what the process may actually use.
func TestEnvCapsConcurrencyFromTheBudget(t *testing.T) {
	env := runner(resources.Budget{CPUs: 4, Memory: 8 << 30}, false).env()

	files, ok := envValue(env, "HF_XET_DATA_MAX_CONCURRENT_FILE_DOWNLOADS")
	if !ok || files != "4" {
		t.Errorf("concurrent file downloads = %q, want the budget's 4", files)
	}

	ranges, ok := envValue(env, "HF_XET_CLIENT_AC_MAX_DOWNLOAD_CONCURRENCY")
	if !ok || ranges != "16" {
		t.Errorf("download concurrency = %q, want 4 CPUs x %d", ranges, rangeGetsPerCPU)
	}

	cache, ok := envValue(env, "HF_XET_CHUNK_CACHE_SIZE_BYTES")
	if !ok || cache != "8589934592" {
		t.Errorf("chunk cache = %q, want the budget's memory share", cache)
	}

	if _, set := envValue(env, "HF_XET_HIGH_PERFORMANCE"); set {
		t.Error("high performance was enabled without being asked for")
	}
}

// High performance is the documented way to ask for the whole machine, so it
// must not be silently capped.
func TestEnvHighPerformanceSuppressesCaps(t *testing.T) {
	env := runner(resources.Budget{CPUs: 4, Memory: 8 << 30}, true).env()

	if value, ok := envValue(env, "HF_XET_HIGH_PERFORMANCE"); !ok || value != "1" {
		t.Errorf("HF_XET_HIGH_PERFORMANCE = %q, want 1", value)
	}
	if _, set := envValue(env, "HF_XET_DATA_MAX_CONCURRENT_FILE_DOWNLOADS"); set {
		t.Error("a cap was set alongside high performance; they contradict each other")
	}
}

// A zero budget means nothing was detected, and inventing a cap from it would
// be worse than leaving Xet's own defaults in place.
func TestEnvZeroBudgetSetsNoCaps(t *testing.T) {
	env := runner(resources.Budget{}, false).env()

	for _, key := range []string{
		"HF_XET_DATA_MAX_CONCURRENT_FILE_DOWNLOADS",
		"HF_XET_CLIENT_AC_MAX_DOWNLOAD_CONCURRENCY",
		"HF_XET_CHUNK_CACHE_SIZE_BYTES",
		"HF_XET_HIGH_PERFORMANCE",
	} {
		if _, set := envValue(env, key); set {
			t.Errorf("%s was set from an empty budget", key)
		}
	}
}

func TestEnvKeepsTheCacheAndTokenSettings(t *testing.T) {
	env := runner(resources.Budget{CPUs: 2, Memory: 1 << 30}, false).env()

	for key, want := range map[string]string{
		"HF_TOKEN":                     "hf_token",
		"HF_HOME":                      "/var/shaide-installer/model-cache",
		"HF_HUB_DISABLE_PROGRESS_BARS": "1",
		"HF_XET_CACHE":                 "/var/shaide-installer/model-cache/xet",
	} {
		if value, ok := envValue(env, key); !ok || value != want {
			t.Errorf("%s = %q, want %q", key, value, want)
		}
	}
}
