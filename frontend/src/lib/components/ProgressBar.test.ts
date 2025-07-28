import { describe, it, expect, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import ProgressBar from './ProgressBar.svelte';

describe('ProgressBar', () => {
	const defaultProps = {
		value: 50,
		max: 100,
		label: 'Test Progress',
		showPercentage: true,
		color: 'blue' as const,
		size: 'md' as const,
		animated: true,
		class: ''
	};

	beforeEach(() => {
		// Reset any global state if needed
	});

	describe('Basic Rendering', () => {
		it('should render with default props', () => {
			render(ProgressBar, { props: { value: 50 } });
			
			const progressBar = screen.getByRole('progressbar');
			expect(progressBar).toBeInTheDocument();
			expect(progressBar).toHaveAttribute('aria-valuenow', '50');
			expect(progressBar).toHaveAttribute('aria-valuemin', '0');
			expect(progressBar).toHaveAttribute('aria-valuemax', '100');
		});

		it('should render with all custom props', () => {
			render(ProgressBar, { props: defaultProps });
			
			const progressBar = screen.getByRole('progressbar');
			expect(progressBar).toBeInTheDocument();
			expect(progressBar).toHaveAttribute('aria-label', 'Test Progress');
			expect(screen.getByText('Test Progress')).toBeInTheDocument();
			expect(screen.getByText('50.0%')).toBeInTheDocument();
		});

		it('should render without label when not provided', () => {
			render(ProgressBar, { 
				props: { value: 75 } 
			});
			
			const progressBar = screen.getByRole('progressbar');
			expect(progressBar).toHaveAttribute('aria-label', 'Progress');
			expect(screen.queryByText('Test Progress')).not.toBeInTheDocument();
		});

		it('should apply custom CSS class', () => {
			render(ProgressBar, { 
				props: { value: 50, class: 'custom-progress-class' } 
			});
			
			const container = screen.getByRole('progressbar').closest('.custom-progress-class');
			expect(container).toBeInTheDocument();
		});
	});

	describe('Progress Values', () => {
		it('should handle zero value', () => {
			render(ProgressBar, { props: { value: 0 } });
			
			const progressBar = screen.getByRole('progressbar');
			expect(progressBar).toHaveAttribute('aria-valuenow', '0');
			expect(screen.getByText('0.0%')).toBeInTheDocument();
		});

		it('should handle maximum value', () => {
			render(ProgressBar, { props: { value: 100 } });
			
			const progressBar = screen.getByRole('progressbar');
			expect(progressBar).toHaveAttribute('aria-valuenow', '100');
			expect(screen.getByText('100.0%')).toBeInTheDocument();
		});

		it('should handle custom max value', () => {
			render(ProgressBar, { props: { value: 75, max: 150 } });
			
			const progressBar = screen.getByRole('progressbar');
			expect(progressBar).toHaveAttribute('aria-valuemax', '150');
			expect(screen.getByText('50.0%')).toBeInTheDocument(); // 75/150 = 50%
		});

		it('should clamp values above maximum to 100%', () => {
			render(ProgressBar, { props: { value: 150, max: 100 } });
			
			expect(screen.getByText('100.0%')).toBeInTheDocument();
		});

		it('should handle decimal values correctly', () => {
			render(ProgressBar, { props: { value: 33.333, max: 100 } });
			
			expect(screen.getByText('33.3%')).toBeInTheDocument();
		});

		it('should handle negative values gracefully', () => {
			render(ProgressBar, { props: { value: -10 } });
			
			const progressBar = screen.getByRole('progressbar');
			expect(progressBar).toHaveAttribute('aria-valuenow', '-10');
			// Should show negative percentage, but progress bar should not display anything
			expect(screen.getByText('-10.0%')).toBeInTheDocument();
		});
	});

	describe('Color Variants', () => {
		const colors = ['blue', 'green', 'purple', 'yellow', 'orange', 'red', 'gray'] as const;

		colors.forEach(color => {
			it(`should apply ${color} color classes`, () => {
				const { container } = render(ProgressBar, { 
					props: { value: 50, color } 
				});
				
				// Check for the color-specific background class
				const progressFill = container.querySelector(`.bg-${color}-500`);
				expect(progressFill).toBeInTheDocument();
				
				// Check for the color-specific background container class
				const progressContainer = container.querySelector(`.bg-${color}-100, .bg-${color}-200`);
				expect(progressContainer).toBeInTheDocument();
			});
		});
	});

	describe('Size Variants', () => {
		it('should apply small size class', () => {
			const { container } = render(ProgressBar, { 
				props: { value: 50, size: 'sm' } 
			});
			
			const progressElements = container.querySelectorAll('.h-2');
			expect(progressElements.length).toBeGreaterThan(0);
		});

		it('should apply medium size class (default)', () => {
			const { container } = render(ProgressBar, { 
				props: { value: 50, size: 'md' } 
			});
			
			const progressElements = container.querySelectorAll('.h-3');
			expect(progressElements.length).toBeGreaterThan(0);
		});

		it('should apply large size class', () => {
			const { container } = render(ProgressBar, { 
				props: { value: 50, size: 'lg' } 
			});
			
			const progressElements = container.querySelectorAll('.h-4');
			expect(progressElements.length).toBeGreaterThan(0);
		});
	});

	describe('Animation', () => {
		it('should include animation classes by default', () => {
			const { container } = render(ProgressBar, { 
				props: { value: 50, animated: true } 
			});
			
			const animatedElement = container.querySelector('.transition-all');
			expect(animatedElement).toBeInTheDocument();
		});

		it('should not include animation classes when disabled', () => {
			const { container } = render(ProgressBar, { 
				props: { value: 50, animated: false } 
			});
			
			const animatedElement = container.querySelector('.transition-all');
			expect(animatedElement).not.toBeInTheDocument();
		});
	});

	describe('Percentage Display', () => {
		it('should show percentage by default', () => {
			render(ProgressBar, { props: { value: 75 } });
			
			expect(screen.getByText('75.0%')).toBeInTheDocument();
		});

		it('should hide percentage when showPercentage is false', () => {
			render(ProgressBar, { 
				props: { value: 75, showPercentage: false } 
			});
			
			expect(screen.queryByText('75.0%')).not.toBeInTheDocument();
		});

		it('should show both label and percentage', () => {
			render(ProgressBar, { 
				props: { 
					value: 60, 
					label: 'Loading', 
					showPercentage: true 
				} 
			});
			
			expect(screen.getByText('Loading')).toBeInTheDocument();
			expect(screen.getByText('60.0%')).toBeInTheDocument();
		});

		it('should show only label when percentage is disabled', () => {
			render(ProgressBar, { 
				props: { 
					value: 60, 
					label: 'Processing', 
					showPercentage: false 
				} 
			});
			
			expect(screen.getByText('Processing')).toBeInTheDocument();
			expect(screen.queryByText('60.0%')).not.toBeInTheDocument();
		});
	});

	describe('Visual Layout', () => {
		it('should not render header section when no label and no percentage', () => {
			render(ProgressBar, { 
				props: { 
					value: 50, 
					label: '', 
					showPercentage: false 
				} 
			});
			
			// Should only have the progress bar, not the header section
			const progressBar = screen.getByRole('progressbar');
			expect(progressBar).toBeInTheDocument();
			
			// The flex container with justify-between should not exist
			const flexContainer = progressBar.parentElement?.querySelector('.flex.justify-between');
			expect(flexContainer).not.toBeInTheDocument();
		});

		it('should render header section when label is provided', () => {
			render(ProgressBar, { 
				props: { 
					value: 50, 
					label: 'Test Label', 
					showPercentage: false 
				} 
			});
			
			expect(screen.getByText('Test Label')).toBeInTheDocument();
		});

		it('should render header section when percentage is enabled', () => {
			render(ProgressBar, { 
				props: { 
					value: 50, 
					label: '', 
					showPercentage: true 
				} 
			});
			
			expect(screen.getByText('50.0%')).toBeInTheDocument();
		});
	});

	describe('Accessibility', () => {
		it('should have proper ARIA attributes', () => {
			render(ProgressBar, { 
				props: { 
					value: 75, 
					max: 200, 
					label: 'File Upload' 
				} 
			});
			
			const progressBar = screen.getByRole('progressbar');
			expect(progressBar).toHaveAttribute('role', 'progressbar');
			expect(progressBar).toHaveAttribute('aria-valuenow', '75');
			expect(progressBar).toHaveAttribute('aria-valuemin', '0');
			expect(progressBar).toHaveAttribute('aria-valuemax', '200');
			expect(progressBar).toHaveAttribute('aria-label', 'File Upload');
		});

		it('should use default aria-label when no label provided', () => {
			render(ProgressBar, { props: { value: 50 } });
			
			const progressBar = screen.getByRole('progressbar');
			expect(progressBar).toHaveAttribute('aria-label', 'Progress');
		});

		it('should have proper structure for screen readers', () => {
			render(ProgressBar, { 
				props: { 
					value: 80, 
					label: 'Download Progress' 
				} 
			});
			
			// Label should be in a span with proper text styling
			const label = screen.getByText('Download Progress');
			expect(label).toHaveClass('text-sm', 'font-medium', 'text-gray-700');
			
			// Percentage should be in a span with proper text styling
			const percentage = screen.getByText('80.0%');
			expect(percentage).toHaveClass('text-sm', 'text-gray-600');
		});
	});

	describe('Progress Bar Fill', () => {
		it('should have correct width based on percentage', () => {
			const { container } = render(ProgressBar, { props: { value: 75 } });
			
			// Find the inner div that represents the progress fill
			const progressFill = container.querySelector('div[style*="width: 75%"]');
			expect(progressFill).toBeInTheDocument();
		});

		it('should have correct width for zero progress', () => {
			const { container } = render(ProgressBar, { props: { value: 0 } });
			
			const progressFill = container.querySelector('div[style*="width: 0%"]');
			expect(progressFill).toBeInTheDocument();
		});

		it('should have correct width for full progress', () => {
			const { container } = render(ProgressBar, { props: { value: 100 } });
			
			const progressFill = container.querySelector('div[style*="width: 100%"]');
			expect(progressFill).toBeInTheDocument();
		});
	});

	describe('Edge Cases', () => {
		it('should handle very large numbers', () => {
			render(ProgressBar, { 
				props: { value: 999999, max: 1000000 } 
			});
			
			expect(screen.getByText('100.0%')).toBeInTheDocument();
		});

		it('should handle very small decimal values', () => {
			render(ProgressBar, { 
				props: { value: 0.001, max: 1 } 
			});
			
			expect(screen.getByText('0.1%')).toBeInTheDocument();
		});

		it('should handle zero max value gracefully', () => {
			expect(() => {
				render(ProgressBar, { props: { value: 50, max: 0 } });
			}).not.toThrow();
		});

		it('should handle empty string label', () => {
			render(ProgressBar, { 
				props: { value: 50, label: '' } 
			});
			
			const progressBar = screen.getByRole('progressbar');
			expect(progressBar).toHaveAttribute('aria-label', 'Progress');
		});

		it('should handle undefined values gracefully', () => {
			expect(() => {
				render(ProgressBar, { 
					props: { value: undefined as any } 
				});
			}).not.toThrow();
		});
	});

	describe('Visual Styling', () => {
		it('should have rounded corners', () => {
			const { container } = render(ProgressBar, { props: { value: 50 } });
			
			// Both container and fill should have rounded-full class
			const roundedElements = container.querySelectorAll('.rounded-full');
			expect(roundedElements.length).toBeGreaterThanOrEqual(2);
		});

		it('should have overflow hidden on container', () => {
			render(ProgressBar, { props: { value: 50 } });
			
			const progressContainer = screen.getByRole('progressbar');
			expect(progressContainer).toHaveClass('overflow-hidden');
		});

		it('should have full width classes', () => {
			const { container } = render(ProgressBar, { props: { value: 50 } });
			
			const fullWidthElements = container.querySelectorAll('.w-full');
			expect(fullWidthElements.length).toBeGreaterThanOrEqual(1);
		});
	});
});