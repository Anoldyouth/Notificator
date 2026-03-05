package loggers

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type FileLogger struct {
	Path string
	mu   sync.Mutex
}

func (l *FileLogger) Info(message string) {
	l.write("INFO", message)
}

func (l *FileLogger) Warning(message string) {
	l.write("WARNING", message)
}

func (l *FileLogger) Error(message string) {
	l.write("ERROR", message)
}

func (l *FileLogger) write(level, message string) {
	path := strings.TrimSpace(l.Path)
	if path == "" {
		path = "application.log"
	}

	dir := filepath.Dir(path)
	if dir != "." {
		_ = os.MkdirAll(dir, 0o755)
	}

	line := fmt.Sprintf("[%s] [%s] %s\n", time.Now().Format(time.RFC3339), level, message)

	l.mu.Lock()
	defer l.mu.Unlock()

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(line)
}
