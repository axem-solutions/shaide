package storage

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

type Stats struct {
	Capacity  int64
	Used      int64
	Available int64
}

type Requirement struct {
	Phase    string
	Target   string
	Expected int64
	Reusable int64
}

type Checker func(Requirement) error

func NewChecker(
	path string,
	logf func(string, ...any),
) Checker {
	return func(requirement Requirement) error {
		required := MissingBytes(
			requirement.Expected,
			requirement.Reusable,
		)

		if required == 0 {
			return nil
		}

		stats, err := GetStats(path)
		if err != nil {
			return fmt.Errorf("check storage before %s for %s: %w", requirement.Phase, requirement.Target, err)
		}

		logf(
			"storage check: phase=%s target=%s capacity=%s used=%s available=%s expected=%s reusable=%s required=%s",
			requirement.Phase,
			requirement.Target,
			FormatBytes(stats.Capacity),
			FormatBytes(stats.Used),
			FormatBytes(stats.Available),
			FormatBytes(requirement.Expected),
			FormatBytes(requirement.Reusable),
			FormatBytes(required),
		)

		if stats.Available < required {
			return fmt.Errorf(
				"insufficient storage before %s for %s: available=%s required=%s",
				requirement.Phase,
				requirement.Target,
				FormatBytes(stats.Available),
				FormatBytes(required),
			)
		}

		return nil
	}
}

func FormatBytes(bytes int64) string {
	const unit = 1000

	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}

	value := float64(bytes)
	for _, suffix := range []string{"kB", "MB", "GB", "TB", "PB"} {
		value /= unit
		if value < unit {
			return fmt.Sprintf("%.2f %s", value, suffix)
		}
	}

	return fmt.Sprintf("%.2f EB", value/unit)
}

func GetStats(path string) (Stats, error) {
	cleanPath, err := cleanAbsPath(path)
	if err != nil {
		return Stats{}, err
	}

	var stat unix.Statfs_t
	if err := unix.Statfs(cleanPath, &stat); err != nil {
		return Stats{}, fmt.Errorf("statfs %s: %w", cleanPath, err)
	}

	blockSize := int64(stat.Bsize)

	capacity := int64(stat.Blocks) * blockSize
	free := int64(stat.Bfree) * blockSize
	available := int64(stat.Bavail) * blockSize

	return Stats{
		Capacity:  capacity,
		Used:      capacity - free,
		Available: available,
	}, nil
}

func MissingBytes(expected, reusable int64) int64 {
	if expected <= 0 {
		return 0
	}

	if reusable <= 0 {
		return expected
	}

	if reusable >= expected {
		return 0
	}

	return expected - reusable
}

func EnsureDirs(paths []string) error {
	for _, path := range paths {
		cleanPath, err := cleanAbsPath(path)
		if err != nil {
			return err
		}

		if err := os.MkdirAll(cleanPath, 0o755); err != nil {
			return fmt.Errorf("create storage dir %s: %w", cleanPath, err)
		}
	}

	return nil
}

// UseTempDir redirects temporary files created by this process to path.
func UseTempDir(path string) error {
	cleanPath, err := cleanAbsPath(path)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(cleanPath, 0o755); err != nil {
		return fmt.Errorf("create temp dir %s: %w", cleanPath, err)
	}

	if err := os.Setenv("TMPDIR", cleanPath); err != nil {
		return fmt.Errorf("set TMPDIR to %s: %w", cleanPath, err)
	}

	return nil
}

func cleanAbsPath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("path is required")
	}

	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("path must be absolute: %q", path)
	}

	return filepath.Clean(path), nil
}

func IsMountPoint(path string) (bool, error) {
	cleanPath, err := cleanAbsPath(path)
	if err != nil {
		return false, err
	}

	file, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return false, fmt.Errorf("open /proc/self/mountinfo: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		mountPoint, err := mountPointFromMountinfoLine(scanner.Text())
		if err != nil {
			return false, err
		}

		if mountPoint == cleanPath {
			return true, nil
		}
	}

	if err := scanner.Err(); err != nil {
		return false, fmt.Errorf("read /proc/self/mountinfo: %w", err)
	}

	return false, nil
}

func mountPointFromMountinfoLine(line string) (string, error) {
	fields := strings.Fields(line)

	// mountinfo format:
	// 1 mount ID
	// 2 parent ID
	// 3 major:minor
	// 4 root - mount point
	// 5 mount point
	if len(fields) < 5 {
		return "", fmt.Errorf("invalid mountinfo line: %q", line)
	}

	mountPoint, err := decodeMountinfoPath(fields[4])
	if err != nil {
		return "", fmt.Errorf("decode mount point %q: %w", fields[4], err)
	}

	return filepath.Clean(mountPoint), nil
}

func decodeMountinfoPath(path string) (string, error) {
	var b strings.Builder

	for i := 0; i < len(path); i++ {
		if path[i] != '\\' {
			b.WriteByte(path[i])
			continue
		}

		if i+3 >= len(path) {
			return "", fmt.Errorf("invalid mountinfo escape in %q", path)
		}

		octal := path[i+1 : i+4]

		value, err := strconv.ParseInt(octal, 8, 8)
		if err != nil {
			return "", fmt.Errorf("invalid mountinfo escape \\%s: %w", octal, err)
		}

		b.WriteByte(byte(value))
		i += 3
	}

	return b.String(), nil
}
