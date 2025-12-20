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

  it('should render section title with total and matched counts', () => {
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
    // 2 total items, 1 auto-matched (PC), 1 needs attention (PS4)
    expect(screen.getByText(/2 total, 1 matched/)).toBeInTheDocument();
    expect(screen.getByText(/1 need attention/)).toBeInTheDocument();
  });

  it('should render all items when all are auto-matched', () => {
    const resolvedItems: PlatformMappingSuggestion[] = [
      { original: 'PC', count: 15, suggestedId: 'pc-windows', suggestedName: 'PC (Windows)' },
    ];

    render(
      <MappingSection
        title="Platforms"
        items={resolvedItems}
        options={mockOptions}
        mappings={{}}
        onMappingChange={vi.fn()}
      />
    );

    // Should still render with all items shown
    expect(screen.getByText('Platforms')).toBeInTheDocument();
    expect(screen.getByText('"PC"')).toBeInTheDocument();
    // Count should show 1 total, 1 matched, no need attention
    expect(screen.getByText(/1 total, 1 matched/)).toBeInTheDocument();
    expect(screen.queryByText(/need attention/)).not.toBeInTheDocument();
  });

  it('should not render when items array is empty', () => {
    const { container } = render(
      <MappingSection
        title="Platforms"
        items={[]}
        options={mockOptions}
        mappings={{}}
        onMappingChange={vi.fn()}
      />
    );

    expect(container).toBeEmptyDOMElement();
  });

  it('should display all items with dropdowns', () => {
    render(
      <MappingSection
        title="Platforms"
        items={mockItems}
        options={mockOptions}
        mappings={{}}
        onMappingChange={vi.fn()}
      />
    );

    // Both items should be shown
    expect(screen.getByText('"PC"')).toBeInTheDocument();
    expect(screen.getByText('15 games')).toBeInTheDocument();
    expect(screen.getByText('"PS4"')).toBeInTheDocument();
    expect(screen.getByText('8 games')).toBeInTheDocument();
  });

  it('should show auto-matched badge for resolved items', () => {
    render(
      <MappingSection
        title="Platforms"
        items={mockItems}
        options={mockOptions}
        mappings={{}}
        onMappingChange={vi.fn()}
      />
    );

    // PC has suggestedId so should show "Auto-matched" badge
    expect(screen.getByText('Auto-matched')).toBeInTheDocument();
    // PS4 has no suggestedId so should show "Needs mapping" badge
    expect(screen.getByText('Needs mapping')).toBeInTheDocument();
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

    // Get all comboboxes - there should be 2 (PC and PS4)
    const comboboxes = screen.getAllByRole('combobox');
    expect(comboboxes).toHaveLength(2);

    // Click the PS4 select trigger (second one)
    await user.click(comboboxes[1]);

    // Select an option
    await user.click(screen.getByText('PlayStation 4'));

    expect(onMappingChange).toHaveBeenCalledWith('PS4', 'playstation-4');
  });

  it('should show selected value in dropdown for manually mapped items', () => {
    render(
      <MappingSection
        title="Platforms"
        items={mockItems}
        options={mockOptions}
        mappings={{ PS4: 'playstation-4' }}
        onMappingChange={vi.fn()}
      />
    );

    // PS4's dropdown should show PlayStation 4
    expect(screen.getByText('PlayStation 4')).toBeInTheDocument();
  });

  it('should pre-select auto-matched value in dropdown', () => {
    render(
      <MappingSection
        title="Platforms"
        items={mockItems}
        options={mockOptions}
        mappings={{}}
        onMappingChange={vi.fn()}
      />
    );

    // PC's dropdown should show PC (Windows) (the auto-matched value)
    expect(screen.getByText('PC (Windows)')).toBeInTheDocument();
  });

  it('should apply green styling to resolved items and orange to unresolved', () => {
    render(
      <MappingSection
        title="Platforms"
        items={mockItems}
        options={mockOptions}
        mappings={{}}
        onMappingChange={vi.fn()}
      />
    );

    // Find the item containers
    const itemContainers = screen.getAllByText(/games/).map((el) =>
      el.closest('.flex.items-center.justify-between')
    );

    // PC (resolved) should have green styling
    expect(itemContainers[0]).toHaveClass('border-green-200');
    expect(itemContainers[0]).toHaveClass('bg-green-50/50');

    // PS4 (unresolved) should have orange styling
    expect(itemContainers[1]).toHaveClass('border-orange-200');
    expect(itemContainers[1]).toHaveClass('bg-orange-50/50');
  });
});
