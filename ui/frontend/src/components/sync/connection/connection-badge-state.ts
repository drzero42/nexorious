export interface ConnectionBadgeProps {
  isConfigured: boolean;
  credentialsError?: boolean;
  disabled?: boolean;
}

/**
 * Resolves the status badge label + className from a connection's flags.
 * Precedence: disabled → credentials error → not configured → connected.
 */
export function connectionBadgeState({
  isConfigured,
  credentialsError = false,
  disabled = false,
}: ConnectionBadgeProps) {
  if (disabled) {
    return { label: 'Disabled', className: 'bg-muted text-muted-foreground' };
  }
  if (credentialsError) {
    return {
      label: 'Credentials Error',
      className: 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-400',
    };
  }
  if (!isConfigured) {
    return { label: 'Not Configured', className: 'bg-muted text-muted-foreground' };
  }
  return {
    label: 'Connected',
    className: 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400',
  };
}
