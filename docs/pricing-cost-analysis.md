# Pricing and cost analysis implementation plan

This work completes the existing S0.2 Admin pricing surface. Product semantics live
in the sibling architecture Control Protocol §19; the Admin OpenAPI owns the wire.
No database migration, Relay behavior change, currency conversion, or cost budget is
introduced. Portal and Android contracts remain unchanged.

## Calculation

1. Read the current whole pricing set once per analysis. For each retained request,
   use the current settlement revision and request completion time. Select a meter
   rule by exact resource, then upstream, then global scope; within a scope use the
   latest effective rule. A rule whose effective interval excludes the request cannot
   price it. The current rule set can reprice history without rewriting usage facts.
2. Use exact decimal rationals through aggregation. Return decimal amounts rounded to 12
   fractional digits only when division cannot terminate. Keep each currency in its
   own subtotal. Show a single `amount/currency` only when there is one currency.
3. Token basis: use TOTAL only when there are no applicable component rates. With
   component rates, use INPUT and OUTPUT; CACHED is already inside INPUT. When a
   CACHED rate exists, subtract its quantity from INPUT before charging the ordinary
   input rate. Never charge TOTAL again. REQUESTS can be an additional explicit fee.
   Other configured, non-overlapping capability meters can be additive.
4. Never turn absent prices or unreliable quantities into zero. KNOWN requires full
   coverage of the selected price basis, PARTIAL preserves known subtotals alongside
   incomplete requests, UNKNOWN has no priced subtotal. A request proven not
   forwarded has zero upstream cost and is shown as a known zero even without a
   currency. Empty analysis has UNKNOWN status.

## Read models and UI

- Use one bounded cursor scan over filtered request facts with a batch join to the
  current semantic revision. Reuse the same evaluator for global summary, daily
  trend, resource/protocol distribution, and request list/detail.
- Extend the Admin wire with an optional cost projection on trend, distribution and
  request views, plus per-currency subtotals, cause counts, and request pricing lines.
  Preserve the existing rule's optional exclusive `effectiveTo` through Admin reads,
  full-set writes, and the editor so saving a price never silently changes its interval.
  Keep the existing summary cost status/amount/currency fields for current callers.
- Summary follows the active time/user/resource/upstream/status/completeness filter.
  The Pricing tab labels its separate all-history estimate explicitly and refreshes
  it after saving rules. Show estimated cost, status and missing-price/meter counts;
  request detail shows each applied rule and arithmetic. Never call an estimate an
  invoice or claim that an unknown amount is zero.

## Acceptance matrix

- Model: INPUT/OUTPUT/CACHED/TOTAL precedence; cached subset; missing rate; UNKNOWN
  and PARTIAL usage; repeating decimal; scoped/latest/effective interval.
- Image, TTS, ASR, MCP: the declared REQUESTS, REQUESTED_IMAGES, CHARACTERS and
  AUDIO_SECONDS meters; per-call and additive rates; no applicable rule.
- Ledger: latest settlement revision only, no forward, combined filters, day and
  resource grouping, pagination boundaries, no unbounded in-memory request list.
- Money: partial and unknown cause counts, multiple currencies without FX,
  empty-window behavior, exact arithmetic and projection identity.
- UI/API: saved pricing set followed by refreshed estimate, filtered summary,
  daily/resource/request presentations, missing-data labels, Admin-only contract,
  generated API drift check, backend/frontend tests, typecheck and production build.
