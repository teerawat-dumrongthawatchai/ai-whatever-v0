# Jarvis Runtime - Phase 0

Complete Phase 0 package for jarvis-runtime system.

## Structure

- `jarvis-runtime/` - The runtime spine with orchestrator, ledger, tools, and policy
- `car-rental-api/` - Target workspace (will be bootstrapped by runtime)

## Quick Start

### 1. Build the runtime

```bash
cd jarvis-runtime
go build -o jarvis ./cmd/jarvis
```

### 2. Run Phase 0 (PASS case)

```bash
./jarvis --task "bootstrap workspace + run tests" --workspace ../car-rental-api
```

Expected:
- Prints `OK: task completed successfully`
- Creates/updates ledger: `jarvis-runtime/.jarvis/ledger.jsonl`
- Creates/updates `car-rental-api` minimal Fiber app + test
- Runs `go test ./...` successfully (proof for Friday)

### 3. Inspect ledger

```bash
tail -n 8 .jarvis/ledger.jsonl
```

You should see events including:
- STATE INTAKE/EXECUTE/VERIFY/COMPLETE
- TOOL_CALL + TOOL_RESULT for tests
- VERIFY event by Friday with PASS

### 4. Prove idempotency (run it again)

```bash
./jarvis --task "bootstrap workspace + run tests" --workspace ../car-rental-api
```

Expected:
- Still succeeds
- README marker remains
- Ledger continues appending events (new task_id)

### 5. Prove Friday can block (FAIL case)

Break the test intentionally:

```bash
sed -i '' 's/expected 200/expected 201/' ../car-rental-api/cmd/server/main_test.go
```

Now run:

```bash
./jarvis --task "bootstrap workspace + run tests" --workspace ../car-rental-api
```

Expected:
- Command fails with: `Friday blocked completion: tests failed`
- Ledger includes Friday event `BLOCK: tests did not pass`

Revert the test:

```bash
git -C ../car-rental-api checkout -- cmd/server/main_test.go
```

## What Phase 0 Proves

You now have a runnable system that:
- Executes a task lifecycle
- Uses controlled tools
- Captures evidence (hashes + exit codes)
- Enforces a gate (Friday) based on evidence, not claims
- Is repeatable and debuggable

That is the "trust spine" your whole system depends on.
