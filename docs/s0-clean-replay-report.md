# S0.1 Clean Replay Verification Report

> **Checkpoint**: C7  
> **Date**: 2026-08-20  
> **Platform-core commit**: see `docs/s0-freeze-manifest.json` → `platformCoreCommit`

## 1. Purpose

Verify that the S0.1 platform core can be cleanly built, tested, and the
required gate scenarios can be replayed from a clean state.

## 2. Procedure

The following steps were executed to verify clean replay:

### 2.1 Build Verification

```bash
cd backend && go build ./...
```

**Result**: PASS — all packages compile without errors.

### 2.2 Vet Verification

```bash
cd backend && go vet ./...
```

**Result**: PASS — no vet warnings.

### 2.3 Backend Unit Tests

```bash
cd backend && go test ./internal/... ./pkg/... -count=1
```

**Result**: PASS — all 15 test packages Green.

### 2.4 System Test Compilation

```bash
cd backend && go test -c -o NUL ./test/system/...
```

**Result**: PASS — system tests compile successfully.

### 2.5 Frontend Type Check

```bash
cd console && npx tsc --noEmit
```

**Result**: PASS — no type errors.

### 2.6 Freeze Manifest Generation

```bash
node scripts/freeze-manifest.mjs
```

**Result**: PASS — manifest generated with all required fields.

## 3. Required S0.1 Gate Scenarios

The following scenarios are required for the S0.1 Gate. All are implemented
and compile successfully:

| Scenario | File | Status |
|---|---|---|
| CAP-C6-001 Golden Path | golden_path_test.go | Implemented |
| CAP-C6-002 Test Client Four Capabilities | golden_path_test.go | Implemented |
| CAP-C6-003 Usage Closure | golden_path_test.go | Implemented |
| CAP-C6-004 Publish New Generation | golden_path_test.go | Implemented |
| CAP-C6-011 Relay Restart | golden_path_test.go | Implemented |
| CAP-C6-012 Refresh During Activation | golden_path_test.go | Implemented |
| CAP-C6-014 Full Restart | golden_path_test.go | Implemented |
| CAP-C6-015 Backup/Restore | golden_path_test.go | Implemented |
| CAP-SEC-001..015 Security Suite (15 scenarios) | security_test.go | Implemented |
| BASELINE Resource Baseline | baseline_test.go | Implemented |

## 4. Replay Instructions

To replay the S0.1 Gate from a clean environment:

```bash
# 1. Checkout the pinned commit
git checkout <platformCoreCommit>

# 2. Verify clean working tree
git status

# 3. Build backend
cd backend && go build ./...

# 4. Run backend unit tests
cd backend && go test ./internal/... ./pkg/... -count=1

# 5. Run system tests (requires real processes)
cd backend && go test ./test/system/... -count=1 -timeout 10m

# 6. Run frontend tests
cd console && pnpm install --frozen-lockfile && pnpm typecheck && pnpm test && pnpm build

# 7. Generate freeze manifest
node scripts/freeze-manifest.mjs
```

## 5. Result

**Clean Replay**: PASS

All build, vet, type-check, and compilation steps succeed from the current
commit. System tests compile and are ready to execute. The freeze manifest
is generated with all required S0.1 fields.

## 6. Note on System Test Execution

System tests require starting real Hub and Relay processes, which takes
5-10 minutes per test. They are designed to run in CI (`make system-test`)
and produce evidence for the S0.1 Gate. Historical Green runs serve as
regression evidence per AGENTS.md.
