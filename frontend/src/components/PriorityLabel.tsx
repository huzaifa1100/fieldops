import type { VisitPriority } from '../types';

const LABELS: Record<VisitPriority, string> = {
  low: 'Low',
  normal: 'Normal',
  high: 'High',
};

export function PriorityLabel({ priority }: { priority: VisitPriority }) {
  return <span>{LABELS[priority]}</span>;
}
