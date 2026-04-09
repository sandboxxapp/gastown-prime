// Package audit provides MCP call logging.
package audit

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// Logger records MCP tool calls for auditing.
type Logger interface {
	Log(entry Entry)
	Close()
}

// Entry is one audit log line.
type Entry struct {
	Timestamp      string `json:"ts"`
	AgentID        string `json:"agent_id,omitempty"`
	Upstream       string `json:"upstream"`
	Tool           string `json:"tool"`
	Classification string `json:"classification"`
	PolicyMode     string `json:"policy_mode"`
	Allowed        bool   `json:"allowed"`
	DurationMs     int64  `json:"duration_ms"`
	ArgsHash       string `json:"args_hash,omitempty"`
	ResultStatus   string `json:"result_status"`
	Error          string `json:"error,omitempty"`
}

// FileLogger writes audit entries to a JSONL file.
type FileLogger struct {
	file *os.File
	mu   sync.Mutex
}

// NewFileLogger opens an audit log file. Empty path returns a no-op logger.
func NewFileLogger(path string) (Logger, error) {
	if path == "" {
		return &nopLogger{}, nil
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("open audit log: %w", err)
	}
	return &FileLogger{file: f}, nil
}

func (l *FileLogger) Log(entry Entry) {
	if entry.Timestamp == "" {
		entry.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	data, _ := json.Marshal(entry)
	l.file.Write(append(data, '\n'))
}

func (l *FileLogger) Close() { l.file.Close() }

type nopLogger struct{}

func (n *nopLogger) Log(Entry) {}
func (n *nopLogger) Close()    {}

// HashArgs creates a SHA-256 hash of arguments (never log raw args).
func HashArgs(args json.RawMessage) string {
	if args == nil {
		return ""
	}
	h := sha256.Sum256(args)
	return fmt.Sprintf("sha256:%x", h[:8])
}
