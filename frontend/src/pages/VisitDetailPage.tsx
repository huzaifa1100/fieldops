import { useCallback, useEffect, useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import { ErrorMessage } from '../components/ErrorMessage';
import { PriorityLabel } from '../components/PriorityLabel';
import { StatusBadge } from '../components/StatusBadge';
import { apiClient, describeError } from '../lib/api';
import { useCurrentUser } from '../lib/auth';
import { formatDateTimeUK, formatRangeUK } from '../lib/dateFormat';
import { recordedByName, technicianName } from '../lib/names';
import {
  canCancelVisits,
  canClock,
  cancelAllowed,
  clockInAllowed,
  clockOutAllowed,
} from '../lib/permissions';
import type { ClockKind, Visit } from '../types';

const KIND_LABELS: Record<ClockKind, string> = { in: 'Clock in', out: 'Clock out' };

export function VisitDetailPage() {
  const { id } = useParams();
  const visitId = Number(id);
  const user = useCurrentUser();

  const [visit, setVisit] = useState<Visit | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    try {
      setVisit(await apiClient.getVisit(visitId));
      setLoadError(null);
    } catch (err) {
      setLoadError(describeError(err));
    }
  }, [visitId]);

  useEffect(() => {
    void load();
  }, [load]);

  const perform = async (action: () => Promise<Visit>) => {
    setBusy(true);
    setActionError(null);
    try {
      await action();
      await load();
    } catch (err) {
      setActionError(describeError(err));
    } finally {
      setBusy(false);
    }
  };

  const cancel = () => {
    if (!window.confirm('Cancel this visit? This cannot be undone.')) return;
    void perform(() => apiClient.cancelVisit(visitId));
  };

  if (loadError && !visit) {
    return (
      <>
        <h1>Visit</h1>
        <ErrorMessage message={loadError} />
        <Link to="/visits">Back to visits</Link>
      </>
    );
  }
  if (!visit) return <p className="muted">Loading visit…</p>;

  const showClock = canClock(user, visit);
  const showCancel = canCancelVisits(user);

  return (
    <>
      <p>
        <Link to="/visits">← Visits</Link>
      </p>
      <h1 className="title-row">
        {visit.site.name} <StatusBadge status={visit.status} />
      </h1>

      <dl className="facts card">
        <dt>Site</dt>
        <dd>{visit.site.name}</dd>
        <dt>Technician</dt>
        <dd>{technicianName(visit.technician)}</dd>
        <dt>Scheduled</dt>
        <dd>{formatRangeUK(visit.scheduled_start, visit.scheduled_end)}</dd>
        <dt>Status</dt>
        <dd>
          <StatusBadge status={visit.status} />
        </dd>
        <dt>Priority</dt>
        <dd>
          <PriorityLabel priority={visit.priority} />
        </dd>
        {visit.cancelled_at && (
          <>
            <dt>Cancelled</dt>
            <dd>{formatDateTimeUK(visit.cancelled_at)}</dd>
          </>
        )}
      </dl>

      {(showClock || showCancel) && (
        <section className="actions">
          {showClock && (
            <>
              <button
                type="button"
                className="btn btn-primary"
                disabled={busy || !clockInAllowed(visit.status)}
                onClick={() => void perform(() => apiClient.clockIn(visitId))}
              >
                Clock in
              </button>
              <button
                type="button"
                className="btn btn-primary"
                disabled={busy || !clockOutAllowed(visit.status)}
                onClick={() => void perform(() => apiClient.clockOut(visitId))}
              >
                Clock out
              </button>
            </>
          )}
          {showCancel && (
            <button
              type="button"
              className="btn btn-danger"
              disabled={busy || !cancelAllowed(visit.status)}
              onClick={cancel}
            >
              Cancel visit
            </button>
          )}
        </section>
      )}
      <ErrorMessage message={actionError} />

      <h2>Clock events</h2>
      {visit.clock_events.length === 0 ? (
        <p className="muted">No clock events yet.</p>
      ) : (
        <table className="list">
          <thead>
            <tr>
              <th>Event</th>
              <th>Time</th>
              <th>Recorded by</th>
            </tr>
          </thead>
          <tbody>
            {visit.clock_events.map((event) => (
              <tr key={event.id}>
                <td data-label="Event">{KIND_LABELS[event.kind]}</td>
                <td data-label="Time">{formatDateTimeUK(event.occurred_at)}</td>
                <td data-label="Recorded by">{recordedByName(event.recorded_by)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </>
  );
}
