# S0.1 Clean Replay Verification Report

> **Checkpoint**: C7  
> **Status**: NOT GREEN — Clean replay has NOT been completed as S0.1 Gate evidence.  
> **Date**: 2026-08-20  
> **Platform-core commit**: see `docs/s0-freeze-manifest.json` → `platformCoreCommit` (if generated)

## 1. Purpose

Verify that the S0.1 platform core can be cleanly built, tested, and the
required gate scenarios can be replayed from a clean state.

## 2. Current Status

The following steps have been verified to compile/pass at the unit/component
level, but **the full required S0.1 system gate has NOT been executed on the
exact candidate SHA as Green evidence**.

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

**Result**: PASS — all internal/pkg test packages Green.

### 2.4 System Test Compilation

```bash
cd backend && go test -c -o NUL ./test/system/...
```

**Result**: PASS — system tests compile successfully.

### 2.5 Frontend Type Check

```bash
cd console && pnpm typecheck
```

**Result**: PASS — no type errors.

### 2.6 Frontend Production Build

```bash
cd console && pnpm build
```

**Result**: PASS — SPA compiled successfully.

## 3. Required S0.1 Gate Scenarios — NOT YET GREEN

The following scenarios are required for the S0.1 Gate. All are implemented
and compile successfully, but **have not been executed to completion as a
clean-environment gate on the exact candidate SHA**:

| Scenario | File | Compile | Execution |
|---|---|---|---|
| CAP-C6-001 Golden Path | golden_path_test.go | PASS | NOT EXECUTED as Gate |
| CAP-C6-002 Test Client Four Capabilities | golden_path_test.go | PASS | NOT EXECUTED as Gate |
| CAP-C6-003 Usage Closure | golden_path_test.go | PASS | NOT EXECUTED as Gate |
| CAP-C6-004 Publish New Generation | golden_path_test.go | PASS | NOT EXECUTED as Gate |
| CAP-C6-010 Hub Crash Around Publish | recovery_test.go | PASS | NOT EXECUTED as Gate |
| CAP-C6-011 Relay Restart | golden_path_test.go | PASS | NOT EXECUTED as Gate |
| CAP-C6-012 Refresh During Activation | golden_path_test.go | PASS | NOT EXECUTED as Gate |
| CAP-C6-013 SQLite Busy/Transient | recovery_test.go | PASS | NOT EXECUTED as Gate |
| CAP-C6-014 Full Restart | golden_path_test.go | PASS | NOT EXECUTED as Gate |
| CAP-C6-015 Backup/Restore | golden_path_test.go | PASS | NOT EXECUTED as Gate |
| CAP-SEC-001..015 Security Suite (15 scenarios) | security_test.go | PASS | NOT EXECUTED as Gate |
| BASELINE Resource Baseline | baseline_test.go | PASS | NOT EXECUTED as Gate |
| CAP-C6-001 Browser Golden Path | console/e2e/ | PASS | NOT EXECUTED as Gate |

## 4. Replay Instructions (To Be Executed)

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
cd backend && go test ./test/system/... -count=1 -timeout 15m

# 6. Run frontend tests
cd console && pnpm install --frozen-lockfile && pnpm typecheck && pnpm test && pnpm build

# 7. Run browser E2E (requires real Hub/Relay/Adapter)
make console-e2e

# 8. Generate freeze manifest (only after all above Green)
node scripts/freeze-manifest.mjs
```

## 5. Result

**Clean Replay: NOT GREEN**

All build, vet, type-check, and compilation steps succeed from the current
commit. However, the full required S0.1 system scenarios have NOT been
executed to completion as a clean-environment gate. The freeze manifest has
NOT been validly generated. This report does NOT constitute S0.1 Gate evidence.

## 6. Required Actions Before Green

1. Execute full system test suite on clean candidate SHA
2. Execute Browser T4.1 Golden Path on clean candidate SHA
3. Execute Real Adapter qualification in a secure environment
4. Verify all CAP scenario results are PASS
5. Generate freeze manifest with real scenario results
6. Only then can Clean Replay be marked PASS
