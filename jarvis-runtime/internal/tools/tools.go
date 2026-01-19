package tools

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"jarvis-runtime/internal/ledger"
)

type Workspace struct {
	ID           string
	RepoRoot     string
	AllowedRoot  string
}

func OpenWorkspace(path string) (*Workspace, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	// Phase 0: allowed root == repo root (strict).
	ws := &Workspace{
		ID:          ledger.HashString(abs)[:12],
		RepoRoot:    abs,
		AllowedRoot: abs,
	}
	return ws, nil
}

func ensureInScope(ws *Workspace, p string) error {
	abs, err := filepath.Abs(p)
	if err != nil {
		return err
	}
	// Must be within AllowedRoot.
	if !strings.HasPrefix(abs, ws.AllowedRoot+string(os.PathSeparator)) && abs != ws.AllowedRoot {
		return fmt.Errorf("path out of scope: %s", abs)
	}
	return nil
}

type Result struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

func runCmd(ctx context.Context, cwd string, name string, args ...string) (Result, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = cwd

	var outb, errb bytes.Buffer
	cmd.Stdout = &outb
	cmd.Stderr = &errb

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		// Determine exit code
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			exitCode = ee.ExitCode()
		} else {
			exitCode = 127
		}
	}
	return Result{Stdout: outb.Bytes(), Stderr: errb.Bytes(), ExitCode: exitCode}, err
}

func GitStatus(ws *Workspace) (Result, error) {
	if err := ensureInScope(ws, ws.RepoRoot); err != nil {
		return Result{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return runCmd(ctx, ws.RepoRoot, "git", "status", "--porcelain")
}

func GitDiff(ws *Workspace) (Result, error) {
	if err := ensureInScope(ws, ws.RepoRoot); err != nil {
		return Result{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return runCmd(ctx, ws.RepoRoot, "git", "diff")
}

func GitApplyPatch(ws *Workspace, patch string) (Result, error) {
	if err := ensureInScope(ws, ws.RepoRoot); err != nil {
		return Result{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Apply via stdin to `git apply -`
	cmd := exec.CommandContext(ctx, "git", "apply", "-")
	cmd.Dir = ws.RepoRoot
	cmd.Stdin = strings.NewReader(patch)

	var outb, errb bytes.Buffer
	cmd.Stdout = &outb
	cmd.Stderr = &errb

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			exitCode = ee.ExitCode()
		} else {
			exitCode = 127
		}
	}
	return Result{Stdout: outb.Bytes(), Stderr: errb.Bytes(), ExitCode: exitCode}, err
}

// Script-only test runner (S1). No arbitrary shell in Phase 0.
func RunTestsScriptOnly(ws *Workspace, scriptPath string) (Result, error) {
	if err := ensureInScope(ws, ws.RepoRoot); err != nil {
		return Result{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	return runCmd(ctx, ws.RepoRoot, scriptPath, ws.RepoRoot)
}
