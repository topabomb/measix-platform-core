# Contributing

This repository uses architecture-first, test-driven development. The authoritative architecture is `topabomb/measix-architecture`; implementation and executable evidence live here.

## 1. Before changing code

Classify the change:

1. **Architecture semantic change** — changes product meaning, component ownership, stable IDs, wire/error/state semantics, security invariants or required S0 scenarios. Update `measix-architecture` first.
2. **Executable-contract change** — changes OpenAPI/fixtures/codegen while preserving existing architecture meaning. Follow `docs/api-contracts.md`.
3. **Implementation change** — source/refactor/performance/operational work that preserves semantics. Work directly here.
4. **Regression fix** — add a failing regression test first; escalate to architecture only if the regression exposes a missing/incorrect requirement.

## 2. Branch and PR workflow

After CI exists, `main` is treated as merge-only. Development uses short-lived branches and pull requests. Suggested prefixes are `feat/`, `fix/`, `test/`, `refactor/`, `docs/` and `chore/`; branch naming is organizational, not architectural.

Open a Draft PR early for behavior changes. This is required for GitHub-only TDD because the PR gives GitHub Actions a stable place to execute and retain Red/Green check evidence.

Every PR states:

- architecture impact;
- relevant architecture document/section or critical scenario IDs;
- Red evidence for behavior changes;
- Green evidence and executed test layers;
- OpenAPI/generated-code impact;
- migration impact;
- operational/release impact.

Use `.github/pull_request_template.md`.

## 3. TDD requirement

For new behavior and bug fixes:

```text
Red: write/enable the smallest executable test that fails for the intended reason
Green: implement the minimum behavior that satisfies it
Refactor: improve structure without weakening the test
```

The Red test should preferably fail on a behavioral assertion rather than because of an unrelated compile/configuration error. The failing test remains in the final change.

See `docs/tdd.md` for local and GitHub-only workflows.

## 4. Commit discipline

A branch may use separate Red and Green commits so CI evidence is easy to inspect. Exact commit-message prefixes are not an architecture requirement, but the history/PR must make the TDD sequence auditable.

Do not:

- weaken/delete a failing requirement to make CI green;
- mix unrelated refactors into a semantic change without tests;
- edit generated artifacts by hand;
- edit an already-published Atlas migration;
- copy architecture Markdown into this repository.

## 5. Required validation

Run the smallest relevant suite during the Red/Green loop, then the complete affected gate before merge.

Typical mapping:

- pure Go domain change → T1 + affected T2;
- Relay HTTP/streaming change → T1 + Relay T2 + affected T3;
- Hub persistence change → unit + real SQLite component integration + migration checks where applicable;
- Admin change → unit/store/component + build + affected real-Hub browser lane;
- OpenAPI change → T0 contract/codegen/fixtures + all affected consumers;
- cross-component behavior → required T3 and mapped `SYS-*` scenario as appropriate.

The architecture Testing Specs define what must be proven; `docs/testing.md` defines how this repository executes that proof.

## 6. Generated code

Generated outputs are derived artifacts. Change the source OpenAPI/schema/generator configuration, regenerate deterministically, and verify drift. Never patch a generated DTO to create a protocol change.

## 7. Database migrations

Follow `docs/database-migrations.md`. Ent schema and generated Atlas migration are reviewed together. An empty-database replay and affected upgrade test must pass before merge.

## 8. Review standard

A PR is ready when:

- architecture impact is correctly classified;
- required Red/Green evidence exists for behavior changes;
- required CI checks pass on the latest commit;
- generated artifacts and fixtures do not drift;
- relevant migration/integration/system tests pass;
- no critical scenario is skipped or hidden by retry;
- documentation is updated only in the repository that owns the changed fact.
