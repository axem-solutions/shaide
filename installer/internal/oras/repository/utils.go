package repository

import (
	"fmt"
	"strconv"
	"strings"
)

func nextChunk(location string, offset, totalSize, chunkSize int64) chunk {
	size := min(chunkSize, totalSize-offset)
	return chunk{
		Location: location,
		Offset:   offset,
		End:      offset + size - 1,
		Size:     size,
	}
}

func nextOffsetFromRange(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, fmt.Errorf("missing Range header")
	}

	value = strings.TrimPrefix(value, "bytes=")

	parts := strings.Split(value, "-")
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid Range header %q", value)
	}

	end, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid Range end %q: %w", parts[1], err)
	}

	return end + 1, nil
}

func validateNextOffset(nextOffset, start, totalSize int64) error {
	if nextOffset <= start || nextOffset > totalSize {
		return fmt.Errorf("registry returned invalid upload offset %d for range starting at %d of %d", nextOffset, start, totalSize)
	}

	return nil
}
