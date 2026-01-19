package ledger

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Event struct {
	Timestamp    string `json:"timestamp"`
	TaskID       string `json:"task_id"`
	WorkspaceID  string `json:"workspace_id"`
	Actor        string `json:"actor"`      // "Jarvis"
	EventType    string `json:"event_type"` // STATE|TOOL_CALL|TOOL_RESULT|CLAIM|VERIFY
	Message      string `json:"message,omitempty"`

	ToolName     string `json:"tool_name,omitempty"`
	ArgsHash     string `json:"args_hash,omitempty"`
	StdoutHash   string `json:"stdout_hash,omitempty"`
	StderrHash   string `json:"stderr_hash,omitempty"`
	ExitCode     *int   `json:"exit_code,omitempty"`
	DiffHash     string `json:"diff_hash,omitempty"`

	PolicyVersion string `json:"policy_version,omitempty"`

	PrevEventHash string `json:"prev_event_hash,omitempty"`
	EventHash     string `json:"event_hash"`
}

type Ledger struct {
	path         string
	prevEventHash string
}

func New(path string) (*Ledger, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	l := &Ledger{path: path}
	// If file exists, load last hash (best-effort).
	_ = l.loadPrevHash()
	return l, nil
}

func (l *Ledger) Append(e Event) error {
	e.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	e.PrevEventHash = l.prevEventHash

	// Compute event_hash as sha256 of JSON without event_hash.
	tmp := e
	tmp.EventHash = ""
	b, err := json.Marshal(tmp)
	if err != nil {
		return err
	}
	h := sha256.Sum256(b)
	e.EventHash = hex.EncodeToString(h[:])

	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	line, err := json.Marshal(e)
	if err != nil {
		return err
	}
	if _, err := w.WriteString(string(line) + "\n"); err != nil {
		return err
	}
	if err := w.Flush(); err != nil {
		return err
	}

	l.prevEventHash = e.EventHash
	return nil
}

func HashBytes(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func HashString(s string) string {
	return HashBytes([]byte(s))
}

func (l *Ledger) loadPrevHash() error {
	f, err := os.Open(l.path)
	if err != nil {
		return nil
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	var lastLine []byte
	for sc.Scan() {
		lastLine = append([]byte(nil), sc.Bytes()...)
	}
	if err := sc.Err(); err != nil {
		return err
	}
	if len(lastLine) == 0 {
		return nil
	}
	var e Event
	if err := json.Unmarshal(lastLine, &e); err != nil {
		return fmt.Errorf("ledger last line parse failed: %w", err)
	}
	l.prevEventHash = e.EventHash
	return nil
}
