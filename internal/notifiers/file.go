package notifiers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type FileNotifier struct {
	Path string
	mu   sync.Mutex
}

func (f *FileNotifier) Send(ctx context.Context, message string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	path := strings.TrimSpace(f.Path)
	if path == "" {
		return fmt.Errorf("file notifier path is empty")
	}

	dir := filepath.Dir(path)
	if dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create log directory: %w", err)
		}
	}

	line := fmt.Sprintf("[%s] %s\n", time.Now().Format(time.RFC3339), message)

	f.mu.Lock()
	defer f.mu.Unlock()

	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}
	defer file.Close()

	if _, err := file.WriteString(line); err != nil {
		return fmt.Errorf("write log file: %w", err)
	}

	return nil
}
