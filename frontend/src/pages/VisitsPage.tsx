import { useEffect, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { ErrorMessage } from '../components/ErrorMessage';
import { Pager } from '../components/Pager';
import { PriorityLabel } from '../components/PriorityLabel';
import { StatusBadge } from '../components/StatusBadge';
import { apiClient, describeError } from '../lib/api';
import { useCurrentUser } from '../lib/auth';
import { formatRangeUK, todayUK } from '../lib/dateFormat';
import { fullName, technicianName } from '../lib/names';
import { isStaff } from '../lib/permissions';
import type { User, VisitListResponse, VisitStatus } from '../types';

const PAGE_SIZE = 25;
const STATUSES: VisitStatus[] = ['scheduled', 'in_progress', 'completed', 'cancelled'];

function asStatus(value: string | null): VisitStatus | undefined {
  return STATUSES.find((status) => status === value);
}

function asPage(value: string | null): number {
  const page = Number(value);
  return Number.isInteger(page) && page > 0 ? page : 1;
}

export function VisitsPage() {
  const user = useCurrentUser();
  const staff = isStaff(user);
  const [params, setParams] = useSearchParams();

  const today = todayUK();
  const from = params.get('from') ?? today;
  const to = params.get('to') ?? today;
  const status = asStatus(params.get('status'));
  const technicianId = params.get('technician_id') ?? '';
  const page = asPage(params.get('page'));

  const [result, setResult] = useState<VisitListResponse | null>(null);
  const [technicians, setTechnicians] = useState<User[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let stale = false;
    setLoading(true);
    setError(null);
    apiClient
      .getVisits({
        from,
        to,
        status,
        technician_id: technicianId ? Number(technicianId) : undefined,
        page,
        page_size: PAGE_SIZE,
      })
      .then((response) => {
        if (!stale) setResult(response);
      })
      .catch((err: unknown) => {
        if (!stale) setError(describeError(err));
      })
      .finally(() => {
        if (!stale) setLoading(false);
      });
    return () => {
      stale = true;
    };
  }, [from, to, status, technicianId, page]);

  useEffect(() => {
    if (!staff) return;
    apiClient
      .getUsers({ role: 'technician' })
      .then((response) => setTechnicians(response.users))
      .catch(() => setTechnicians([]));
  }, [staff]);

  const setFilter = (key: string, value: string) => {
    setParams(
      (previous) => {
        const next = new URLSearchParams(previous);
        if (value) next.set(key, value);
        else next.delete(key);
        if (key !== 'page') next.delete('page');
        return next;
      },
      { replace: true },
    );
  };

  return (
    <>
      <h1>Visits</h1>
      <form className="toolbar" onSubmit={(e) => e.preventDefault()}>
        <label className="field">
          From
          <input type="date" value={from} onChange={(e) => setFilter('from', e.target.value)} />
        </label>
        <label className="field">
          To
          <input type="date" value={to} onChange={(e) => setFilter('to', e.target.value)} />
        </label>
        <label className="field">
          Status
          <select value={status ?? ''} onChange={(e) => setFilter('status', e.target.value)}>
            <option value="">All statuses</option>
            {STATUSES.map((s) => (
              <option key={s} value={s}>
                {s.replace('_', ' ')}
              </option>
            ))}
          </select>
        </label>
        {staff && (
          <label className="field">
            Technician
            <select value={technicianId} onChange={(e) => setFilter('technician_id', e.target.value)}>
              <option value="">All technicians</option>
              {technicians.map((t) => (
                <option key={t.id} value={t.id}>
                  {fullName(t)}
                </option>
              ))}
            </select>
          </label>
        )}
      </form>

      <ErrorMessage message={error} />

      <div aria-busy={loading}>
        {loading && !result && <p className="muted">Loading visits…</p>}
        {result && result.visits.length === 0 && <p className="muted">No visits in this range.</p>}
        {result && result.visits.length > 0 && (
          <table className="list">
            <thead>
              <tr>
                <th>Site</th>
                <th>Technician</th>
                <th>Scheduled</th>
                <th>Status</th>
                <th>Priority</th>
              </tr>
            </thead>
            <tbody>
              {result.visits.map((visit) => (
                <tr key={visit.id}>
                  <td data-label="Site">
                    <Link to={`/visits/${visit.id}`}>{visit.site.name}</Link>
                  </td>
                  <td data-label="Technician">{technicianName(visit.technician)}</td>
                  <td data-label="Scheduled">{formatRangeUK(visit.scheduled_start, visit.scheduled_end)}</td>
                  <td data-label="Status">
                    <StatusBadge status={visit.status} />
                  </td>
                  <td data-label="Priority">
                    <PriorityLabel priority={visit.priority} />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {result && (
        <Pager
          page={result.page}
          pageSize={result.page_size}
          totalPages={result.total_pages}
          total={result.total}
          onPage={(next) => setFilter('page', String(next))}
        />
      )}
    </>
  );
}
