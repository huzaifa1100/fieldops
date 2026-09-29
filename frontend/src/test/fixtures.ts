import type { ClockEvent, DailyReport, User, Visit, VisitListResponse } from '../types';

export function makeUser(overrides: Partial<User> = {}): User {
  return {
    id: 3,
    email: 'tech1@fieldops.local',
    first_name: 'Tess',
    last_name: 'Turner',
    role: 'technician',
    is_active: true,
    created_at: '2026-01-05T09:00:00Z',
    ...overrides,
  };
}

export function makeClockEvent(overrides: Partial<ClockEvent> = {}): ClockEvent {
  return {
    id: 11,
    kind: 'in',
    occurred_at: '2026-09-21T08:02:00Z',
    recorded_by_id: 3,
    recorded_by: { id: 3, first_name: 'Tess', last_name: 'Turner' },
    ...overrides,
  };
}

export function makeVisit(overrides: Partial<Visit> = {}): Visit {
  return {
    id: 7,
    site_id: 2,
    site: { id: 2, name: 'Riverside Depot' },
    technician_id: 3,
    technician: { id: 3, first_name: 'Tess', last_name: 'Turner' },
    scheduled_start: '2026-09-21T08:00:00Z',
    scheduled_end: '2026-09-21T10:00:00Z',
    status: 'scheduled',
    priority: 'normal',
    cancelled_by_id: null,
    cancelled_at: null,
    clock_events: [],
    ...overrides,
  };
}

export function makeVisitList(overrides: Partial<VisitListResponse> = {}): VisitListResponse {
  return {
    visits: [makeVisit()],
    total: 1,
    page: 1,
    page_size: 25,
    total_pages: 1,
    ...overrides,
  };
}

export function makeReport(overrides: Partial<DailyReport> = {}): DailyReport {
  return {
    date: '2026-09-21',
    scheduled: 12,
    completed: 9,
    cancelled: 1,
    technicians: [
      { technician_id: 3, name: 'Tess Turner', visits: 5, completed: 4, hours_clocked: 7.5 },
      { technician_id: 4, name: 'Omar Bell', visits: 4, completed: 3, hours_clocked: 6.125 },
    ],
    ...overrides,
  };
}
