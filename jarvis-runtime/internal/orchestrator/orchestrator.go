package orchestrator

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"jarvis-runtime/internal/ledger"
	"jarvis-runtime/internal/policy"
	"jarvis-runtime/internal/task"
	"jarvis-runtime/internal/tools"
)

func Run(taskText string, workspacePath string) error {
	t := task.New(taskText)

	led, err := ledger.New(filepath.Join(".jarvis", "ledger.jsonl"))
	if err != nil {
		return err
	}

	ws, err := tools.OpenWorkspace(workspacePath)
	if err != nil {
		return err
	}

	// STATE: INTAKE
	if err := led.Append(ledger.Event{
		TaskID:        t.ID,
		WorkspaceID:   ws.ID,
		Actor:         "Jarvis",
		EventType:     "STATE",
		Message:       string(task.StateIntake),
		PolicyVersion: policy.Version,
	}); err != nil {
		return err
	}

	// Ensure workspace is a git repo. If not, bootstrap minimal repo.
	if err := bootstrapWorkspaceIfNeeded(ws, led, t.ID); err != nil {
		_ = led.Append(ledger.Event{TaskID: t.ID, WorkspaceID: ws.ID, Actor: "Jarvis", EventType: "STATE", Message: string(task.StateFailed), PolicyVersion: policy.Version})
		return err
	}

	// STATE: EXECUTE
	if err := led.Append(ledger.Event{
		TaskID:        t.ID,
		WorkspaceID:   ws.ID,
		Actor:         "Jarvis",
		EventType:     "STATE",
		Message:       string(task.StateExecute),
		PolicyVersion: policy.Version,
	}); err != nil {
		return err
	}

	// Apply a trivial patch: add README.md line (idempotent-ish: will fail if already applied; we handle).
	patch := buildReadmePatch()
	if err := logToolCall(led, t.ID, ws.ID, "git.apply_patch", patch); err != nil {
		return err
	}
	applyRes, applyErr := tools.GitApplyPatch(ws, patch)
	if err := logToolResult(led, t.ID, ws.ID, "git.apply_patch", patch, applyRes); err != nil {
		return err
	}
	if applyErr != nil {
		// If patch already applied, git apply will fail. We tolerate if README already contains marker.
		if !readmeAlreadyBootstrapped(ws.RepoRoot) {
			return fmt.Errorf("patch apply failed: %s", strings.TrimSpace(string(applyRes.Stderr)))
		}
	}

	// Record diff hash after patch attempt
	diffRes, _ := tools.GitDiff(ws)
	if err := led.Append(ledger.Event{
		TaskID:        t.ID,
		WorkspaceID:   ws.ID,
		Actor:         "Jarvis",
		EventType:     "CLAIM",
		Message:       "Applied patch (README bootstrap marker present)",
		DiffHash:      ledger.HashBytes(diffRes.Stdout),
		PolicyVersion: policy.Version,
	}); err != nil {
		return err
	}

	// STATE: VERIFY
	if err := led.Append(ledger.Event{
		TaskID:        t.ID,
		WorkspaceID:   ws.ID,
		Actor:         "Jarvis",
		EventType:     "STATE",
		Message:       string(task.StateVerify),
		PolicyVersion: policy.Version,
	}); err != nil {
		return err
	}

	// Run tests via script-only tool
	script := filepath.Join("scripts", "go_test.sh")
	if err := logToolCall(led, t.ID, ws.ID, "test.run_script_only", script); err != nil {
		return err
	}
	testRes, testErr := tools.RunTestsScriptOnly(ws, script)
	if err := logToolResult(led, t.ID, ws.ID, "test.run_script_only", script, testRes); err != nil {
		return err
	}
	if testErr != nil || testRes.ExitCode != 0 {
		_ = led.Append(ledger.Event{
			TaskID:        t.ID,
			WorkspaceID:   ws.ID,
			Actor:         "Friday",
			EventType:     "VERIFY",
			Message:       fmt.Sprintf("BLOCK: tests did not pass (exit_code=%d)", testRes.ExitCode),
			StdoutHash:    ledger.HashBytes(testRes.Stdout),
			StderrHash:    ledger.HashBytes(testRes.Stderr),
			ExitCode:      &testRes.ExitCode,
			PolicyVersion: policy.Version,
		})
		_ = led.Append(ledger.Event{TaskID: t.ID, WorkspaceID: ws.ID, Actor: "Jarvis", EventType: "STATE", Message: string(task.StateFailed), PolicyVersion: policy.Version})
		return fmt.Errorf("Friday blocked completion: tests failed (exit_code=%d)", testRes.ExitCode)
	}

	// Friday gate passes
	if err := led.Append(ledger.Event{
		TaskID:        t.ID,
		WorkspaceID:   ws.ID,
		Actor:         "Friday",
		EventType:     "VERIFY",
		Message:       "PASS: tests ran and passed; completion allowed",
		StdoutHash:    ledger.HashBytes(testRes.Stdout),
		StderrHash:    ledger.HashBytes(testRes.Stderr),
		ExitCode:      &testRes.ExitCode,
		PolicyVersion: policy.Version,
	}); err != nil {
		return err
	}

	// STATE: COMPLETE
	if err := led.Append(ledger.Event{
		TaskID:        t.ID,
		WorkspaceID:   ws.ID,
		Actor:         "Jarvis",
		EventType:     "STATE",
		Message:       string(task.StateComplete),
		PolicyVersion: policy.Version,
	}); err != nil {
		return err
	}

	return nil
}

func logToolCall(led *ledger.Ledger, taskID, workspaceID, toolName, args string) error {
	return led.Append(ledger.Event{
		TaskID:        taskID,
		WorkspaceID:   workspaceID,
		Actor:         "Jarvis",
		EventType:     "TOOL_CALL",
		ToolName:      toolName,
		ArgsHash:      ledger.HashString(args),
		PolicyVersion: policy.Version,
	})
}

func logToolResult(led *ledger.Ledger, taskID, workspaceID, toolName, args string, res tools.Result) error {
	exit := res.ExitCode
	return led.Append(ledger.Event{
		TaskID:        taskID,
		WorkspaceID:   workspaceID,
		Actor:         "Jarvis",
		EventType:     "TOOL_RESULT",
		ToolName:      toolName,
		ArgsHash:      ledger.HashString(args),
		StdoutHash:    ledger.HashBytes(res.Stdout),
		StderrHash:    ledger.HashBytes(res.Stderr),
		ExitCode:      &exit,
		PolicyVersion: policy.Version,
	})
}

func isGitRepo(path string) bool {
	_, err := os.Stat(filepath.Join(path, ".git"))
	return err == nil
}

func bootstrapWorkspaceIfNeeded(ws *tools.Workspace, led *ledger.Ledger, taskID string) error {
	if err := os.MkdirAll(ws.RepoRoot, 0o755); err != nil {
		return err
	}

	if !isGitRepo(ws.RepoRoot) {
		// init repo
		if err := logToolCall(led, taskID, ws.ID, "git.init", ws.RepoRoot); err != nil {
			return err
		}
		// run `git init` directly (still a tool action)
		// reuse runCmd pattern by invoking system git.
		// For Phase 0, keep it simple here:
		_, _ = os.Stat(ws.RepoRoot)
	}

	// If repo not initialized, do it now
	if !isGitRepo(ws.RepoRoot) {
		// `git init`
		// Using the tools runner indirectly would be nicer; Phase 0 keeps it minimal.
		// NOTE: We keep this limited and auditable via ledger event above.
		cmd := fmt.Sprintf("cd %q && git init", ws.RepoRoot)
		if err := runSystemShell(cmd); err != nil {
			return err
		}
	}

	// Ensure minimal Go project exists with tests
	if err := ensureMinimalGoFiberProject(ws.RepoRoot); err != nil {
		return err
	}

	return nil
}

func execSh(cmd string) error {
	// Minimal helper for bootstrap only. No arbitrary use beyond this file.
	// This is acceptable in Phase 0 because it is tightly scoped and not exposed to agents.
	return runSystemShell(cmd)
}

// runSystemShell is intentionally narrow (bootstrap only). In Phase 1 we will remove this and route through Tool Gateway tiers.
func runSystemShell(cmd string) error {
	// Using /bin/bash -lc to handle `cd`.
	// This is NOT exposed as a tool in Phase 0; it's internal bootstrap to avoid extra complexity.
	p := "/bin/bash"
	args := []string{"-lc", cmd}
	proc := exec.Command(p, args...)
	out, err := proc.CombinedOutput()
	if err != nil {
		return fmt.Errorf("bootstrap shell failed: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func buildReadmePatch() string {
	// Adds README.md if missing, and appends a marker line.
	// NOTE: This patch will fail if README exists with different content in a way that conflicts.
	// We tolerate already-applied state via readmeAlreadyBootstrapped().
	return `diff --git a/README.md b/README.md
new file mode 100644
index 0000000..d9b1c65
--- /dev/null
+++ b/README.md
@@ -0,0 +1 @@
+Bootstrapped by Jarvis Phase 0
`
}

func readmeAlreadyBootstrapped(repoRoot string) bool {
	b, err := os.ReadFile(filepath.Join(repoRoot, "README.md"))
	if err != nil {
		return false
	}
	return strings.Contains(string(b), "Bootstrapped by Jarvis Phase 0")
}

func ensureMinimalGoFiberProject(repoRoot string) error {
	// Create minimal project only if go.mod doesn't exist.
	if _, err := os.Stat(filepath.Join(repoRoot, "go.mod")); err == nil {
		return nil
	}

	// Create directories
	if err := os.MkdirAll(filepath.Join(repoRoot, "cmd", "server"), 0o755); err != nil {
		return err
	}

	// Write go.mod
	mod := `module car-rental-api

go 1.22
`
	if err := os.WriteFile(filepath.Join(repoRoot, "go.mod"), []byte(mod), 0o644); err != nil {
		return err
	}

	// main.go (Fiber health endpoint)
	main := `package main

import "github.com/gofiber/fiber/v2"

func main() {
	app := fiber.New()
	app.Get("/healthz", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})
	_ = app.Listen(":8088")
}
`
	if err := os.WriteFile(filepath.Join(repoRoot, "cmd", "server", "main.go"), []byte(main), 0o644); err != nil {
		return err
	}

	// test file
	test := `package main

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestHealthz(t *testing.T) {
	app := fiber.New()
	app.Get("/healthz", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/healthz", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}
`
	if err := os.WriteFile(filepath.Join(repoRoot, "cmd", "server", "main_test.go"), []byte(test), 0o644); err != nil {
		return err
	}

	// Initialize git add baseline files
	cmd := fmt.Sprintf("cd %q && git add -A && git commit -m %q || true", repoRoot, "bootstrap: minimal fiber healthz + test")
	if err := runSystemShell(cmd); err != nil {
		return err
	}

	// Fetch fiber dependency
	cmd = fmt.Sprintf("cd %q && go mod tidy", repoRoot)
	if err := runSystemShell(cmd); err != nil {
		return err
	}

	// Commit tidy results
	cmd = fmt.Sprintf("cd %q && git add -A && git commit -m %q || true", repoRoot, "bootstrap: go mod tidy")
	if err := runSystemShell(cmd); err != nil {
		return err
	}

	return nil
}
