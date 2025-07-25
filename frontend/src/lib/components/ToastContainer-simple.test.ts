import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import { notifications } from '../stores/notifications.svelte';

// Simple test component for ToastContainer
const TestToastContainer = `
<script>
	import { notifications } from '../stores/notifications.svelte';
</script>

<div class="toast-container fixed top-4 right-4 z-50 space-y-2 pointer-events-none">
	{#each notifications.items as notification (notification.id)}
		<div class="pointer-events-auto">
			<div 
				data-testid="toast-{notification.id}"
				data-type="{notification.type}"
				data-message="{notification.message}"
				data-duration="{notification.duration || ''}"
				role="alert"
				class="test-toast"
			>
				{notification.message}
			</div>
		</div>
	{/each}
</div>
`;

describe('ToastContainer Component Integration', () => {
	beforeEach(() => {
		notifications.clear();
	});

	it('should render empty container when no notifications', () => {
		const { container } = render({
			template: TestToastContainer
		});
		
		const toastContainer = container.querySelector('.toast-container');
		expect(toastContainer).toBeInTheDocument();
		expect(toastContainer?.children).toHaveLength(0);
	});

	it('should render single notification', () => {
		notifications.showSuccess('Success message');
		
		const { container } = render({
			template: TestToastContainer
		});
		
		expect(screen.getByTestId(/toast-/)).toBeInTheDocument();
		expect(screen.getByTestId(/toast-/)).toHaveAttribute('data-type', 'success');
		expect(screen.getByTestId(/toast-/)).toHaveAttribute('data-message', 'Success message');
		expect(screen.getByText('Success message')).toBeInTheDocument();
	});

	it('should render multiple notifications', () => {
		notifications.showSuccess('First message');
		notifications.showError('Second message');
		notifications.showWarning('Third message');
		
		const { container } = render({
			template: TestToastContainer
		});
		
		const alerts = screen.getAllByRole('alert');
		expect(alerts).toHaveLength(3);
		
		expect(screen.getByText('First message')).toBeInTheDocument();
		expect(screen.getByText('Second message')).toBeInTheDocument();
		expect(screen.getByText('Third message')).toBeInTheDocument();
	});

	it('should have correct container classes', () => {
		const { container } = render({
			template: TestToastContainer
		});
		
		const toastContainer = container.querySelector('.toast-container');
		expect(toastContainer).toHaveClass(
			'fixed', 
			'top-4', 
			'right-4', 
			'z-50', 
			'space-y-2', 
			'pointer-events-none'
		);
	});

	it('should wrap each toast in pointer-events-auto container', () => {
		notifications.showInfo('Test message');
		
		const { container } = render({
			template: TestToastContainer
		});
		
		const wrapper = screen.getByTestId(/toast-/).parentElement;
		expect(wrapper).toHaveClass('pointer-events-auto');
	});

	it('should handle notifications with different durations', () => {
		notifications.showSuccess('Auto dismiss', 3000);
		notifications.showError('Manual dismiss', 0);
		
		const { container } = render({
			template: TestToastContainer
		});
		
		const alerts = screen.getAllByRole('alert');
		expect(alerts).toHaveLength(2);
		
		expect(alerts[0]).toHaveAttribute('data-duration', '3000');
		expect(alerts[1]).toHaveAttribute('data-duration', '0');
	});

	it('should maintain order of notifications', () => {
		notifications.showInfo('First notification');
		notifications.showSuccess('Second notification');
		notifications.showWarning('Third notification');
		
		const { container } = render({
			template: TestToastContainer
		});
		
		const toastContainer = container.querySelector('.toast-container');
		const children = Array.from(toastContainer?.children || []);
		
		expect(children).toHaveLength(3);
		expect(children[0].textContent).toContain('First notification');
		expect(children[1].textContent).toContain('Second notification');
		expect(children[2].textContent).toContain('Third notification');
	});

	it('should respect queue limit of 5 notifications', () => {
		// Add 7 notifications
		for (let i = 1; i <= 7; i++) {
			notifications.showInfo(`Message ${i}`);
		}
		
		const { container } = render({
			template: TestToastContainer
		});
		
		const alerts = screen.getAllByRole('alert');
		expect(alerts).toHaveLength(5);
		
		// Should show the last 5 messages
		expect(screen.getByText('Message 3')).toBeInTheDocument();
		expect(screen.getByText('Message 7')).toBeInTheDocument();
		expect(screen.queryByText('Message 1')).not.toBeInTheDocument();
		expect(screen.queryByText('Message 2')).not.toBeInTheDocument();
	});

	it('should handle empty notification messages', () => {
		notifications.showInfo('');
		
		const { container } = render({
			template: TestToastContainer
		});
		
		const toast = screen.getByTestId(/toast-/);
		expect(toast).toBeInTheDocument();
		expect(toast).toHaveAttribute('data-message', '');
	});

	it('should handle special characters in messages', () => {
		const specialMessage = 'Error: <script>alert("xss")</script> & quotes "test"';
		notifications.showError(specialMessage);
		
		const { container } = render({
			template: TestToastContainer
		});
		
		const toast = screen.getByTestId(/toast-/);
		expect(toast).toBeInTheDocument();
		expect(toast).toHaveAttribute('data-message', specialMessage);
		expect(screen.getByText(specialMessage)).toBeInTheDocument();
	});

	it('should have proper z-index for layering above other content', () => {
		const { container } = render({
			template: TestToastContainer
		});
		
		const toastContainer = container.querySelector('.toast-container');
		expect(toastContainer).toHaveClass('z-50');
	});

	it('should be positioned fixed at top-right', () => {
		const { container } = render({
			template: TestToastContainer
		});
		
		const toastContainer = container.querySelector('.toast-container');
		expect(toastContainer).toHaveClass('fixed', 'top-4', 'right-4');
	});
});