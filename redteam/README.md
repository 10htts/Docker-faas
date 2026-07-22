# Red-team process for Docker-faas OpenFaaS-compatibility hardening

This directory is the adversarial record for the 2026-07 hardening pass
("OpenFaaS provider contract + production readiness"). Prior red-team rounds
(objections SZ-01..SZ-12) are referenced throughout the code but their records
were not committed; this directory reconstructs the live ledger and carries the
new round.

## Process

1. **Builders** implement against explicitly scoped objections (disjoint file
   scopes; one builder per area: security, gate state machine, conformance,
   scaling safety).
2. **Critics** are read-only reviewers. Each critic attacks one builder's output
   plus its blast radius and files objections here with concrete evidence
   (file:line, failing scenario, or reproducible command). Critics may reopen
   any objection closed without evidence.
3. **Adjudicator** is independent of builders and critics. It rules each
   objection: `accepted` (must fix), `rejected` (with reasoning), or
   `deferred` (documented residual risk with rationale).
4. **Integrator** reviews the combined diff for OpenFaaS contract drift before
   the verdict.

Stop condition: no HIGH objection open, or the same blocker repeats twice with
no new evidence (then it is recorded as a blocker in the final verdict).

## Files

- `OBJECTIONS.md` — the live ledger (single source of truth for status).
- `ADJUDICATION.md` — adjudicator rulings with reasoning and evidence.
- `CONFORMANCE_MATRIX.md` — per-operation OpenFaaS endpoint inventory with
  mandatory/optional, supported/unsupported, and evidence for each claim.
- `VERIFICATION.md` — exact commands and outputs for the verification runs.

## Severity

- **HIGH** — can corrupt state, lose work, break the OpenFaaS contract for
  standard clients, or allow unauthenticated mutation. Blocks the verdict.
- **MED** — degrades operations or leaves a contract edge unproven.
- **LOW** — cosmetic, documentation, or hardening-in-depth.
