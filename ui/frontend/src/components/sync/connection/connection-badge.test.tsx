import { describe, it, expect } from 'vitest';
import { connectionBadgeState } from './connection-badge-state';

describe('connectionBadgeState', () => {
  it('reports "Disabled" with highest precedence', () => {
    const { label } = connectionBadgeState({
      isConfigured: true,
      credentialsError: true,
      disabled: true,
    });
    expect(label).toBe('Disabled');
  });

  it('reports "Credentials Error" over not-configured and connected', () => {
    const { label } = connectionBadgeState({ isConfigured: false, credentialsError: true });
    expect(label).toBe('Credentials Error');
  });

  it('reports "Not Configured" when not configured and no error', () => {
    const { label } = connectionBadgeState({ isConfigured: false });
    expect(label).toBe('Not Configured');
  });

  it('reports "Connected" when configured with no error', () => {
    const { label } = connectionBadgeState({ isConfigured: true });
    expect(label).toBe('Connected');
  });
});
