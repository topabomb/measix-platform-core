# Test-Driven Development (TDD)

This repository uses TDD for new behavior and regression fixes. Architecture defines the required behavior; executable tests in this repository prove it.

## 1. The cycle

```text
Requirement
  ↓
Red      — write/enable the smallest executable test that fails for the intended reason
  ↓
Green    — implement the minimum behavior that satisfies the test
  ↓
Refactor — improve design while all relevant tests remain Green
  ↓
Gate     — run the complete affected CI/component/system layer
```

TDD is not “write tests eventually.” The failure must be observed before the implementation is considered proven by that test.

## 2. What counts as valid Red

A useful Red test:

- describes a real architecture requirement, regression or implementation invariant;
- fails before the production change;
- fails for the expected behavioral reason;
- is deterministic;
- remains in the final codebase.

Prefer an assertion failure that demonstrates missing/wrong behavior. A compile failure can be a transient Red signal when introducing a new interface, but it is weaker evidence if it does not demonstrate the behavior being built.

Do not create meaningless tests solely to manufacture a Red commit.

## 3. Feature TDD

For a new capability already authorized by architecture:

1. identify the architecture requirement and applicable critical scenario ID;
2. choose the lowest test layer that can prove the next behavior slice;
3. add the failing test/fixture;
4. observe Red;
5. implement the smallest slice;
6. observe Green;
7. refactor;
8. add higher-layer integration/system proof only when the slice crosses real boundaries.

Do not start with a huge T4 test when a T1/T2 test can drive the internal behavior. Build outward.

## 4. Regression TDD

For a bug:

```text
reproduce with a test
→ confirm Red on the buggy implementation
→ fix
→ confirm Green
→ run affected regression/component/system gate
```

If the bug exposes a missing architecture scenario, update the relevant Testing Spec and reference its stable ID. The local test does not become a new semantic authority by itself.

## 5. Cross-component TDD

Cross-component work should use two loops:

### Inner loop

Drive component behavior with T1/T2 tests using controlled peers/fakes.

### Contract/system loop

Then prove the real boundary with T3/T4:

```text
contract/fixture Red
→ component implementation Green
→ real Hub/Relay/Admin integration
→ mapped SYS scenario Green
```

Do not require the full system harness for every tiny domain edit, and do not stop at mocks when architecture requires real-process integration.

## 6. GitHub-only TDD

A fully GitHub-based Red/Green loop is valid when GitHub Actions is the execution environment.

### Required sequence

1. Create a feature/fix branch.
2. Open a **Draft PR before the Red commit is evaluated** so `pull_request` workflows run and evidence is attached to the PR.
3. Add the failing test in a Red commit.
4. Let GitHub Actions execute the relevant job.
5. Inspect the failed check/job log and verify that the intended test failed for the expected reason.
6. Record the Red commit SHA/check in the PR description when the change is non-trivial.
7. Add the minimum production implementation in a later commit.
8. Let Actions run on the new head SHA.
9. Verify the affected checks are Green on the latest commit.
10. Refactor in additional commits as needed; Actions must remain Green.
11. Merge only after the complete required aggregate gate passes.

### Why this is verifiable

GitHub Actions creates check runs for workflow jobs. Protected branches can require those checks to pass before merge, and required checks apply to the current PR head rather than an older successful commit. Test reports/logs can be retained as workflow artifacts.

Therefore a developer/agent without a local runtime can still produce auditable evidence:

```text
PR
├── Red commit SHA → failing Actions check/log
├── Green commit SHA → successful Actions check
└── final SHA → required ci-gate success + artifacts
```

### Remote-only agent rule

An agent that can edit GitHub but cannot execute locally must never say “tests pass” based only on reading code. It must inspect the GitHub Actions run/check for the relevant commit. If the repository lacks CI for the required test, the result is **not verified** and the missing CI capability should be implemented or reported.

## 7. Local TDD

With a local checkout:

1. add test;
2. run the narrow test and observe Red;
3. implement;
4. rerun and observe Green;
5. refactor;
6. run affected component suite;
7. push and let GitHub CI independently reproduce the result.

Local execution optimizes feedback time; GitHub CI provides merge evidence. Neither replaces the other when both are available.

## 8. PR evidence format

For non-trivial behavior changes, include:

```text
Architecture: <document/section/scenario ID>
Red: <commit SHA> / <test name> / expected failure
Green: <commit SHA or latest> / <checks passed>
Additional gates: <T0/T1/T2/T3/T4 lanes>
```

A screenshot is optional and weaker than a check/run link + commit SHA.

## 9. Refactoring

Refactoring begins from Green. It should not require changing expected behavior. If a refactor forces behavior expectations to change, it is no longer a pure refactor and must be reclassified.

Use characterization tests first when refactoring poorly understood legacy behavior.

## 10. Exceptions

Do not force artificial Red/Green commits for:

- documentation-only edits;
- formatting-only changes;
- deterministic regeneration where source semantics did not change;
- dependency lockfile refresh with no intended behavior change.

These changes still run applicable static/build/contract gates.

Migration/schema changes are not exempt: use repository/migration tests that fail before the schema/migration behavior exists.

## 11. Anti-patterns

Forbidden TDD shortcuts:

- writing implementation and test together, then claiming an unobserved theoretical Red;
- disabling the test during implementation;
- adding retries to hide product failures;
- asserting internal implementation details when the requirement is observable behavior;
- using only mocks for a required real-boundary scenario;
- deleting a regression test once the fix is Green;
- changing architecture semantics inside the test to make implementation convenient.

## 12. Merge policy target

After I0 CI is live, configure `main` so changes merge through PRs and the stable aggregate CI check is required. The required check should run for every PR rather than being suppressed by workflow-level path filters.

This turns TDD from a convention into an enforceable development loop: Red may exist on the branch, but Green is required before merge.
