export type Role = 'admin' | 'dispatcher' | 'technician';

export interface User {
  id: number;
  email: string;
  first_name: string;
  last_name: string;
  role: Role;
  is_active: boolean;
  created_at: string;
}

export interface Site {
  id: number;
  name: string;
  address: string;
  contact_phone: string;
}

export type VisitStatus = 'scheduled' | 'in_progress' | 'completed' | 'cancelled';

export type VisitPriority = 'low' | 'normal' | 'high';

export type ClockKind = 'in' | 'out';

export interface PersonRef {
  id: number;
  first_name: string;
  last_name: string;
}

export interface ClockEvent {
  id: number;
  kind: ClockKind;
  occurred_at: string;
  recorded_by_id: number;
  recorded_by: PersonRef | null;
}

export interface Visit {
  id: number;
  site_id: number;
  site: { id: number; name: string };
  technician_id: number | null;
  technician: PersonRef | null;
  scheduled_start: string;
  scheduled_end: string;
  status: VisitStatus;
  priority: VisitPriority;
  cancelled_by_id: number | null;
  cancelled_at: string | null;
  clock_events: ClockEvent[];
}

export interface TechnicianDay {
  technician_id: number;
  name: string;
  visits: number;
  completed: number;
  hours_clocked: number;
}

export interface DailyReport {
  date: string;
  scheduled: number;
  completed: number;
  cancelled: number;
  technicians: TechnicianDay[];
}

export interface LoginResponse {
  token: string;
  user: User;
}

export interface VisitListParams {
  from: string;
  to: string;
  technician_id?: number;
  status?: VisitStatus;
  page?: number;
  page_size?: number;
}

export interface VisitListResponse {
  visits: Visit[];
  total: number;
  page: number;
  page_size: number;
  total_pages: number;
}

export interface UserListResponse {
  users: User[];
  total: number;
  page: number;
  page_size: number;
  total_pages: number;
}

export interface SiteListResponse {
  sites: Site[];
  total: number;
  page: number;
  page_size: number;
  total_pages: number;
}

export interface UserListParams {
  role?: Role;
  active?: boolean;
}
