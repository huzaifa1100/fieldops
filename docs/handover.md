# Handover

## What I changed and why

- **Daily report bug (Task 1).** The report worked out "which day" from the server's own timezone and used a fixed 24-hour day, so late-evening visits and the day the clocks change landed on the wrong day. It also dropped technicians who had been deleted since. The report now uses the Europe/London day (`UKDayRange`) and loads deleted technicians. The tests went in first, in their own commit, and failed for those exact reasons.
- **Visit priority (Task 2).** `low`, `normal` (the default) or `high`: set through the API on create and edit, filtered with `?priority=`, recorded in the audit log, and shown in the visits list and detail. An edit that leaves it out resets it to `normal`, because an edit replaces the whole visit, the same way leaving out `technician_id` unassigns it. The rule is in BUSINESS_RULES §3.7 and in `openapi.yaml`.
- **Review of PR #1** in `docs/review-visit-notes.md`. **Dispatch requests** in `docs/dispatch-requests.md`: no code changes, one waiting for a decision and one declined. **Operations answers** in `docs/ops-answers.md`.

## What worried me but I did not touch

- **The health check never touches the database** (`handlers.go:29`). An instance with a broken database still looks healthy, so a bad deploy can finish "green".
- **Every instance runs the migration at boot.** Two instances starting together could race on the same change (I have not tested this), and nothing records which schema version is live.
- **Neither secret can be rotated safely.** `JWT_SECRET` is one key, so a rotation logs everyone out, and during a rolling refresh users get random 401s. `ENCRYPTION_KEY` is also one key with no key id. A wrong key breaks licence numbers and also every audited change to a user, because the audit diff decrypts the user (`audit.go:101`).
- **Backend tests run on SQLite, production on Postgres.** SQLite compares timestamps as text, so a timezone bug can behave differently in tests or not show up at all. I noted this in `report_test.go` for the Task 1 test.
- **CONVENTIONS §5 (no server-timezone dates in the backend) has no CI check.** `check-naive-timestamp-reads.sh` scans only the frontend. The Task 1 bug broke exactly this rule.
- **I could not check that production has `seed_on_boot` off.** It defaults to false, but the real value lives with whoever runs AWS. If it were on, every deploy would add demo visits to real data.
- **`openapi.yaml` is not validated in CI.** My first Task 2 change failed redocly, and only a manual run caught it.

## Three things for my first month

1. **Make the health check prove the database works**, including a simple schema check. This is the cheapest guard against the kind of failure in the incident question, and it lets the load balancer stop a bad deploy by itself.
2. **Make both secrets rotatable.** Check tokens against a list of JWT secrets, and give encrypted values a key id with a tool that re-encrypts them. Today, rotating either secret means an outage or lost data, so in practice no one will rotate them.
3. **Run the backend tests on Postgres in CI, and add a backend check for §5.** The system's main rule is about time, and today neither the tests nor the linter caught the Task 1 bug.
