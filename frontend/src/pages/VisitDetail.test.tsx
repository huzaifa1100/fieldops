import { fireEvent, screen, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { ApiError, apiClient } from '../lib/api';
import { makeClockEvent, makeUser, makeVisit } from '../test/fixtures';
import { renderApp } from '../test/render';

describe('VisitDetailPage', () => {
  const assignedTech = makeUser({ id: 3, role: 'technician' });
  const otherTech = makeUser({ id: 4, role: 'technician', first_name: 'Omar', last_name: 'Bell' });
  const dispatcher = makeUser({ id: 2, role: 'dispatcher', first_name: 'Dee', last_name: 'Patch' });

  // rule: §4.1 — a technician clocks their own visits only
  it('shows the clock buttons to the assigned technician', async () => {
    vi.spyOn(apiClient, 'getVisit').mockResolvedValue(makeVisit({ technician_id: 3 }));
    renderApp('/visits/7', assignedTech);

    expect(await screen.findByRole('button', { name: 'Clock in' })).toBeEnabled();
    expect(screen.getByRole('button', { name: 'Clock out' })).toBeDisabled();
    expect(screen.queryByRole('button', { name: 'Cancel visit' })).not.toBeInTheDocument();
  });

  // rule: §4.1 — a technician clocks their own visits only
  it('hides the clock buttons from a technician who is not assigned', async () => {
    vi.spyOn(apiClient, 'getVisit').mockResolvedValue(makeVisit({ technician_id: 3 }));
    renderApp('/visits/7', otherTech);

    await screen.findByText('Tess Turner');
    expect(screen.queryByRole('button', { name: 'Clock in' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Clock out' })).not.toBeInTheDocument();
  });

  // rule: §4.1 — admins and dispatchers may clock on any visit
  it('shows clock and cancel buttons to a dispatcher', async () => {
    vi.spyOn(apiClient, 'getVisit').mockResolvedValue(makeVisit({ technician_id: 3 }));
    renderApp('/visits/7', dispatcher);

    expect(await screen.findByRole('button', { name: 'Clock in' })).toBeEnabled();
    expect(screen.getByRole('button', { name: 'Cancel visit' })).toBeEnabled();
  });

  // rule: §4.2 — clock-in moves the visit to in_progress; clock-out to completed
  it('enables clock out only while in progress and reloads after a successful clock', async () => {
    const getVisit = vi
      .spyOn(apiClient, 'getVisit')
      .mockResolvedValueOnce(makeVisit({ status: 'in_progress' }))
      .mockResolvedValueOnce(makeVisit({ status: 'completed' }));
    const clockOut = vi.spyOn(apiClient, 'clockOut').mockResolvedValue(makeVisit({ status: 'completed' }));
    renderApp('/visits/7', assignedTech);

    expect(await screen.findByRole('button', { name: 'Clock in' })).toBeDisabled();
    fireEvent.click(screen.getByRole('button', { name: 'Clock out' }));

    await waitFor(() => expect(getVisit).toHaveBeenCalledTimes(2));
    expect(clockOut).toHaveBeenCalledWith(7);
    expect(await screen.findAllByText('Completed')).not.toHaveLength(0);
    expect(screen.getByRole('button', { name: 'Clock out' })).toBeDisabled();
  });

  // rule: §4.2 — clock-out without a clock-in is rejected (409)
  it('renders the server message inline when clockOut returns 409', async () => {
    vi.spyOn(apiClient, 'getVisit').mockResolvedValue(makeVisit({ status: 'in_progress' }));
    vi.spyOn(apiClient, 'clockOut').mockRejectedValue(new ApiError(409, 'visit has no clock-in to close'));
    renderApp('/visits/7', assignedTech);

    fireEvent.click(await screen.findByRole('button', { name: 'Clock out' }));

    expect(await screen.findByRole('alert')).toHaveTextContent('visit has no clock-in to close');
    expect(screen.getByRole('button', { name: 'Clock out' })).toBeEnabled();
  });

  // rule: §4.3 — the recorded-by name is shown even after that user is gone
  it('renders "Unknown user" for a clock event whose recorder is null and keeps the rest intact', async () => {
    vi.spyOn(apiClient, 'getVisit').mockResolvedValue(
      makeVisit({
        status: 'completed',
        clock_events: [
          makeClockEvent({ id: 11, kind: 'in', occurred_at: '2026-09-21T08:02:00Z' }),
          makeClockEvent({ id: 12, kind: 'out', occurred_at: '2026-09-21T09:58:00Z', recorded_by: null }),
        ],
      }),
    );
    renderApp('/visits/7', dispatcher);

    await screen.findByText('Unknown user');
    expect(screen.getByText('21 Sep 2026, 09:02')).toBeInTheDocument();
    expect(screen.getByText('21 Sep 2026, 10:58')).toBeInTheDocument();
    expect(screen.getByRole('cell', { name: 'Tess Turner' })).toBeInTheDocument();
  });

  // rule: §3.7 — the visit's priority is shown on the detail page
  it('shows the visit\'s priority', async () => {
    vi.spyOn(apiClient, 'getVisit').mockResolvedValue(makeVisit({ priority: 'high' }));
    renderApp('/visits/7', dispatcher);

    expect(await screen.findByText('High')).toBeInTheDocument();
    expect(screen.getByText('Priority')).toBeInTheDocument();
  });

  it('cancels after confirmation and shows the cancelled state', async () => {
    vi.spyOn(window, 'confirm').mockReturnValue(true);
    const getVisit = vi
      .spyOn(apiClient, 'getVisit')
      .mockResolvedValueOnce(makeVisit())
      .mockResolvedValueOnce(
        makeVisit({ status: 'cancelled', cancelled_by_id: 2, cancelled_at: '2026-09-21T07:00:00Z' }),
      );
    const cancelVisit = vi.spyOn(apiClient, 'cancelVisit').mockResolvedValue(makeVisit({ status: 'cancelled' }));
    renderApp('/visits/7', dispatcher);

    fireEvent.click(await screen.findByRole('button', { name: 'Cancel visit' }));

    await waitFor(() => expect(getVisit).toHaveBeenCalledTimes(2));
    expect(cancelVisit).toHaveBeenCalledWith(7);
    expect(await screen.findByText('21 Sep 2026, 08:00')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Cancel visit' })).toBeDisabled();
  });
});
