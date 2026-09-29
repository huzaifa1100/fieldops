import { fireEvent, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { apiClient } from '../lib/api';
import { todayUK } from '../lib/dateFormat';
import { makeUser, makeVisit, makeVisitList } from '../test/fixtures';
import { renderApp } from '../test/render';

describe('VisitsPage', () => {
  const dispatcher = makeUser({ id: 2, role: 'dispatcher', first_name: 'Dee', last_name: 'Patch' });

  beforeEach(() => {
    vi.spyOn(apiClient, 'getUsers').mockResolvedValue({
      users: [
        makeUser({ id: 3, first_name: 'Tess', last_name: 'Turner' }),
        makeUser({ id: 4, first_name: 'Omar', last_name: 'Bell' }),
      ],
      total: 2,
      page: 1,
      page_size: 20,
      total_pages: 1,
    });
  });

  it('fetches today (London day) to today by default, server-side', async () => {
    const getVisits = vi.spyOn(apiClient, 'getVisits').mockResolvedValue(makeVisitList());
    renderApp('/visits', dispatcher);

    await screen.findByText('Riverside Depot');
    const today = todayUK();
    expect(getVisits).toHaveBeenCalledTimes(1);
    expect(getVisits).toHaveBeenCalledWith(
      expect.objectContaining({ from: today, to: today, page: 1, page_size: 25 }),
    );
    expect(getVisits.mock.calls[0][0]).not.toHaveProperty('status', expect.anything());
  });

  it('refetches with the new range when the date inputs change', async () => {
    const getVisits = vi.spyOn(apiClient, 'getVisits').mockResolvedValue(makeVisitList());
    renderApp('/visits', dispatcher);
    await screen.findByText('Riverside Depot');

    fireEvent.change(screen.getByLabelText('From'), { target: { value: '2026-09-01' } });
    await waitFor(() =>
      expect(getVisits).toHaveBeenLastCalledWith(expect.objectContaining({ from: '2026-09-01', to: todayUK() })),
    );

    fireEvent.change(screen.getByLabelText('To'), { target: { value: '2026-09-30' } });
    await waitFor(() =>
      expect(getVisits).toHaveBeenLastCalledWith(expect.objectContaining({ from: '2026-09-01', to: '2026-09-30' })),
    );
    expect(getVisits).toHaveBeenCalledTimes(3);
  });

  it('pushes status and technician filters into the query and resets to page 1', async () => {
    const getVisits = vi
      .spyOn(apiClient, 'getVisits')
      .mockResolvedValue(makeVisitList({ page: 2, page_size: 25, total_pages: 3, total: 60 }));
    renderApp('/visits?page=2', dispatcher);
    await screen.findByText('Riverside Depot');
    expect(getVisits).toHaveBeenLastCalledWith(expect.objectContaining({ page: 2 }));

    await screen.findByRole('option', { name: 'Omar Bell' });
    fireEvent.change(screen.getByLabelText('Technician'), { target: { value: '4' } });
    await waitFor(() =>
      expect(getVisits).toHaveBeenLastCalledWith(expect.objectContaining({ technician_id: 4, page: 1 })),
    );

    fireEvent.change(screen.getByLabelText('Status'), { target: { value: 'completed' } });
    await waitFor(() =>
      expect(getVisits).toHaveBeenLastCalledWith(
        expect.objectContaining({ technician_id: 4, status: 'completed', page: 1 }),
      ),
    );
  });

  it('renders the pager from the echoed page_size and total_pages', async () => {
    vi.spyOn(apiClient, 'getVisits').mockResolvedValue(
      makeVisitList({ page: 2, page_size: 10, total_pages: 4, total: 37 }),
    );
    renderApp('/visits?page=2', dispatcher);

    await screen.findByText('Riverside Depot');
    expect(screen.getByText('Page 2 of 4 · 10 per page · 37 visits')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Previous' })).toBeEnabled();
    expect(screen.getByRole('button', { name: 'Next' })).toBeEnabled();
  });

  it('shows "Unassigned" and London times in the list', async () => {
    vi.spyOn(apiClient, 'getVisits').mockResolvedValue(
      makeVisitList({
        visits: [makeVisit({ technician_id: null, technician: null, status: 'in_progress' })],
      }),
    );
    renderApp('/visits', dispatcher);

    await screen.findByText('Riverside Depot');
    expect(screen.getByText('Unassigned')).toBeInTheDocument();
    expect(screen.getByText('21 Sep 2026, 09:00 – 11:00')).toBeInTheDocument();
    expect(screen.getByText('In progress')).toBeInTheDocument();
  });

  // rule: §3.7 — each visit's priority is shown in the list
  it('shows each visit\'s priority in the list', async () => {
    vi.spyOn(apiClient, 'getVisits').mockResolvedValue(
      makeVisitList({
        visits: [makeVisit({ id: 7, priority: 'high' }), makeVisit({ id: 8, priority: 'low' })],
        total: 2,
      }),
    );
    renderApp('/visits', dispatcher);

    await screen.findAllByText('Riverside Depot');
    expect(screen.getByRole('columnheader', { name: 'Priority' })).toBeInTheDocument();
    expect(screen.getByRole('cell', { name: 'High' })).toBeInTheDocument();
    expect(screen.getByRole('cell', { name: 'Low' })).toBeInTheDocument();
  });

  it('hides the technician filter and the report link from technicians', async () => {
    vi.spyOn(apiClient, 'getVisits').mockResolvedValue(makeVisitList());
    renderApp('/visits', makeUser({ role: 'technician' }));

    await screen.findByText('Riverside Depot');
    expect(screen.queryByLabelText('Technician')).not.toBeInTheDocument();
    expect(screen.queryByRole('link', { name: 'Daily report' })).not.toBeInTheDocument();
    expect(apiClient.getUsers).not.toHaveBeenCalled();
  });

  it('shows the server error inline when the list fails', async () => {
    vi.spyOn(apiClient, 'getVisits').mockRejectedValue(new Error('from and to are required'));
    renderApp('/visits', dispatcher);

    expect(await screen.findByRole('alert')).toHaveTextContent('from and to are required');
  });
});
