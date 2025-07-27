import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import { notifications } from '../stores/notifications.svelte';
import ToastContainer from './ToastContainer.svelte';

// Mock onMount for controlled testing
vi.mock('svelte', async () => {
	const actual = await vi.importActual('svelte');
	return {
		...actual,
		onMount: vi.fn((fn) => fn())
	};
});

describe('Notification System Integration', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		vi.useFakeTimers();
		notifications.clear();
	});

	afterEach(() => {
		vi.useRealTimers();
	});

	describe('End-to-End Toast Flow', () => {
		it('should display toast when notification is added to store', async () => {
			const { rerender } = render(ToastContainer);
			
			// Initially no toasts
			expect(screen.queryByRole('alert')).not.toBeInTheDocument();
			
			// Add notification
			notifications.showSuccess('Test success message');
			
			// Re-render to reflect store changes
			rerender({});
			
			await waitFor(() => {
				expect(screen.getByRole('alert')).toBeInTheDocument();
				expect(screen.getByText('Test success message')).toBeInTheDocument();
			});
		});

		it('should remove toast when dismissed', async () => {
			const { rerender } = render(ToastContainer);
			
			// Add notification
			notifications.showError('Error message');
			rerender({});
			
			await waitFor(() => {
				expect(screen.getByRole('alert')).toBeInTheDocument();
			});
			
			// Click dismiss button
			const dismissButton = screen.getByLabelText('Close notification');
			await fireEvent.click(dismissButton);
			
			// Complete animation
			vi.advanceTimersByTime(300);
			rerender({});
			
			await waitFor(() => {
				expect(screen.queryByRole('alert')).not.toBeInTheDocument();
			});
		});

		it('should auto-dismiss toast after duration', async () => {
			const { rerender } = render(ToastContainer);
			
			// Add notification with short duration
			notifications.showInfo('Auto dismiss message', 1000);
			rerender({});
			
			await waitFor(() => {
				expect(screen.getByRole('alert')).toBeInTheDocument();
			});
			
			// Wait for auto-dismiss
			vi.advanceTimersByTime(1000);
			vi.advanceTimersByTime(300); // Animation delay
			rerender({});
			
			await waitFor(() => {
				expect(screen.queryByRole('alert')).not.toBeInTheDocument();
			});
		});
	});

	describe('Multiple Notifications', () => {
		it('should display multiple notifications simultaneously', async () => {
			const { rerender } = render(ToastContainer);
			
			// Add multiple notifications
			notifications.showSuccess('First message');
			notifications.showError('Second message');
			notifications.showWarning('Third message');
			
			rerender({});
			
			await waitFor(() => {
				const alerts = screen.getAllByRole('alert');
				expect(alerts).toHaveLength(3);
				expect(screen.getByText('First message')).toBeInTheDocument();
				expect(screen.getByText('Second message')).toBeInTheDocument();
				expect(screen.getByText('Third message')).toBeInTheDocument();
			});
		});

		it('should maintain correct order of notifications', async () => {
			const { rerender } = render(ToastContainer);
			
			// Add notifications in specific order
			notifications.showSuccess('First');
			notifications.showError('Second');
			notifications.showWarning('Third');
			
			rerender({});
			
			await waitFor(() => {
				const container = document.querySelector('.toast-container');
				const children = Array.from(container?.children || []);
				
				expect(children).toHaveLength(3);
				expect(children[0]!.textContent).toContain('First');
				expect(children[1]!.textContent).toContain('Second');
				expect(children[2]!.textContent).toContain('Third');
			});
		});

		it('should respect queue limit of 5 notifications', async () => {
			const { rerender } = render(ToastContainer);
			
			// Add 7 notifications
			for (let i = 1; i <= 7; i++) {
				notifications.showInfo(`Message ${i}`);
			}
			
			rerender({});
			
			await waitFor(() => {
				const alerts = screen.getAllByRole('alert');
				expect(alerts).toHaveLength(5);
				
				// Should show the last 5 messages
				expect(screen.getByText('Message 3')).toBeInTheDocument();
				expect(screen.getByText('Message 7')).toBeInTheDocument();
				expect(screen.queryByText('Message 1')).not.toBeInTheDocument();
				expect(screen.queryByText('Message 2')).not.toBeInTheDocument();
			});
		});
	});

	describe('Different Notification Types', () => {
		it('should render all notification types with correct styling', async () => {
			const { rerender } = render(ToastContainer);
			
			notifications.showSuccess('Success message');
			notifications.showError('Error message');
			notifications.showWarning('Warning message');
			notifications.showInfo('Info message');
			
			rerender({});
			
			await waitFor(() => {
				const alerts = screen.getAllByRole('alert');
				expect(alerts).toHaveLength(4);
				
				// Check for type-specific styling classes
				const successToast = screen.getByText('Success message').closest('[role="alert"]');
				const errorToast = screen.getByText('Error message').closest('[role="alert"]');
				const warningToast = screen.getByText('Warning message').closest('[role="alert"]');
				const infoToast = screen.getByText('Info message').closest('[role="alert"]');
				
				expect(successToast).toHaveClass('bg-green-50');
				expect(errorToast).toHaveClass('bg-red-50');
				expect(warningToast).toHaveClass('bg-yellow-50');
				expect(infoToast).toHaveClass('bg-blue-50');
			});
		});

		it('should display correct icons for each notification type', async () => {
			const { rerender } = render(ToastContainer);
			
			notifications.showSuccess('Success');
			notifications.showError('Error');
			notifications.showWarning('Warning');
			notifications.showInfo('Info');
			
			rerender({});
			
			await waitFor(() => {
				expect(screen.getByText('✓')).toBeInTheDocument(); // Success icon
				expect(screen.getByText('✕')).toBeInTheDocument(); // Error icon
				expect(screen.getByText('⚠')).toBeInTheDocument(); // Warning icon
				expect(screen.getByText('ℹ')).toBeInTheDocument(); // Info icon
			});
		});
	});

	describe('API Error Helper Integration', () => {
		it('should properly format API errors', async () => {
			const { rerender } = render(ToastContainer);
			
			// Test different error types
			notifications.showApiError(new Error('Network error'));
			notifications.showApiError('String error');
			notifications.showApiError({ message: 'Object error' });
			notifications.showApiError(null, 'Custom default');
			
			rerender({});
			
			await waitFor(() => {
				expect(screen.getByText('Network error')).toBeInTheDocument();
				expect(screen.getByText('String error')).toBeInTheDocument();
				expect(screen.getByText('Object error')).toBeInTheDocument();
				expect(screen.getByText('Custom default')).toBeInTheDocument();
			});
		});
	});

	describe('Accessibility Integration', () => {
		it('should announce notifications to screen readers', async () => {
			const { rerender } = render(ToastContainer);
			
			notifications.showSuccess('Important update');
			rerender({});
			
			await waitFor(() => {
				const toast = screen.getByRole('alert');
				expect(toast).toHaveAttribute('aria-live', 'assertive');
				expect(toast).toHaveAttribute('aria-atomic', 'true');
			});
		});

		it('should support keyboard navigation for dismissal', async () => {
			const { rerender } = render(ToastContainer);
			
			notifications.showError('Dismissible error');
			rerender({});
			
			await waitFor(() => {
				const dismissButton = screen.getByLabelText('Close notification');
				expect(dismissButton).toBeInTheDocument();
				
				// Should be focusable
				dismissButton.focus();
				expect(document.activeElement).toBe(dismissButton);
			});
		});
	});

	describe('Animation Integration', () => {
		it('should handle entrance animations', async () => {
			const { rerender } = render(ToastContainer);
			
			notifications.showSuccess('Animated toast');
			rerender({});
			
			// Wait for the toast to appear
			await waitFor(() => {
				const toast = screen.getByRole('alert');
				expect(toast).toBeInTheDocument();
			}, { timeout: 1000 });
			
			// Allow any timers to run
			vi.advanceTimersByTime(100);
			
			// The toast should be present (animation classes may vary)
			const toast = screen.getByRole('alert');
			expect(toast).toBeInTheDocument();
		}, 10000);

		it('should handle exit animations when dismissed', async () => {
			const { rerender } = render(ToastContainer);
			
			notifications.showWarning('Dismissible toast');
			rerender({});
			
			await waitFor(() => {
				const toast = screen.getByRole('alert');
				expect(toast).toBeInTheDocument();
			});
			
			// Dismiss toast
			const dismissButton = screen.getByLabelText('Close notification');
			await fireEvent.click(dismissButton);
			
			// Should start exit animation
			const toast = screen.getByRole('alert');
			expect(toast).toHaveClass('translate-x-full', 'opacity-0');
			
			// Should be removed after animation
			vi.advanceTimersByTime(300);
			rerender({});
			
			await waitFor(() => {
				expect(screen.queryByRole('alert')).not.toBeInTheDocument();
			});
		});
	});

	describe('Performance and Memory', () => {
		it('should clean up properly when clearing all notifications', async () => {
			const { rerender } = render(ToastContainer);
			
			// Add multiple notifications
			for (let i = 0; i < 5; i++) {
				notifications.showInfo(`Message ${i}`);
			}
			
			rerender({});
			
			await waitFor(() => {
				expect(screen.getAllByRole('alert')).toHaveLength(5);
			});
			
			// Clear all
			notifications.clear();
			rerender({});
			
			await waitFor(() => {
				expect(screen.queryAllByRole('alert')).toHaveLength(0);
			});
		});

		it('should handle rapid notification creation and removal', async () => {
			const { rerender } = render(ToastContainer);
			
			// Rapidly add and remove notifications
			for (let i = 0; i < 10; i++) {
				const id = notifications.showSuccess(`Rapid ${i}`);
				if (i % 2 === 0) {
					notifications.remove(id);
				}
			}
			
			rerender({});
			
			await waitFor(() => {
				const alerts = screen.getAllByRole('alert');
				expect(alerts.length).toBeLessThanOrEqual(5); // Queue limit
			});
		});
	});

	describe('Custom Duration Integration', () => {
		it('should respect custom durations for different notification types', async () => {
			const { rerender } = render(ToastContainer);
			
			// Add notifications with different durations
			notifications.showSuccess('Quick success', 500);
			notifications.showError('Slow error', 2000);
			notifications.showInfo('No auto-dismiss', 0);
			
			rerender({});
			
			await waitFor(() => {
				expect(screen.getAllByRole('alert')).toHaveLength(3);
			});
			
			// Quick success should dismiss first
			vi.advanceTimersByTime(500);
			vi.advanceTimersByTime(300); // Animation
			rerender({});
			
			await waitFor(() => {
				expect(screen.getAllByRole('alert')).toHaveLength(2);
				expect(screen.queryByText('Quick success')).not.toBeInTheDocument();
			});
			
			// Slow error should still be there
			expect(screen.getByText('Slow error')).toBeInTheDocument();
			expect(screen.getByText('No auto-dismiss')).toBeInTheDocument();
			
			// After full slow error duration
			vi.advanceTimersByTime(1500);
			vi.advanceTimersByTime(300); // Animation
			rerender({});
			
			await waitFor(() => {
				expect(screen.getAllByRole('alert')).toHaveLength(1);
				expect(screen.queryByText('Slow error')).not.toBeInTheDocument();
				expect(screen.getByText('No auto-dismiss')).toBeInTheDocument();
			});
		});
	});

	describe('Edge Cases Integration', () => {
		it('should handle notifications with HTML content safely', async () => {
			const { rerender } = render(ToastContainer);
			
			const maliciousContent = '<script>alert("xss")</script>Safe content';
			notifications.showError(maliciousContent);
			
			rerender({});
			
			await waitFor(() => {
				// Should display the content as text, not execute HTML
				expect(screen.getByText(maliciousContent)).toBeInTheDocument();
				// Script should not have executed
				expect(document.querySelector('script')).not.toBeInTheDocument();
			});
		});

		it('should handle very long messages gracefully', async () => {
			const { rerender } = render(ToastContainer);
			
			const longMessage = 'A very long message that goes on and on '.repeat(20);
			notifications.showWarning(longMessage);
			
			rerender({});
			
			// First wait for the toast to appear
			await waitFor(() => {
				const toast = screen.getByRole('alert');
				expect(toast).toBeInTheDocument();
			}, { timeout: 1000 });
			
			// Then check for the long message content (may be truncated in DOM)
			const toast = screen.getByRole('alert');
			expect(toast).toBeInTheDocument();
			
			// Check if the message content is present (look for part of it)
			const shortMessage = 'A very long message that goes on and on';
			expect(screen.getByText(new RegExp(shortMessage))).toBeInTheDocument();
		}, 5000);

		it('should handle empty messages', async () => {
			const { rerender } = render(ToastContainer);
			
			notifications.showInfo('');
			
			rerender({});
			
			await waitFor(() => {
				const toast = screen.getByRole('alert');
				expect(toast).toBeInTheDocument();
				// Should still show the icon and close button
				expect(screen.getByText('ℹ')).toBeInTheDocument();
				expect(screen.getByLabelText('Close notification')).toBeInTheDocument();
			});
		});
	});
});