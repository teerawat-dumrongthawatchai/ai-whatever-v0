package task

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

type State string

const (
	StateIntake   State = "INTAKE"
	StateExecute  State = "EXECUTE"
	StateVerify   State = "VERIFY"
	StateComplete State = "COMPLETE"
	StateFailed   State = "FAILED"
)

type Task struct {
	ID        string
	Text      string
	State     State
	CreatedAt time.Time
}

func New(text string) *Task {
	return &Task{
		ID:        newID(),
		Text:      text,
		State:     StateIntake,
		CreatedAt: time.Now(),
	}
}

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
