# Review: PR #1, notes on a visit (`review/visit-notes`)

## Verdict: request changes

The main idea is good and most of it is built well. It cannot be merged yet for three reasons:

- CI will fail. Three of the four lint checks fail on this branch.
- Changes to notes are not recorded in the audit log, so an old note can be overwritten with no trace.
- The recent-notes panel can miss visits because it filters dates the wrong way.

All of these are small fixes. Once they are done I would expect to approve.

Line numbers below are from the branch as it is now.

## How I checked

I read the full diff against `main` and ran the branch in a separate copy:

```bash
for f in scripts/check-*.sh; do bash "$f"; done
(cd backend && go test -count=1 ./...)
(cd frontend && npx vitest run && npx tsc --noEmit -p .)
```

The backend tests pass, the 44 frontend tests pass and the type check is clean. These lint checks fail:

```
check-audit-coverage   routes.go:45  PATCH /api/visits/:id/notes has no Register entry
check-frontend-fetch   RecentNotes.tsx:25  raw fetch(
check-hardcoded-roles  visit_handler.go:154  "dispatcher"
```

## Must change before merge

### 1. Notes changes are not audited

`backend/internal/api/routes.go:45` adds `PATCH /api/visits/:id/notes`, but there is no matching `registry.Register(...)` in `backend/internal/middleware/audit.go`.

Why it matters: when a route is not registered, the audit middleware lets the request through and writes nothing (`middleware/audit.go:79-82`). Notes can be edited on closed visits, so the audit log is the only record of what a note said before. Without it, notes can be rewritten after the job is done and nobody can see who changed them or what they said. This breaks CONVENTIONS §7 and fails CI.

The fix is small. `SnapshotVisit` already includes `notes` in this PR (`services/audit.go:180`, `:192`), so the route only needs registering, for example with `AuditActionUpdate` and `audit.SnapshotVisit`.

### 2. The update error is ignored inside the transaction

`backend/internal/services/visit.go:196`:

```go
tx.Model(&visit).Update("notes", value)
```

The error from `Update` is never checked. CONVENTIONS §4 says never to do this inside a transaction. On Postgres a failed statement stops the whole transaction, so the next query (`s.load` on line 197) also fails. The user gets a 500, and the log shows "transaction aborted" instead of the real cause. On SQLite, which the tests use, the failure is silent and the API returns 200 with the old notes.

Fix: check `.Error` and return it.

### 3. The permission check is in the handler

`backend/internal/api/visit_handler.go:153-159` decides who may write notes inside the handler, and it uses the string `"dispatcher"`.

Why it matters:

- CONVENTIONS §1 says business logic belongs in `internal/services`. No other handler on `main` checks roles. `Cancel`, `ClockIn` and `ClockOut` pass the actor to the service, and the service checks it with `assertOwnVisit` (`services/visit.go:345`).
- `UpdateNotes` does not take an actor, so any other caller of the service skips the check completely.
- The `"dispatcher"` string breaks CONVENTIONS §2 and fails CI.

Fix: change the service to `UpdateNotes(ctx, id, actor, notes)`, call `assertOwnVisit` inside it, and delete the role check from the handler. This fixes both problems.

### 4. The recent-notes panel fetches and filters data the wrong way

`frontend/src/components/RecentNotes.tsx:25-32` has three problems:

- **Raw `fetch` (line 25).** CONVENTIONS §9 says every call goes through `ApiClient`. CI fails on this. It also skips `ApiClient`'s handling of an expired login (`lib/api.ts:77-79` clears the session and sends the user to the login page), so an expired session just shows "Recent notes could not be loaded."
- **Wrong day check (line 31).** `v.scheduled_start.split('T')[0] === day` compares the UTC date with a UK day. The API sends times in UTC, so a visit at 00:30 UK time in summer is `...T23:30:00Z` on the previous day, and the panel drops it. CONVENTIONS §12 says to use `ukDatePart` for this. The lint only catches `.split('T')[1]`, so it does not see this one. The test fixtures are all midday UTC, so the tests do not catch it either. The server already filters by UK day through `from` and `to`, so this check can simply be deleted.
- **Only reads the first page.** The request has no `page_size`, so it gets the first 20 visits and filters them in the browser. CONVENTIONS §11 says never to filter a paginated result on the client, because rows after page 1 silently disappear.

## Should change

### 5. No limit on note length

`backend/internal/models/visit.go:16` makes `notes` an unlimited `text` column, and `UpdateNotes` does not check the length. Every other text field is limited (site name 200, address 500, user names 100), and the backend has no request body limit. One very large request could store megabytes in a single row, and it would be copied into every visit list response. Suggest a limit (for example 2,000 characters) that returns 400, documented in BUSINESS_RULES §3.7 and `openapi.yaml`.

### 6. Is the recent-notes panel part of this change?

The task was notes on a visit. The panel (`RecentNotes.tsx`, used at `VisitDetailPage.tsx:176`) is a separate view across several visits. Two things suggest it could go in its own PR:

- CONVENTIONS §16 asks for one concern per PR, and item 4 above is all in the panel. Without it, the main notes feature could merge sooner.
- BUSINESS_RULES §3.7 does not mention the panel or the list marker, yet their tests cite `// rule: §3.7`. If they stay, §3.7 should describe them (CONVENTIONS §14).

If the panel was asked for, keeping it is fine once item 4 is fixed.

### 7. Tests to add or adjust

- Add a test that a notes change writes an audit row. That covers item 1 and stops it from coming back.
- Add a service test where the technician is not assigned, once the check moves into the service (item 3).
- `TestVisitNotes_PutByAssignedTechnicianAndStaff` (`visit_notes_test.go:54`) sends `PATCH`, not `PUT`. Renaming it would avoid confusion.

## Fine as it is

- **No XSS risk.** Notes are shown as plain text by React, and nothing on the branch uses `dangerouslySetInnerHTML`. `white-space: pre-wrap` keeps line breaks.
- **Safe schema change.** Adding a nullable column needs no backfill, and existing visits get `null`.
- **One meaning for "no notes".** Whitespace is trimmed and a blank note is saved as `null`, so "no notes" is always `null` and never `""` or spaces. This is tested.
- **Documentation came with the change.** BUSINESS_RULES §3.7 and `openapi.yaml` describe the new route, and the tests cite the rule (CONVENTIONS §14).
- **Frontend and backend agree on who can edit.** `canEditNotes` uses `canClock`, which allows staff or the assigned technician. That matches the backend rule.
- **Editing closed visits is a clear decision.** Notes stay writable after clock-out because findings are written up afterwards. This is reasoned and documented in §3.7. It is also why item 1 matters.
- **Good frontend tests.** Both the editor and the read-only view are covered, including a technician who is not assigned.
