## Architecture impact

- [ ] None — pure implementation/documentation/tooling change
- [ ] Implements existing architecture requirement: <!-- document/section/scenario ID -->
- [ ] Requires architecture change: <!-- measix-architecture PR/commit -->

## Change

<!-- What changed and why. Do not restate architecture; link the authority. -->

## TDD evidence

For behavior changes / bug fixes:

- Red commit/check: <!-- SHA, test name, expected failure -->
- Green commit/check: <!-- SHA/latest head, checks -->
- Refactor evidence: <!-- if applicable -->

For a TDD-exempt change (docs-only, formatting, deterministic regeneration), explain briefly:

<!-- reason -->

## Tests executed

- [ ] T0 Static / Contract
- [ ] T1 Unit / Domain
- [ ] T2 Component Integration
- [ ] T3 Cross-component Integration
- [ ] T4 System / E2E
- Critical scenario IDs: <!-- HUB-*, RLY-*, ADM-*, SYS-* -->

## Contract / generated artifacts

- OpenAPI impact: <!-- none / files -->
- Fixture impact: <!-- none / files -->
- Generated-code drift verified: <!-- yes/no/not applicable -->
- Android contract synchronization required: <!-- yes/no -->

## Database / operations

- Migration impact: <!-- none / migration + tests -->
- Configuration/operations impact: <!-- none / docs updated -->
- Release/RC impact: <!-- none / required gates -->

## Verification gaps / risks

<!-- Anything not executed or not yet reproducible must be stated explicitly. -->
