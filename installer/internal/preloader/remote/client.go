package remote

import (
	"context"
	"os"
	"strings"

	"github.com/axem-solutions/ai_platform/installer/internal/progress"
)

type RemoteClient interface {
	Run(ctx context.Context, command Command) (stdout string, stderr string, err error)
	Upload(ctx context.Context, localPath string, remotePath string, mode os.FileMode, tracker *progress.Tracker) error
	Close() error
}

type Command struct {
	Program string
	Args    []string
}

func (c Command) String() string {
	parts := make([]string, 0, 1+len(c.Args))
	parts = append(parts, c.Program)
	parts = append(parts, c.Args...)

	quoted := make([]string, 0, len(parts))
	for _, part := range parts {
		quoted = append(quoted, shellQuote(part))
	}

	return strings.Join(quoted, " ")
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
