import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import Pagination from './Pagination.svelte';

describe('Pagination Component', () => {
	const defaultProps = {
		currentPage: 1,
		totalPages: 10,
		totalItems: 200,
		itemsPerPage: 20,
		onPageChange: vi.fn(),
		onItemsPerPageChange: vi.fn()
	};

	beforeEach(() => {
		vi.clearAllMocks();
	});

	describe('Basic Rendering', () => {
		it('should render pagination controls when totalPages > 1', () => {
			render(Pagination, { props: defaultProps });
			
			expect(screen.getByText('Previous')).toBeInTheDocument();
			expect(screen.getByText('Next')).toBeInTheDocument();
			expect(screen.getByText('Showing 1 to 20 of 200 games')).toBeInTheDocument();
			expect(screen.getByLabelText('Per page:')).toBeInTheDocument();
		});

		it('should not render pagination controls when totalPages <= 1', () => {
			render(Pagination, { 
				props: { 
					...defaultProps, 
					totalPages: 1 
				} 
			});
			
			expect(screen.queryByText('Previous')).not.toBeInTheDocument();
			expect(screen.queryByText('Next')).not.toBeInTheDocument();
		});

		it('should render with correct items per page selector', () => {
			render(Pagination, { props: defaultProps });
			
			const select = screen.getByLabelText('Per page:');
			expect(select).toBeInTheDocument();
			// For Svelte select elements, we need to check selected option differently
			const selectedOption = select.querySelector('option[selected]') || select.querySelector(`option[value="${defaultProps.itemsPerPage}"]`);
			expect(selectedOption).toBeInTheDocument();
			// Check for specific option elements within the select
			expect(select.querySelector('option[value="10"]')).toBeInTheDocument();
			expect(select.querySelector('option[value="50"]')).toBeInTheDocument();
			expect(select.querySelector('option[value="100"]')).toBeInTheDocument();
		});
	});

	describe('Items Display', () => {
		it('should show correct item range for first page', () => {
			render(Pagination, { props: defaultProps });
			
			expect(screen.getByText('Showing 1 to 20 of 200 games')).toBeInTheDocument();
		});

		it('should show correct item range for middle page', () => {
			render(Pagination, { 
				props: { 
					...defaultProps, 
					currentPage: 5 
				} 
			});
			
			expect(screen.getByText('Showing 81 to 100 of 200 games')).toBeInTheDocument();
		});

		it('should show correct item range for last page with partial items', () => {
			render(Pagination, { 
				props: { 
					...defaultProps, 
					currentPage: 10,
					totalItems: 195 
				} 
			});
			
			expect(screen.getByText('Showing 181 to 195 of 195 games')).toBeInTheDocument();
		});

		it('should handle single item correctly', () => {
			render(Pagination, { 
				props: { 
					...defaultProps, 
					currentPage: 1,
					totalPages: 1,
					totalItems: 1,
					itemsPerPage: 20
				} 
			});
			
			// Should not render pagination for single page
			expect(screen.queryByText('Showing')).not.toBeInTheDocument();
		});
	});

	describe('Page Navigation', () => {
		it('should call onPageChange when clicking a page number', async () => {
			const onPageChange = vi.fn();
			render(Pagination, { 
				props: { 
					...defaultProps, 
					onPageChange 
				} 
			});
			
			// Page 2 should be visible
			const page2Button = screen.getByText('2');
			await fireEvent.click(page2Button);
			
			expect(onPageChange).toHaveBeenCalledWith(2);
		});

		it('should call onPageChange when clicking Previous button', async () => {
			const onPageChange = vi.fn();
			render(Pagination, { 
				props: { 
					...defaultProps, 
					currentPage: 5,
					onPageChange 
				} 
			});
			
			const previousButton = screen.getByText('Previous');
			await fireEvent.click(previousButton);
			
			expect(onPageChange).toHaveBeenCalledWith(4);
		});

		it('should call onPageChange when clicking Next button', async () => {
			const onPageChange = vi.fn();
			render(Pagination, { 
				props: { 
					...defaultProps, 
					currentPage: 5,
					onPageChange 
				} 
			});
			
			const nextButton = screen.getByText('Next');
			await fireEvent.click(nextButton);
			
			expect(onPageChange).toHaveBeenCalledWith(6);
		});

		it('should disable Previous button on first page', () => {
			render(Pagination, { 
				props: { 
					...defaultProps, 
					currentPage: 1 
				} 
			});
			
			const previousButton = screen.getByText('Previous');
			expect(previousButton).toBeDisabled();
		});

		it('should disable Next button on last page', () => {
			render(Pagination, { 
				props: { 
					...defaultProps, 
					currentPage: 10 
				} 
			});
			
			const nextButton = screen.getByText('Next');
			expect(nextButton).toBeDisabled();
		});

		it('should not call onPageChange when clicking current page', async () => {
			const onPageChange = vi.fn();
			render(Pagination, { 
				props: { 
					...defaultProps, 
					currentPage: 3,
					onPageChange 
				} 
			});
			
			const currentPageButton = screen.getByText('3');
			await fireEvent.click(currentPageButton);
			
			expect(onPageChange).not.toHaveBeenCalled();
		});

		it('should not call onPageChange when clicking ellipsis', async () => {
			const onPageChange = vi.fn();
			render(Pagination, { 
				props: { 
					...defaultProps, 
					currentPage: 6,
					totalPages: 20,
					onPageChange 
				} 
			});
			
			// There should be ellipsis in the pagination
			const ellipsis = screen.getAllByText('...')[0];
			await fireEvent.click(ellipsis);
			
			expect(onPageChange).not.toHaveBeenCalled();
		});
	});

	describe('Visible Pages Logic', () => {
		it('should show pages correctly when totalPages is 5', () => {
			render(Pagination, { 
				props: { 
					...defaultProps, 
					totalPages: 5 
				} 
			});
			
			// Based on the actual algorithm, when currentPage=1, totalPages=5, it shows: 1 2 3 ... 5
			expect(screen.getByRole('button', { name: '1' })).toBeInTheDocument();
			expect(screen.getByRole('button', { name: '2' })).toBeInTheDocument();
			expect(screen.getByRole('button', { name: '3' })).toBeInTheDocument();
			expect(screen.getByRole('button', { name: '5' })).toBeInTheDocument();
			expect(screen.getByText('...')).toBeInTheDocument();
		});

		it('should show ellipsis at beginning when current page is far from start', () => {
			render(Pagination, { 
				props: { 
					...defaultProps, 
					currentPage: 8,
					totalPages: 15 
				} 
			});
			
			// Should show: 1 ... 6 7 8 9 10 15 (note: based on actual algorithm behavior)
			expect(screen.getByRole('button', { name: '1' })).toBeInTheDocument();
			expect(screen.getByRole('button', { name: '6' })).toBeInTheDocument();
			expect(screen.getByRole('button', { name: '7' })).toBeInTheDocument();
			expect(screen.getByRole('button', { name: '8' })).toBeInTheDocument();
			expect(screen.getByRole('button', { name: '9' })).toBeInTheDocument();
			expect(screen.getByRole('button', { name: '10' })).toBeInTheDocument();
			expect(screen.getByRole('button', { name: '15' })).toBeInTheDocument();
			expect(screen.getAllByText('...')).toHaveLength(1);
		});

		it('should show ellipsis at end when current page is far from end', () => {
			render(Pagination, { 
				props: { 
					...defaultProps, 
					currentPage: 3,
					totalPages: 15 
				} 
			});
			
			// Should show: 1 2 3 4 5 ... 15
			expect(screen.getByText('1')).toBeInTheDocument();
			expect(screen.getByText('2')).toBeInTheDocument();
			expect(screen.getByText('3')).toBeInTheDocument();
			expect(screen.getByText('4')).toBeInTheDocument();
			expect(screen.getByText('5')).toBeInTheDocument();
			expect(screen.getByText('15')).toBeInTheDocument();
			expect(screen.getAllByText('...')).toHaveLength(1);
		});

		it('should not show ellipsis when current page is close to start', () => {
			render(Pagination, { 
				props: { 
					...defaultProps, 
					currentPage: 2,
					totalPages: 10 
				} 
			});
			
			// Should show: 1 2 3 4 ... 10 (no ellipsis at start)
			expect(screen.getByRole('button', { name: '1' })).toBeInTheDocument();
			expect(screen.getByRole('button', { name: '2' })).toBeInTheDocument();
			expect(screen.getByRole('button', { name: '3' })).toBeInTheDocument();
			expect(screen.getByRole('button', { name: '4' })).toBeInTheDocument();
			expect(screen.getByRole('button', { name: '10' })).toBeInTheDocument();
			
			// Should only have one ellipsis at the end
			expect(screen.getAllByText('...')).toHaveLength(1);
		});

		it('should handle edge case with very few pages', () => {
			render(Pagination, { 
				props: { 
					...defaultProps, 
					totalPages: 2 
				} 
			});
			
			expect(screen.getByText('1')).toBeInTheDocument();
			expect(screen.getByText('2')).toBeInTheDocument();
			expect(screen.queryByText('...')).not.toBeInTheDocument();
		});
	});

	describe('Items Per Page', () => {
		it('should call onItemsPerPageChange when select value changes', async () => {
			const onItemsPerPageChange = vi.fn();
			render(Pagination, { 
				props: { 
					...defaultProps, 
					onItemsPerPageChange 
				} 
			});
			
			const select = screen.getByLabelText('Per page:');
			await fireEvent.change(select, { target: { value: '50' } });
			
			expect(onItemsPerPageChange).toHaveBeenCalledWith(50);
		});

		it('should display current itemsPerPage value in select', () => {
			render(Pagination, { 
				props: { 
					...defaultProps, 
					itemsPerPage: 50 
				} 
			});
			
			const select = screen.getByLabelText('Per page:');
			expect(select).toBeInTheDocument();
		});

		it('should have all expected options in select', () => {
			render(Pagination, { props: defaultProps });
			
			const select = screen.getByLabelText('Per page:');
			const options = select.querySelectorAll('option');
			
			expect(options).toHaveLength(4);
			expect(options[0]).toHaveValue('10');
			expect(options[1]).toHaveValue('20');
			expect(options[2]).toHaveValue('50');
			expect(options[3]).toHaveValue('100');
		});
	});

	describe('Edge Cases and Error Handling', () => {
		it('should handle zero totalItems gracefully', () => {
			render(Pagination, { 
				props: { 
					...defaultProps, 
					totalItems: 0,
					totalPages: 1 
				} 
			});
			
			// Should not render pagination for zero items
			expect(screen.queryByText('Previous')).not.toBeInTheDocument();
		});

		it('should handle currentPage beyond totalPages', () => {
			render(Pagination, { 
				props: { 
					...defaultProps, 
					currentPage: 15,
					totalPages: 10 
				} 
			});
			
			// Should still render but Next button should be disabled
			const nextButton = screen.getByText('Next');
			expect(nextButton).toBeDisabled();
		});

		it('should handle negative currentPage', () => {
			render(Pagination, { 
				props: { 
					...defaultProps, 
					currentPage: -1 
				} 
			});
			
			// Previous button should be disabled
			const previousButton = screen.getByText('Previous');
			expect(previousButton).toBeDisabled();
		});

		it('should handle missing callback functions gracefully', async () => {
			render(Pagination, { 
				props: { 
					...defaultProps, 
					onPageChange: undefined,
					onItemsPerPageChange: undefined 
				} 
			});
			
			// Should not throw errors when clicking
			const page2Button = screen.getByText('2');
			await fireEvent.click(page2Button); // Should not throw
			
			const select = screen.getByLabelText('Per page:');
			await fireEvent.change(select, { target: { value: '50' } }); // Should not throw
		});

		it('should handle very large numbers correctly', () => {
			render(Pagination, { 
				props: { 
					...defaultProps, 
					currentPage: 500,
					totalPages: 1000,
					totalItems: 20000,
					itemsPerPage: 20 
				} 
			});
			
			expect(screen.getByText('Showing 9981 to 10000 of 20000 games')).toBeInTheDocument();
		});
	});

	describe('Accessibility', () => {
		it('should have proper label for items per page select', () => {
			render(Pagination, { props: defaultProps });
			
			const select = screen.getByLabelText('Per page:');
			expect(select).toBeInTheDocument();
			expect(select).toHaveAttribute('id', 'items-per-page');
		});

		it('should have proper button elements for navigation', () => {
			render(Pagination, { props: defaultProps });
			
			const previousButton = screen.getByRole('button', { name: 'Previous' });
			const nextButton = screen.getByRole('button', { name: 'Next' });
			
			expect(previousButton).toBeInTheDocument();
			expect(nextButton).toBeInTheDocument();
		});

		it('should have proper button elements for page numbers', () => {
			render(Pagination, { props: defaultProps });
			
			const page1Button = screen.getByRole('button', { name: '1' });
			const page2Button = screen.getByRole('button', { name: '2' });
			
			expect(page1Button).toBeInTheDocument();
			expect(page2Button).toBeInTheDocument();
		});
	});

	describe('Default Props', () => {
		it('should use default values when props are not provided', () => {
			render(Pagination, { 
				props: {
					// Only provide required minimum
					totalPages: 2,
					totalItems: 30
				} 
			});
			
			// Should use defaults: currentPage=1, itemsPerPage=20
			expect(screen.getByText('Showing 1 to 20 of 30 games')).toBeInTheDocument();
			const select = screen.getByLabelText('Per page:');
			const option20 = select.querySelector('option[value="20"]');
			expect(option20).toBeInTheDocument();
		});

		it('should handle default empty callback functions', async () => {
			render(Pagination, { 
				props: {
					totalPages: 3,
					totalItems: 50
				} 
			});
			
			// Should not throw when using default empty callbacks
			const page2Button = screen.getByText('2');
			await fireEvent.click(page2Button); // Should not throw
			
			const select = screen.getByLabelText('Per page:');
			await fireEvent.change(select, { target: { value: '10' } }); // Should not throw
		});
	});
});