# Business rules

The source of truth for what fieldops does. Tests that verify a rule cite it with `// rule: §X.Y` above the test function.

## §1 Users and roles

- **§1.1** Three roles: `admin`, `dispatcher`, `technician`. Admin can do everything; dispatcher manages sites and visits; technician sees and clocks their own visits only.
- **§1.2** Login is email + password → JWT (8 h). A deactivated user (`is_active = false`) cannot log in; a soft-deleted user cannot log in and does not appear in user lists, but their historical attributions (visits scheduled, clock events) still show their name.
- **§1.3** `users.licence_number` is encrypted at rest and returned decrypted only on `GET /api/users/:id` to admins.

## §2 Sites

- **§2.1** A site has a name, an address and a contact phone. Sites are soft-deleted; a deleted site's past visits remain readable.

## §3 Visits

- **§3.1** A visit has a site, an optional technician, a scheduled start and end (instants), and a status: `scheduled`, `in_progress`, `completed`, `cancelled`.
- **§3.2** `GET /api/visits` requires `from` and `to` (YYYY-MM-DD, Europe/London days, inclusive) and filters server-side; `technician_id`, `status` and `priority` are optional filters. Pages are capped at 100 and the effective `page_size` is echoed.
- **§3.3** A visit whose scheduled start is on Europe/London day D belongs to day D in every report and list, whatever the server's or the browser's timezone.
- **§3.4** Cancelling a visit keeps the row (status `cancelled`) and records who cancelled it and when. Only a `scheduled` or `in_progress` visit can be cancelled; cancelling a `completed` or `cancelled` one is rejected (409).
- **§3.5** A `completed` or `cancelled` visit is closed: `PUT /api/visits/:id` on it is rejected (409). Status never changes through `PUT`; only cancel and clock actions move it.
- **§3.6** A visit's site must be a live (not soft-deleted) site and its technician, when set, must be an active user with the `technician` role; otherwise the request is rejected (400). `scheduled_end` must be after `scheduled_start`, and both are RFC3339 instants with an offset.
- **§3.7** A visit has a priority: `low`, `normal` or `high`. It is set on create and on `PUT /api/visits/:id`. When the request leaves it out or sends null, it is `normal`, on an edit too, because `PUT` replaces every writable field (the same way an omitted `technician_id` unassigns the visit). Any other value is rejected (400). `GET /api/visits?priority=` takes one value; an unknown value is rejected (400), not answered with an empty list.

## §4 Clock events

- **§4.1** A technician clocks in and out of their own visits only; admins and dispatchers may clock on any visit (recorded as done by them).
- **§4.2** Clock-in moves the visit to `in_progress`; clock-out moves it to `completed`. Clock-out without a clock-in is rejected (409). A second clock-in is rejected (409).
- **§4.3** Each clock event stores `occurred_at` (an instant) and `recorded_by`. The recorded-by name is shown even after that user is soft-deleted (§1.2).

## §5 Daily report

- **§5.1** `GET /api/reports/daily?date=YYYY-MM-DD` returns, for that Europe/London day: visits scheduled, completed, cancelled, and per-technician counts with hours clocked. Day membership follows §3.3.
- **§5.2** A technician with visits on the day appears in the report even if they were deactivated or soft-deleted afterwards.
- **§5.3** `scheduled` is the day's whole book: every visit whose scheduled start falls on the day, in any status. `completed` and `cancelled` are the subsets in those states. Per technician, `visits` counts their visits that day in any status, `completed` the completed subset, and `hours_clocked` the sum of clock-in → clock-out pairs (an open clock-in contributes nothing). Visits with no technician count in the day totals but appear on no technician's line.

## §6 Audit trail

- **§6.1** Every create, update, delete and clock action writes an `audit_logs` row: actor, action, resource type and id, and a before/after diff of changed fields.
- **§6.2** Audit rows are never edited or deleted.
- **§6.3** A successful login writes an audit row (`login` on the user, empty diff). A request that fails (4xx/5xx) writes no row, because nothing changed. In a user's audit diff `licence_number` is recorded as a fingerprint of the value, never the value itself, so a change is visible without the licence number leaving its encrypted column.
- **§6.4** A bearer token is re-checked against the user row on every request: a deactivated or soft-deleted user's outstanding token stops working immediately, not when it expires.
