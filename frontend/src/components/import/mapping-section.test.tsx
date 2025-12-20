import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MappingSection } from './mapping-section';
import type { PlatformMappingSuggestion } from '@/types';

describe('MappingSection', () => {
  const mockItems: PlatformMappingSuggestion[] = [
    { original: 'PC', count: 15, suggestedId: 'pc-windows', suggestedName: 'PC (Windows)' },
    { original: 'PS4', count: 8, suggestedId: null, suggestedName: null },
  ];

  const mockOptions = [
    { id: 'pc-windows', display_name: 'PC (Windows)' },
    { id: 'pc-linux', display_name: 'PC (Linux)' },
    { id: 'playstation-4', display_name: 'PlayStation 4' },
  ];

  it('should render section title and count', () => {
    render(
      <MappingSection
        title="Platforms"
        items={mockItems}
        options={mockOptions}
        mappings={{}}
        onMappingChange={vi.fn()}
      />
    );

    expect(screen.getByText('Platforms')).toBeInTheDocument();
    expect(screen.getByText('(1 unresolved)')).toBeInTheDocument();
  });

  it('should not render when no unresolved items', () => {
    const resolvedItems: PlatformMappingSuggestion[] = [
      { original: 'PC', count: 15, suggestedId: 'pc-windows', suggestedName: 'PC (Windows)' },
    ];

    const { container } = render(
      <MappingSection
        title="Platforms"
        items={resolvedItems}
        options={mockOptions}
        mappings={{}}
        onMappingChange={vi.fn()}
      />
    );

    expect(container).toBeEmptyDOMElement();
  });

  it('should display unresolved items with dropdowns', () => {
    render(
      <MappingSection
        title="Platforms"
        items={mockItems}
        options={mockOptions}
        mappings={{}}
        onMappingChange={vi.fn()}
      />
    );

    // Only unresolved item (PS4) should be shown
    expect(screen.getByText('"PS4"')).toBeInTheDocument();
    expect(screen.getByText('8 games')).toBeInTheDocument();
    expect(screen.queryByText('"PC"')).not.toBeInTheDocument();
  });

  it('should call onMappingChange when selection is made', async () => {
    const user = userEvent.setup();
    const onMappingChange = vi.fn();

    render(
      <MappingSection
        title="Platforms"
        items={mockItems}
        options={mockOptions}
        mappings={{}}
        onMappingChange={onMappingChange}
      />
    );

    // Click the select trigger
    await user.click(screen.getByRole('combobox'));

    // Select an option
    await user.click(screen.getByText('PlayStation 4'));

    expect(onMappingChange).toHaveBeenCalledWith('PS4', 'playstation-4');
  });

  it('should show selected value in dropdown', () => {
    render(
      <MappingSection
        title="Platforms"
        items={mockItems}
        options={mockOptions}
        mappings={{ PS4: 'playstation-4' }}
        onMappingChange={vi.fn()}
      />
    );

    expect(screen.getByText('PlayStation 4')).toBeInTheDocument();
  });
});
