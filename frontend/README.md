# frontend

React 18 + TypeScript + Vite. Technicians use it on a phone on site; dispatchers and admins use it on a desktop. One hand-written stylesheet (`src/styles.css`), no UI framework, no state library.

## Run

```bash
npm install
npm run dev        # http://localhost:3000, /api proxied to http://localhost:8080
```

From the repository root, `make frontend` does the same. Seed logins are listed in the root `README.md`.

## Test

```bash
npx vitest run                          # or: make test-frontend
TZ=Asia/Tokyo npx vitest run            # CI runs the suite under two foreign zones
TZ=America/Los_Angeles npx vitest run
```

Vitest with jsdom and Testing Library. `src/lib/*.test.ts` mock at the `fetch` level; page tests (`src/pages/*.test.tsx`) mount the real router through `src/test/render.tsx` and mock `apiClient` methods with `vi.spyOn`.

## Build

```bash
npx tsc --noEmit   # must be clean; a single tsconfig covers src/ and the tests
npm run build      # runs the type check, then writes dist/
```

## Layout

```
src/
├── main.tsx, App.tsx      entry + routes: /login, /visits, /visits/:id, /reports/daily
├── types.ts               the API's JSON shapes, snake_case, field for field
├── styles.css             the stylesheet (tables collapse to labelled cards under 640px)
├── lib/
│   ├── api.ts             the only HTTP client
│   ├── dateFormat.ts      the only date renderer
│   ├── session.ts         token + user in localStorage
│   ├── auth.tsx           AuthProvider / useAuth / useCurrentUser
│   ├── permissions.ts     who may clock, cancel, see reports; visit state machine gates
│   └── names.ts           "First Last", "Unassigned", "Unknown user"
├── components/            Header, Layout, RequireAuth, StatusBadge, PriorityLabel, Pager, ErrorMessage
├── pages/                 LoginPage, VisitsPage, VisitDetailPage, DailyReportPage (+ tests)
└── test/                  setup, fixtures, renderApp
```

## The two modules that own a concern

**`src/lib/api.ts`** is the only file that calls `fetch`. Its private `request()` attaches the JWT from `localStorage` (`fieldops_token`), sends JSON, unwraps the backend envelope — a `{ "success": true, "data": … }` body returns `data`, a `{ "success": false, "error": … }` body throws an `ApiError` carrying the HTTP status and that message — and, on a 401 to an authenticated request, clears the session and sends the browser to `/login`. Every public method is typed to the exact response shape, so nothing downstream ever reads `.data` or reaches for `any`. Keeping one client means auth, error shape and envelope handling are decided once; `scripts/check-frontend-fetch.sh` fails the build on a `fetch` anywhere else (CONVENTIONS rule 9).

**`src/lib/dateFormat.ts`** is the only file that turns an API timestamp into text. The API sends RFC3339 instants; which calendar day or wall-clock time an instant is on is a Europe/London question, never a browser-timezone one (BUSINESS_RULES §3.3). `formatDateTimeUK` / `formatTimeUK` / `formatRangeUK` render London time through `Intl.DateTimeFormat` with `timeZone: 'Europe/London'`; `ukDatePart` gives the London `YYYY-MM-DD` of an instant (the first ten characters of the ISO string would be the UTC day, which is wrong for an hour a night all summer); `todayUK` and `addDaysUK` do day arithmetic for the filters. Nothing else in `src/` calls `toLocale*String` or slices ISO strings, and `scripts/check-naive-timestamp-reads.sh` enforces that (rule 12).

## Conventions that show up in the pages

- Lists are filtered server-side: every change to the date range, status or technician filter refetches with those params and the pager renders from the `page_size` / `total_pages` the server echoes (rule 11).
- Clock in / clock out appear only for users who may use them (§4.1) and are enabled by the visit's status (§4.2); the server's 409 message is shown inline, verbatim.
- A clock event whose recorder has been deleted renders "Unknown user" and nothing else on the page changes (§4.3).
