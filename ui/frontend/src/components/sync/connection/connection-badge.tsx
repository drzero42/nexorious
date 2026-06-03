import { Badge } from '@/components/ui/badge';
import { connectionBadgeState, type ConnectionBadgeProps } from './connection-badge-state';

export function ConnectionBadge(props: ConnectionBadgeProps) {
  const { label, className } = connectionBadgeState(props);
  return (
    <Badge variant="outline" className={className}>
      {label}
    </Badge>
  );
}
