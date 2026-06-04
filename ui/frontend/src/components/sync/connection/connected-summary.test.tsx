import { render, screen } from '@testing-library/react';
import { describe, it, expect } from 'vitest';
import { ConnectedSummary } from './connected-summary';

describe('ConnectedSummary', () => {
  it('renders "Connected as {name}" when a name is given', () => {
    render(<ConnectedSummary name="alice" />);
    expect(screen.getByText('Connected as alice')).toBeInTheDocument();
  });

  it('renders just "Connected" when no name is given', () => {
    render(<ConnectedSummary />);
    expect(screen.getByText('Connected')).toBeInTheDocument();
  });
});
