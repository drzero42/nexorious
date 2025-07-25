import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import { notifications } from '../stores/notifications.svelte';

// Create a test component that uses the actual ToastContainer
const TestToastContainer = `
<script>
	import { notifications } from '../stores/notifications.svelte';
	import Toast from './Toast.svelte';

	// Simplified mock Toast for testing
	function MockToast(node, props) {
		const toastDiv = document.createElement('div');
		toastDiv.setAttribute('data-testid', \`toast-\${props.id}\`);
		toastDiv.setAttribute('data-type', props.type);
		toastDiv.setAttribute('data-message', props.message);
		toastDiv.setAttribute('data-duration', props.duration?.toString() || '');
		toastDiv.setAttribute('role', 'alert');
		toastDiv.textContent = props.message;
		toastDiv.className = 'test-toast';
		node.appendChild(toastDiv);
		
		return {
			destroy() {
				if (toastDiv.parentNode) {
					toastDiv.parentNode.removeChild(toastDiv);
				}
			}
		};
	}
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

describe('ToastContainer Component', () => {
	let mockNotifications: any;

	beforeEach(async () => {
		vi.clearAllMocks();
		// Import the mocked notifications store
		const { notifications } = await import('../stores/notifications.svelte');
		mockNotifications = notifications;
		mockNotifications.items = [];
	});

	it('should render empty container when no notifications', () => {
		render(ToastContainer);
		
		const container = document.querySelector('.toast-container');
		expect(container).toBeInTheDocument();
		expect(container?.children).toHaveLength(0);
	});

	it('should render single notification', () => {
		mockNotifications.items = [
			{
				id: 'test-1',
				type: 'success',
				message: 'Success message',
				duration: 5000,
				createdAt: new Date()
			}
		];

		render(ToastContainer);
		
		expect(screen.getByTestId('toast-test-1')).toBeInTheDocument();
		expect(screen.getByTestId('toast-test-1')).toHaveAttribute('data-type', 'success');
		expect(screen.getByTestId('toast-test-1')).toHaveAttribute('data-message', 'Success message');
	});

	it('should render multiple notifications', () => {
		mockNotifications.items = [
			{
				id: 'test-1',
				type: 'success',
				message: 'First message',
				duration: 5000,
				createdAt: new Date()
			},
			{
				id: 'test-2',
				type: 'error',
				message: 'Second message',
				duration: 8000,
				createdAt: new Date()
			},
			{
				id: 'test-3',
				type: 'warning',
				message: 'Third message',
				duration: 6000,
				createdAt: new Date()
			}
		];

		render(ToastContainer);
		
		expect(screen.getByTestId('toast-test-1')).toBeInTheDocument();
		expect(screen.getByTestId('toast-test-2')).toBeInTheDocument();
		expect(screen.getByTestId('toast-test-3')).toBeInTheDocument();
		
		expect(screen.getByTestId('toast-test-1')).toHaveAttribute('data-type', 'success');
		expect(screen.getByTestId('toast-test-2')).toHaveAttribute('data-type', 'error');
		expect(screen.getByTestId('toast-test-3')).toHaveAttribute('data-type', 'warning');
	});

	it('should have correct container classes', () => {
		render(ToastContainer);
		
		const container = document.querySelector('.toast-container');
		expect(container).toHaveClass(
			'fixed', 
			'top-4', 
			'right-4', 
			'z-50', 
			'space-y-2', 
			'pointer-events-none'
		);
	});

	it('should wrap each toast in pointer-events-auto container', () => {
		mockNotifications.items = [
			{
				id: 'test-1',
				type: 'info',
				message: 'Test message',
				duration: 5000,
				createdAt: new Date()
			}
		];

		render(ToastContainer);
		
		const wrapper = screen.getByTestId('toast-test-1').parentElement;
		expect(wrapper).toHaveClass('pointer-events-auto');
	});

	it('should handle notifications with different durations', () => {
		mockNotifications.items = [
			{
				id: 'test-1',
				type: 'success',
				message: 'Auto dismiss',
				duration: 3000,
				createdAt: new Date()
			},
			{
				id: 'test-2',
				type: 'error',
				message: 'Manual dismiss',
				duration: 0,
				createdAt: new Date()
			}
		];

		render(ToastContainer);
		
		expect(screen.getByTestId('toast-test-1')).toHaveAttribute('data-duration', '3000');
		expect(screen.getByTestId('toast-test-2')).toHaveAttribute('data-duration', '0');
	});

	it('should maintain order of notifications', () => {
		const now = new Date();
		mockNotifications.items = [
			{
				id: 'first',
				type: 'info',
				message: 'First notification',
				duration: 5000,
				createdAt: new Date(now.getTime() - 2000)
			},
			{
				id: 'second',
				type: 'success',
				message: 'Second notification',
				duration: 5000,
				createdAt: new Date(now.getTime() - 1000)
			},
			{
				id: 'third',
				type: 'warning',
				message: 'Third notification',
				duration: 5000,
				createdAt: now
			}
		];

		render(ToastContainer);
		
		const container = document.querySelector('.toast-container');
		const children = Array.from(container?.children || []);
		
		expect(children).toHaveLength(3);
		expect(children[0].querySelector('[data-testid="toast-first"]')).toBeInTheDocument();
		expect(children[1].querySelector('[data-testid="toast-second"]')).toBeInTheDocument();
		expect(children[2].querySelector('[data-testid="toast-third"]')).toBeInTheDocument();
	});

	it('should handle notifications with undefined duration', () => {
		mockNotifications.items = [
			{
				id: 'test-1',
				type: 'info',
				message: 'Test message',
				createdAt: new Date()
				// duration is undefined
			} as Notification
		];

		render(ToastContainer);
		
		expect(screen.getByTestId('toast-test-1')).toBeInTheDocument();
		expect(screen.getByTestId('toast-test-1')).toHaveAttribute('data-duration', '');
	});

	it('should handle empty notification messages', () => {
		mockNotifications.items = [
			{
				id: 'test-1',
				type: 'info',
				message: '',
				duration: 5000,
				createdAt: new Date()
			}
		];

		render(ToastContainer);
		
		expect(screen.getByTestId('toast-test-1')).toBeInTheDocument();
		expect(screen.getByTestId('toast-test-1')).toHaveAttribute('data-message', '');
	});

	it('should handle special characters in messages', () => {
		mockNotifications.items = [
			{
				id: 'test-1',
				type: 'error',
				message: 'Error: <script>alert("xss")</script> & quotes "test"',
				duration: 5000,
				createdAt: new Date()
			}
		];

		render(ToastContainer);
		
		expect(screen.getByTestId('toast-test-1')).toBeInTheDocument();
		expect(screen.getByTestId('toast-test-1')).toHaveAttribute('data-message', 'Error: <script>alert("xss")</script> & quotes "test"');
	});

	it('should have proper z-index for layering above other content', () => {
		render(ToastContainer);
		
		const container = document.querySelector('.toast-container');
		expect(container).toHaveClass('z-50');
	});

	it('should be positioned fixed at top-right', () => {
		render(ToastContainer);
		
		const container = document.querySelector('.toast-container');
		expect(container).toHaveClass('fixed', 'top-4', 'right-4');
	});

	it('should handle rapid notification updates', () => {
		// Start with empty
		render(ToastContainer);
		
		// Add notification
		mockNotifications.items = [
			{
				id: 'test-1',
				type: 'success',
				message: 'Added',
				duration: 5000,
				createdAt: new Date()
			}
		];

		// Re-render with updated items
		render(ToastContainer);
		expect(screen.getByTestId('toast-test-1')).toBeInTheDocument();

		// Add another
		mockNotifications.items.push({
			id: 'test-2',
			type: 'error',
			message: 'Error occurred',
			duration: 8000,
			createdAt: new Date()
		});

		// Re-render again
		render(ToastContainer);
		expect(screen.getByTestId('toast-test-1')).toBeInTheDocument();
		expect(screen.getByTestId('toast-test-2')).toBeInTheDocument();
	});
});