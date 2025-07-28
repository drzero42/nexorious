import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, fireEvent, screen } from '@testing-library/svelte';
import { PlayStatus } from '$lib/stores/user-games.svelte';
import PlayStatusDropdown from './PlayStatusDropdown.svelte';

describe('PlayStatusDropdown', () => {
	const defaultProps = {
		value: PlayStatus.NOT_STARTED,
		disabled: false,
		class: '',
		id: 'test-dropdown',
		name: 'play-status'
	};

	beforeEach(() => {
		vi.clearAllMocks();
	});

	describe('Rendering', () => {
		it('should render with default props', () => {
			render(PlayStatusDropdown, { props: defaultProps });
			
			const select = screen.getByRole('combobox');
			expect(select).toBeInTheDocument();
			expect(select).toHaveAttribute('id', 'test-dropdown');
			expect(select).toHaveAttribute('name', 'play-status');
			expect(select).not.toBeDisabled();
		});

		it('should render all status options', () => {
			render(PlayStatusDropdown, { props: defaultProps });
			
			const options = screen.getAllByRole('option');
			expect(options).toHaveLength(8);
			
			// Check that all expected options are present
			const expectedLabels = [
				'Not Started',
				'In Progress',
				'Completed',
				'Mastered',
				'Dominated',
				'Shelved',
				'Dropped',
				'Replay'
			];
			
			expectedLabels.forEach(label => {
				expect(screen.getByRole('option', { name: label })).toBeInTheDocument();
			});
		});

		it('should show selected option as current value', () => {
			render(PlayStatusDropdown, { 
				props: { ...defaultProps, value: PlayStatus.IN_PROGRESS } 
			});
			
			const select = screen.getByRole('combobox') as HTMLSelectElement;
			expect(select.value).toBe(PlayStatus.IN_PROGRESS);
		});

		it('should apply custom CSS class', () => {
			render(PlayStatusDropdown, { 
				props: { ...defaultProps, class: 'custom-class' } 
			});
			
			const container = screen.getByRole('combobox').closest('.custom-class');
			expect(container).toBeInTheDocument();
		});

		it('should be disabled when disabled prop is true', () => {
			render(PlayStatusDropdown, { 
				props: { ...defaultProps, disabled: true } 
			});
			
			const select = screen.getByRole('combobox');
			expect(select).toBeDisabled();
		});
	});

	describe('Status Badge and Description', () => {
		it('should display status badge for not_started', () => {
			render(PlayStatusDropdown, { 
				props: { ...defaultProps, value: PlayStatus.NOT_STARTED } 
			});
			
			// Check for the badge specifically (not the option text)
			const badge = screen.getByText('Not Started', { selector: 'span' });
			expect(badge).toBeInTheDocument();
			expect(screen.getByText("Haven't begun playing")).toBeInTheDocument();
		});

		it('should display status badge for in_progress', () => {
			render(PlayStatusDropdown, { 
				props: { ...defaultProps, value: PlayStatus.IN_PROGRESS } 
			});
			
			const badge = screen.getByText('In Progress', { selector: 'span' });
			expect(badge).toBeInTheDocument();
			expect(screen.getByText('Currently playing')).toBeInTheDocument();
		});

		it('should display status badge for completed', () => {
			render(PlayStatusDropdown, { 
				props: { ...defaultProps, value: PlayStatus.COMPLETED } 
			});
			
			const badge = screen.getByText('Completed', { selector: 'span' });
			expect(badge).toBeInTheDocument();
			expect(screen.getByText('Finished main story/campaign')).toBeInTheDocument();
		});

		it('should display status badge for mastered', () => {
			render(PlayStatusDropdown, { 
				props: { ...defaultProps, value: PlayStatus.MASTERED } 
			});
			
			const badge = screen.getByText('Mastered', { selector: 'span' });
			expect(badge).toBeInTheDocument();
			expect(screen.getByText('Completed main story plus all side quests and content')).toBeInTheDocument();
		});

		it('should display status badge for dominated', () => {
			render(PlayStatusDropdown, { 
				props: { ...defaultProps, value: PlayStatus.DOMINATED } 
			});
			
			const badge = screen.getByText('Dominated', { selector: 'span' });
			expect(badge).toBeInTheDocument();
			expect(screen.getByText('100% completion including all achievements/trophies')).toBeInTheDocument();
		});

		it('should display status badge for shelved', () => {
			render(PlayStatusDropdown, { 
				props: { ...defaultProps, value: PlayStatus.SHELVED } 
			});
			
			const badge = screen.getByText('Shelved', { selector: 'span' });
			expect(badge).toBeInTheDocument();
			expect(screen.getByText('Temporarily paused with intent to return')).toBeInTheDocument();
		});

		it('should display status badge for dropped', () => {
			render(PlayStatusDropdown, { 
				props: { ...defaultProps, value: PlayStatus.DROPPED } 
			});
			
			const badge = screen.getByText('Dropped', { selector: 'span' });
			expect(badge).toBeInTheDocument();
			expect(screen.getByText('Permanently abandoned')).toBeInTheDocument();
		});

		it('should display status badge for replay', () => {
			render(PlayStatusDropdown, { 
				props: { ...defaultProps, value: PlayStatus.REPLAY } 
			});
			
			const badge = screen.getByText('Replay', { selector: 'span' });
			expect(badge).toBeInTheDocument();
			expect(screen.getByText('Playing again after previous completion')).toBeInTheDocument();
		});
	});

	describe('Interactions', () => {
		it('should update value when different option is selected', async () => {
			render(PlayStatusDropdown, { props: defaultProps });
			
			const select = screen.getByRole('combobox') as HTMLSelectElement;
			expect(select.value).toBe(PlayStatus.NOT_STARTED);
			
			await fireEvent.change(select, { target: { value: PlayStatus.IN_PROGRESS } });
			expect(select.value).toBe(PlayStatus.IN_PROGRESS);
		});

		it('should be disabled when disabled prop is true', async () => {
			render(PlayStatusDropdown, { 
				props: { ...defaultProps, disabled: true } 
			});
			
			const select = screen.getByRole('combobox');
			expect(select).toBeDisabled();
			
			// Should not be able to change value when disabled
			await fireEvent.change(select, { target: { value: PlayStatus.COMPLETED } });
			expect(select).toBeDisabled();
		});

		it('should handle select changes without crashing', async () => {
			render(PlayStatusDropdown, { props: defaultProps });
			
			const select = screen.getByRole('combobox');
			
			// Test changing through multiple values
			await fireEvent.change(select, { target: { value: PlayStatus.COMPLETED } });
			await fireEvent.change(select, { target: { value: PlayStatus.SHELVED } });
			await fireEvent.change(select, { target: { value: PlayStatus.NOT_STARTED } });
			
			// Should not crash and should show final value
			expect((select as HTMLSelectElement).value).toBe(PlayStatus.NOT_STARTED);
		});
	});

	describe('Visual Elements', () => {
		it('should render dropdown arrow icon', () => {
			render(PlayStatusDropdown, { props: defaultProps });
			
			// Check for SVG dropdown arrow
			const arrow = screen.getByRole('combobox').nextElementSibling?.querySelector('svg');
			expect(arrow).toBeInTheDocument();
		});

		it('should show tooltip with selected option description', () => {
			render(PlayStatusDropdown, { 
				props: { ...defaultProps, value: PlayStatus.COMPLETED } 
			});
			
			const select = screen.getByRole('combobox');
			expect(select).toHaveAttribute('title', 'Finished main story/campaign');
		});

		it('should apply status-specific colors to badge', () => {
			// Test different statuses individually instead of rerendering
			const { unmount } = render(PlayStatusDropdown, { 
				props: { ...defaultProps, value: PlayStatus.NOT_STARTED } 
			});
			
			// Check for gray styling for not_started
			let badge = screen.getByText('Not Started', { selector: 'span' });
			expect(badge).toHaveClass('text-gray-600', 'bg-gray-100');
			unmount();
			
			// Test in_progress
			const { unmount: unmount2 } = render(PlayStatusDropdown, { 
				props: { ...defaultProps, value: PlayStatus.IN_PROGRESS } 
			});
			badge = screen.getByText('In Progress', { selector: 'span' });
			expect(badge).toHaveClass('text-blue-600', 'bg-blue-100');
			unmount2();
			
			// Test completed
			render(PlayStatusDropdown, { 
				props: { ...defaultProps, value: PlayStatus.COMPLETED } 
			});
			badge = screen.getByText('Completed', { selector: 'span' });
			expect(badge).toHaveClass('text-green-600', 'bg-green-100');
		});
	});

	describe('Edge Cases', () => {
		it('should handle undefined value gracefully', () => {
			// Test with undefined as value (shouldn't crash)
			expect(() => {
				render(PlayStatusDropdown, { 
					props: { ...defaultProps, value: undefined as any } 
				});
			}).not.toThrow();
		});

		it('should handle empty string value', () => {
			expect(() => {
				render(PlayStatusDropdown, { 
					props: { ...defaultProps, value: '' as any } 
				});
			}).not.toThrow();
		});

		it('should render without optional props', () => {
			render(PlayStatusDropdown, { 
				props: { value: PlayStatus.NOT_STARTED } 
			});
			
			const select = screen.getByRole('combobox');
			expect(select).toBeInTheDocument();
			expect(select).not.toBeDisabled();
		});
	});

	describe('Accessibility', () => {
		it('should have proper ARIA attributes', () => {
			render(PlayStatusDropdown, { props: defaultProps });
			
			const select = screen.getByRole('combobox');
			expect(select).toHaveAttribute('id', 'test-dropdown');
			expect(select).toHaveAttribute('name', 'play-status');
		});

		it('should be keyboard navigable', async () => {
			render(PlayStatusDropdown, { props: defaultProps });
			
			const select = screen.getByRole('combobox');
			
			// Focus the select
			select.focus();
			expect(document.activeElement).toBe(select);
			
			// Should be able to change value with keyboard
			await fireEvent.keyDown(select, { key: 'ArrowDown' });
			// Note: Full keyboard navigation testing would require more complex setup
		});

		it('should have descriptive help text', () => {
			render(PlayStatusDropdown, { 
				props: { ...defaultProps, value: PlayStatus.MASTERED } 
			});
			
			const helpText = screen.getByText('Completed main story plus all side quests and content');
			expect(helpText).toBeInTheDocument();
			expect(helpText).toHaveClass('text-xs', 'text-gray-500');
		});
	});

	describe('Value Binding', () => {
		it('should display correct value when initialized', async () => {
			const { unmount } = render(PlayStatusDropdown, { 
				props: { 
					...defaultProps, 
					value: PlayStatus.NOT_STARTED 
				} 
			});
			
			let select = screen.getByRole('combobox') as HTMLSelectElement;
			expect(select.value).toBe(PlayStatus.NOT_STARTED);
			unmount();
			
			// Test with different initial value
			render(PlayStatusDropdown, { 
				props: { 
					...defaultProps, 
					value: PlayStatus.COMPLETED 
				} 
			});
			
			select = screen.getByRole('combobox') as HTMLSelectElement;
			expect(select.value).toBe(PlayStatus.COMPLETED);
		});
	});

	describe('Status Option Configuration', () => {
		it('should have correct value mappings for all status options', () => {
			render(PlayStatusDropdown, { props: defaultProps });
			
			const expectedMappings = [
				{ value: PlayStatus.NOT_STARTED, label: 'Not Started' },
				{ value: PlayStatus.IN_PROGRESS, label: 'In Progress' },
				{ value: PlayStatus.COMPLETED, label: 'Completed' },
				{ value: PlayStatus.MASTERED, label: 'Mastered' },
				{ value: PlayStatus.DOMINATED, label: 'Dominated' },
				{ value: PlayStatus.SHELVED, label: 'Shelved' },
				{ value: PlayStatus.DROPPED, label: 'Dropped' },
				{ value: PlayStatus.REPLAY, label: 'Replay' }
			];
			
			expectedMappings.forEach(({ value, label }) => {
				const option = screen.getByRole('option', { name: label }) as HTMLOptionElement;
				expect(option.value).toBe(value);
			});
		});

		it('should have unique colors for different statuses', () => {
			const statusesToTest = [
				{ value: PlayStatus.NOT_STARTED, expectedClass: 'text-gray-600', label: 'Not Started' },
				{ value: PlayStatus.IN_PROGRESS, expectedClass: 'text-blue-600', label: 'In Progress' },
				{ value: PlayStatus.COMPLETED, expectedClass: 'text-green-600', label: 'Completed' },
				{ value: PlayStatus.MASTERED, expectedClass: 'text-purple-600', label: 'Mastered' },
				{ value: PlayStatus.DOMINATED, expectedClass: 'text-yellow-600', label: 'Dominated' },
				{ value: PlayStatus.SHELVED, expectedClass: 'text-orange-600', label: 'Shelved' },
				{ value: PlayStatus.DROPPED, expectedClass: 'text-red-600', label: 'Dropped' },
				{ value: PlayStatus.REPLAY, expectedClass: 'text-indigo-600', label: 'Replay' }
			];
			
			statusesToTest.forEach(({ value, expectedClass, label }) => {
				const { unmount } = render(PlayStatusDropdown, { 
					props: { ...defaultProps, value } 
				});
				
				const badge = screen.getByText(label, { selector: 'span' });
				expect(badge).toHaveClass(expectedClass);
				
				unmount();
			});
		});
	});
});