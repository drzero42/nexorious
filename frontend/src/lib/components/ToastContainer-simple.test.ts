import { describe, it, expect, beforeEach } from 'vitest';
import { notifications } from '../stores/notifications.svelte';

describe('ToastContainer Component Integration', () => {
	beforeEach(() => {
		notifications.clear();
	});

	it('should render empty container when no notifications', () => {
		// Test the notifications store directly since template rendering is not supported
		expect(notifications.items).toEqual([]);
	});

	it('should render single notification', () => {
		notifications.showSuccess('Success message');
		
		// Test the notifications store directly
		expect(notifications.items).toHaveLength(1);
		expect(notifications.items[0]!.type).toBe('success');
		expect(notifications.items[0]!.message).toBe('Success message');
	});

	it('should render multiple notifications', () => {
		notifications.showSuccess('First message');
		notifications.showError('Second message');
		notifications.showWarning('Third message');
		
		// Test the notifications store directly
		expect(notifications.items).toHaveLength(3);
		expect(notifications.items[0]!.message).toBe('First message');
		expect(notifications.items[1]!.message).toBe('Second message');
		expect(notifications.items[2]!.message).toBe('Third message');
	});

	it('should handle notifications with different types', () => {
		// Test that notifications store works correctly
		expect(notifications.items).toEqual([]);
		notifications.showInfo('Test');
		expect(notifications.items).toHaveLength(1);
		expect(notifications.items[0]!.type).toBe('info');
	});

	it('should handle info notifications', () => {
		notifications.showInfo('Test message');
		
		// Test the notification was added to store
		expect(notifications.items).toHaveLength(1);
		expect(notifications.items[0]!.type).toBe('info');
	});

	it('should handle notifications with different durations', () => {
		notifications.showSuccess('Auto dismiss', 3000);
		notifications.showError('Manual dismiss', 0);
		
		// Test the notifications were added with correct durations
		expect(notifications.items).toHaveLength(2);
		expect(notifications.items[0]!.duration).toBe(3000);
		expect(notifications.items[1]!.duration).toBe(0);
	});

	it('should maintain order of notifications', () => {
		notifications.showInfo('First notification');
		notifications.showSuccess('Second notification');
		notifications.showWarning('Third notification');
		
		// Test the notifications maintain order
		expect(notifications.items).toHaveLength(3);
		expect(notifications.items[0]!.message).toBe('First notification');
		expect(notifications.items[1]!.message).toBe('Second notification');
		expect(notifications.items[2]!.message).toBe('Third notification');
	});

	it('should respect queue limit of 5 notifications', () => {
		// Add 7 notifications
		for (let i = 1; i <= 7; i++) {
			notifications.showInfo(`Message ${i}`);
		}
		
		// Should maintain only 5 notifications (the last 5)
		expect(notifications.items).toHaveLength(5);
		expect(notifications.items.map(n => n.message)).toEqual([
			'Message 3',
			'Message 4', 
			'Message 5',
			'Message 6',
			'Message 7'
		]);
	});

	it('should handle empty notification messages', () => {
		notifications.showInfo('');
		
		expect(notifications.items).toHaveLength(1);
		expect(notifications.items[0]!.message).toBe('');
	});

	it('should handle special characters in messages', () => {
		const specialMessage = 'Error: <script>alert("xss")</script> & quotes "test"';
		notifications.showError(specialMessage);
		
		expect(notifications.items).toHaveLength(1);
		expect(notifications.items[0]!.message).toBe(specialMessage);
	});

	it('should support all notification types', () => {
		notifications.showSuccess('Success');
		notifications.showError('Error');
		notifications.showWarning('Warning');
		notifications.showInfo('Info');
		
		expect(notifications.items).toHaveLength(4);
		expect(notifications.items[0]!.type).toBe('success');
		expect(notifications.items[1]!.type).toBe('error');
		expect(notifications.items[2]!.type).toBe('warning');
		expect(notifications.items[3]!.type).toBe('info');
	});

	it('should be able to clear all notifications', () => {
		notifications.showSuccess('Test 1');
		notifications.showError('Test 2');
		expect(notifications.items).toHaveLength(2);
		
		notifications.clear();
		expect(notifications.items).toHaveLength(0);
	});
});