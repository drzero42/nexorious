import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import TimeTrackingInput from './TimeTrackingInput.svelte';

describe('TimeTrackingInput Component', () => {
	const defaultProps = {
		value: 2.5,
		id: 'test-time-input',
		name: 'hours_played'
	};

	beforeEach(() => {
		vi.clearAllMocks();
	});

	describe('Basic Rendering', () => {
		it('should render with default simple mode', () => {
			render(TimeTrackingInput, { props: defaultProps });
			
			// Should show mode toggle buttons
			expect(screen.getByText('Simple')).toBeInTheDocument();
			expect(screen.getByText('Detailed')).toBeInTheDocument();
			
			// Should show current value display
			expect(screen.getByText(/Total: 2\.5h/)).toBeInTheDocument();
		});

		it('should render with correct input attributes', () => {
			render(TimeTrackingInput, { props: defaultProps });
			
			const input = screen.getByDisplayValue('2.5');
			expect(input).toHaveAttribute('id', 'test-time-input');
			expect(input).toHaveAttribute('name', 'hours_played');
			expect(input).toHaveAttribute('type', 'number');
			expect(input).toHaveAttribute('min', '0');
			expect(input).toHaveAttribute('step', '0.25');
		});

		it('should render with custom placeholder', () => {
			render(TimeTrackingInput, { 
				props: { 
					...defaultProps, 
					placeholder: 'Custom placeholder' 
				} 
			});
			
			const input = screen.getByPlaceholderText('Custom placeholder');
			expect(input).toBeInTheDocument();
		});

		it('should render disabled when disabled prop is true', () => {
			render(TimeTrackingInput, { 
				props: { 
					...defaultProps, 
					disabled: true 
				} 
			});
			
			const input = screen.getByDisplayValue('2.5');
			expect(input).toBeDisabled();
			
			// Mode toggle buttons should also be disabled
			const simpleButton = screen.getByText('Simple');
			const detailedButton = screen.getByText('Detailed');
			expect(simpleButton).toBeDisabled();
			expect(detailedButton).toBeDisabled();
		});
	});

	describe('Simple Mode', () => {
		it('should show number input in simple mode', () => {
			render(TimeTrackingInput, { props: defaultProps });
			
			const input = screen.getByDisplayValue('2.5');
			expect(input).toHaveAttribute('type', 'number');
			expect(screen.getByText('hours')).toBeInTheDocument();
		});

		it('should update value when typing in simple mode', async () => {
			render(TimeTrackingInput, { props: defaultProps });
			
			const input = screen.getByDisplayValue('2.5');
			await fireEvent.input(input, { target: { value: '4.75' } });
			
			expect(input).toHaveValue(4.75);
		});
	});

	describe('Detailed Mode', () => {
		it('should switch to detailed mode when button is clicked', async () => {
			render(TimeTrackingInput, { props: defaultProps });
			
			const detailedButton = screen.getByText('Detailed');
			await fireEvent.click(detailedButton);
			
			// Should show hours and minutes inputs
			expect(screen.getByLabelText('Hours')).toBeInTheDocument();
			expect(screen.getByLabelText('Minutes')).toBeInTheDocument();
			
			// Should show increment/decrement buttons
			expect(screen.getByLabelText('Decrease hours')).toBeInTheDocument();
			expect(screen.getByLabelText('Increase hours')).toBeInTheDocument();
			expect(screen.getByLabelText('Decrease minutes')).toBeInTheDocument();
			expect(screen.getByLabelText('Increase minutes')).toBeInTheDocument();
		});

		it('should display correct hours and minutes values', async () => {
			render(TimeTrackingInput, { props: { ...defaultProps, value: 3.75 } });
			
			const detailedButton = screen.getByText('Detailed');
			await fireEvent.click(detailedButton);
			
			const hoursInput = screen.getByLabelText('Hours');
			const minutesInput = screen.getByLabelText('Minutes');
			
			expect(hoursInput).toHaveValue(3);
			expect(minutesInput).toHaveValue(45);
		});

		it('should increment hours when increment button is clicked', async () => {
			render(TimeTrackingInput, { props: defaultProps });
			
			const detailedButton = screen.getByText('Detailed');
			await fireEvent.click(detailedButton);
			
			const incrementButton = screen.getByLabelText('Increase hours');
			await fireEvent.click(incrementButton);
			
			const hoursInput = screen.getByLabelText('Hours');
			expect(hoursInput).toHaveValue(3);
			
			// Total should update
			expect(screen.getByText(/Total: 3\.5h/)).toBeInTheDocument();
		});

		it('should decrement hours when decrement button is clicked', async () => {
			render(TimeTrackingInput, { props: defaultProps });
			
			const detailedButton = screen.getByText('Detailed');
			await fireEvent.click(detailedButton);
			
			const decrementButton = screen.getByLabelText('Decrease hours');
			await fireEvent.click(decrementButton);
			
			const hoursInput = screen.getByLabelText('Hours');
			expect(hoursInput).toHaveValue(1);
			
			// Total should update
			expect(screen.getByText(/Total: 1\.5h/)).toBeInTheDocument();
		});

		it('should not allow hours to go below 0', async () => {
			render(TimeTrackingInput, { props: { ...defaultProps, value: 0 } });
			
			const detailedButton = screen.getByText('Detailed');
			await fireEvent.click(detailedButton);
			
			const decrementButton = screen.getByLabelText('Decrease hours');
			await fireEvent.click(decrementButton);
			
			const hoursInput = screen.getByLabelText('Hours');
			expect(hoursInput).toHaveValue(0);
		});

		it('should increment minutes by 15 when increment button is clicked', async () => {
			render(TimeTrackingInput, { props: { ...defaultProps, value: 2.25 } });
			
			const detailedButton = screen.getByText('Detailed');
			await fireEvent.click(detailedButton);
			
			const incrementButton = screen.getByLabelText('Increase minutes');
			await fireEvent.click(incrementButton);
			
			const minutesInput = screen.getByLabelText('Minutes');
			expect(minutesInput).toHaveValue(30);
			
			// Total should update
			expect(screen.getByText(/Total: 2\.5h/)).toBeInTheDocument();
		});

		it('should render correct input fields in detailed mode', async () => {
			render(TimeTrackingInput, { props: defaultProps });
			
			const detailedButton = screen.getByText('Detailed');
			await fireEvent.click(detailedButton);
			
			const hoursInput = screen.getByLabelText('Hours');
			const minutesInput = screen.getByLabelText('Minutes');
			
			// Should start with correct values from props
			expect(hoursInput).toHaveValue(2);
			expect(minutesInput).toHaveValue(30);
			expect(hoursInput).toHaveAttribute('type', 'number');
			expect(minutesInput).toHaveAttribute('type', 'number');
		});
	});

	describe('Quick Add Buttons', () => {
		it('should show quick add buttons in detailed mode', async () => {
			render(TimeTrackingInput, { props: defaultProps });
			
			const detailedButton = screen.getByText('Detailed');
			await fireEvent.click(detailedButton);
			
			expect(screen.getByText('Quick Add')).toBeInTheDocument();
			expect(screen.getByText('+30m')).toBeInTheDocument();
			expect(screen.getByText('+1h')).toBeInTheDocument();
			expect(screen.getByText('+2h')).toBeInTheDocument();
			expect(screen.getByText('+5h')).toBeInTheDocument();
		});

		it('should add 30 minutes when +30m button is clicked', async () => {
			render(TimeTrackingInput, { props: defaultProps });
			
			const detailedButton = screen.getByText('Detailed');
			await fireEvent.click(detailedButton);
			
			const addButton = screen.getByText('+30m');
			await fireEvent.click(addButton);
			
			expect(screen.getByText(/Total: 3h/)).toBeInTheDocument();
		});

		it('should add 1 hour when +1h button is clicked', async () => {
			render(TimeTrackingInput, { props: defaultProps });
			
			const detailedButton = screen.getByText('Detailed');
			await fireEvent.click(detailedButton);
			
			const addButton = screen.getByText('+1h');
			await fireEvent.click(addButton);
			
			expect(screen.getByText(/Total: 3\.5h/)).toBeInTheDocument();
		});
	});

	describe('Value Display', () => {
		it('should display total time in decimal and hours:minutes format', () => {
			render(TimeTrackingInput, { props: { ...defaultProps, value: 3.25 } });
			
			expect(screen.getByText(/Total: 3\.25h/)).toBeInTheDocument();
		});

		it('should handle zero values correctly', () => {
			render(TimeTrackingInput, { props: { ...defaultProps, value: 0 } });
			
			expect(screen.getByText(/Total: 0h/)).toBeInTheDocument();
		});

		it('should handle fractional hours correctly', () => {
			render(TimeTrackingInput, { props: { ...defaultProps, value: 1.75 } });
			
			expect(screen.getByText(/Total: 1\.75h/)).toBeInTheDocument();
		});
	});

	describe('Accessibility', () => {
		it('should have proper labels for input fields', async () => {
			render(TimeTrackingInput, { props: defaultProps });
			
			const detailedButton = screen.getByText('Detailed');
			await fireEvent.click(detailedButton);
			
			expect(screen.getByLabelText('Hours')).toBeInTheDocument();
			expect(screen.getByLabelText('Minutes')).toBeInTheDocument();
		});

		it('should have proper aria-labels for buttons', async () => {
			render(TimeTrackingInput, { props: defaultProps });
			
			const detailedButton = screen.getByText('Detailed');
			await fireEvent.click(detailedButton);
			
			expect(screen.getByLabelText('Decrease hours')).toBeInTheDocument();
			expect(screen.getByLabelText('Increase hours')).toBeInTheDocument();
			expect(screen.getByLabelText('Decrease minutes')).toBeInTheDocument();
			expect(screen.getByLabelText('Increase minutes')).toBeInTheDocument();
		});

		it('should have proper input attributes for hours field', async () => {
			render(TimeTrackingInput, { props: defaultProps });
			
			const detailedButton = screen.getByText('Detailed');
			await fireEvent.click(detailedButton);
			
			const hoursInput = screen.getByLabelText('Hours');
			expect(hoursInput).toHaveAttribute('id', 'hours-input');
			expect(hoursInput).toHaveAttribute('type', 'number');
			expect(hoursInput).toHaveAttribute('min', '0');
		});

		it('should have proper input attributes for minutes field', async () => {
			render(TimeTrackingInput, { props: defaultProps });
			
			const detailedButton = screen.getByText('Detailed');
			await fireEvent.click(detailedButton);
			
			const minutesInput = screen.getByLabelText('Minutes');
			expect(minutesInput).toHaveAttribute('id', 'minutes-input');
			expect(minutesInput).toHaveAttribute('type', 'number');
			expect(minutesInput).toHaveAttribute('min', '0');
			expect(minutesInput).toHaveAttribute('max', '59');
		});
	});

	describe('Edge Cases', () => {
		it('should handle very large numbers gracefully', () => {
			render(TimeTrackingInput, { props: { ...defaultProps, value: 1000.5 } });
			
			expect(screen.getByText(/Total: 1000\.5h/)).toBeInTheDocument();
		});

		it('should handle non-numeric input gracefully', async () => {
			render(TimeTrackingInput, { props: defaultProps });
			
			const input = screen.getByDisplayValue('2.5');
			await fireEvent.input(input, { target: { value: 'abc' } });
			await fireEvent.change(input);
			
			// Should show 0 as fallback for NaN
			expect(screen.getByText(/Total: 0h/)).toBeInTheDocument();
		});
	});
});