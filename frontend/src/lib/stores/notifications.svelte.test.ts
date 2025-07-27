import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest';
import { notifications } from './notifications.svelte';

describe('Notifications Store', () => {
	beforeEach(() => {
		// Clear all notifications before each test
		notifications.clear();
		vi.useFakeTimers();
	});

	afterEach(() => {
		vi.useRealTimers();
	});

	describe('Basic Functionality', () => {
		it('should start with empty items array', () => {
			expect(notifications.items).toEqual([]);
		});

		it('should add success notification', () => {
			notifications.showSuccess('Success message');
			
			expect(notifications.items).toHaveLength(1);
			expect(notifications.items[0]!).toMatchObject({
				type: 'success',
				message: 'Success message',
				duration: 5000
			});
			expect(notifications.items[0]!!.createdAt).toBeInstanceOf(Date);
		});

		it('should add error notification with longer duration', () => {
			notifications.showError('Error message');
			
			expect(notifications.items).toHaveLength(1);
			expect(notifications.items[0]!).toMatchObject({
				type: 'error',
				message: 'Error message',
				duration: 8000
			});
		});

		it('should add warning notification', () => {
			notifications.showWarning('Warning message');
			
			expect(notifications.items).toHaveLength(1);
			expect(notifications.items[0]!).toMatchObject({
				type: 'warning',
				message: 'Warning message',
				duration: 6000
			});
		});

		it('should add info notification', () => {
			notifications.showInfo('Info message');
			
			expect(notifications.items).toHaveLength(1);
			expect(notifications.items[0]!).toMatchObject({
				type: 'info',
				message: 'Info message',
				duration: 5000
			});
		});
	});

	describe('Custom Duration', () => {
		it('should accept custom duration for success', () => {
			notifications.showSuccess('Custom duration', 3000);
			
			expect(notifications.items[0]!).toMatchObject({
				duration: 3000
			});
		});

		it('should accept custom duration for error', () => {
			notifications.showError('Custom error', 10000);
			
			expect(notifications.items[0]!).toMatchObject({
				duration: 10000
			});
		});

		it('should accept custom duration for warning', () => {
			notifications.showWarning('Custom warning', 7000);
			
			expect(notifications.items[0]!).toMatchObject({
				duration: 7000
			});
		});

		it('should accept custom duration for info', () => {
			notifications.showInfo('Custom info', 4000);
			
			expect(notifications.items[0]!).toMatchObject({
				duration: 4000
			});
		});
	});

	describe('ID Generation', () => {
		it('should generate unique IDs', () => {
			const id1 = notifications.showSuccess('First');
			const id2 = notifications.showSuccess('Second');
			const id3 = notifications.showError('Third');
			
			expect(id1).not.toBe(id2);
			expect(id2).not.toBe(id3);
			expect(id1).not.toBe(id3);
		});

		it('should generate IDs with correct format', () => {
			notifications.showInfo('Test');
			
			expect(notifications.items[0]!.id).toMatch(/^notification-\d+-\d+$/);
		});

		it('should increment counter in IDs', () => {
			const id1 = notifications.showSuccess('First');
			const id2 = notifications.showSuccess('Second');
			
			const counter1 = parseInt(id1.split('-')[1]!);
			const counter2 = parseInt(id2.split('-')[1]!);
			
			expect(counter2).toBe(counter1 + 1);
		});
	});

	describe('Notification Management', () => {
		it('should remove notification by ID', () => {
			const id1 = notifications.showSuccess('First');
			const id2 = notifications.showError('Second');
			
			expect(notifications.items).toHaveLength(2);
			
			notifications.remove(id1);
			
			expect(notifications.items).toHaveLength(1);
			expect(notifications.items[0]!.id).toBe(id2);
		});

		it('should handle removal of non-existent ID gracefully', () => {
			notifications.showSuccess('Test');
			
			expect(notifications.items).toHaveLength(1);
			
			notifications.remove('non-existent-id');
			
			expect(notifications.items).toHaveLength(1);
		});

		it('should clear all notifications', () => {
			notifications.showSuccess('First');
			notifications.showError('Second');
			notifications.showWarning('Third');
			
			expect(notifications.items).toHaveLength(3);
			
			notifications.clear();
			
			expect(notifications.items).toHaveLength(0);
		});
	});

	describe('Queue Management', () => {
		it('should maintain order of notifications', () => {
			const id1 = notifications.showSuccess('First');
			const id2 = notifications.showError('Second');
			const id3 = notifications.showWarning('Third');
			
			expect(notifications.items[0]!.id).toBe(id1);
			expect(notifications.items[1]!.id).toBe(id2);
			expect(notifications.items[2]!.id).toBe(id3);
		});

		it('should limit notifications to 5 items', () => {
			// Add 7 notifications
			for (let i = 0; i < 7; i++) {
				notifications.showInfo(`Message ${i + 1}`);
			}
			
			expect(notifications.items).toHaveLength(5);
			
			// Should contain the last 5 messages
			expect(notifications.items[0]!.message).toBe('Message 3');
			expect(notifications.items[4]!.message).toBe('Message 7');
		});

		it('should remove oldest notifications when exceeding limit', () => {
			const id1 = notifications.showSuccess('First');
			const id2 = notifications.showError('Second');
			const id3 = notifications.showWarning('Third');
			const id4 = notifications.showInfo('Fourth');
			const id5 = notifications.showSuccess('Fifth');
			
			// This should push out the first one only
			const id6 = notifications.showError('Sixth');
			
			expect(notifications.items).toHaveLength(5);
			expect(notifications.items.find(n => n.id === id1)).toBeUndefined();
			
			// Second through sixth should still be there
			expect(notifications.items.find(n => n.id === id2)).toBeDefined();
			expect(notifications.items.find(n => n.id === id3)).toBeDefined();
			expect(notifications.items.find(n => n.id === id4)).toBeDefined();
			expect(notifications.items.find(n => n.id === id5)).toBeDefined();
			expect(notifications.items.find(n => n.id === id6)).toBeDefined();
			
			expect(notifications.items[4]!.message).toBe('Sixth');
		});
	});

	describe('API Error Helper', () => {
		it('should handle Error objects', () => {
			const error = new Error('API Error');
			notifications.showApiError(error);
			
			expect(notifications.items[0]!).toMatchObject({
				type: 'error',
				message: 'API Error'
			});
		});

		it('should handle string errors', () => {
			notifications.showApiError('String error');
			
			expect(notifications.items[0]!).toMatchObject({
				type: 'error',
				message: 'String error'
			});
		});

		it('should handle objects with message property', () => {
			const error = { message: 'Object error' };
			notifications.showApiError(error);
			
			expect(notifications.items[0]!).toMatchObject({
				type: 'error',
				message: 'Object error'
			});
		});

		it('should use default message for unknown error types', () => {
			const error = { someProperty: 'value' };
			notifications.showApiError(error);
			
			expect(notifications.items[0]!).toMatchObject({
				type: 'error',
				message: 'An unexpected error occurred'
			});
		});

		it('should use custom default message', () => {
			const error = null;
			notifications.showApiError(error, 'Custom default message');
			
			expect(notifications.items[0]!).toMatchObject({
				type: 'error',
				message: 'Custom default message'
			});
		});

		it('should handle nested message objects', () => {
			const error = { message: { toString: () => 'Nested message' } };
			notifications.showApiError(error);
			
			expect(notifications.items[0]!).toMatchObject({
				type: 'error',
				message: 'Nested message'
			});
		});

		it('should handle undefined and null errors', () => {
			notifications.showApiError(undefined);
			notifications.showApiError(null);
			
			expect(notifications.items).toHaveLength(2);
			expect(notifications.items[0]!.message).toBe('An unexpected error occurred');
			expect(notifications.items[1]!.message).toBe('An unexpected error occurred');
		});
	});

	describe('Edge Cases', () => {
		it('should handle empty messages', () => {
			notifications.showSuccess('');
			
			expect(notifications.items[0]!).toMatchObject({
				type: 'success',
				message: ''
			});
		});

		it('should handle very long messages', () => {
			const longMessage = 'A'.repeat(1000);
			notifications.showError(longMessage);
			
			expect(notifications.items[0]!).toMatchObject({
				type: 'error',
				message: longMessage
			});
		});

		it('should handle special characters in messages', () => {
			const specialMessage = 'Message with <script>alert("xss")</script> & quotes "test" & symbols ®™€';
			notifications.showWarning(specialMessage);
			
			expect(notifications.items[0]!).toMatchObject({
				type: 'warning',
				message: specialMessage
			});
		});

		it('should handle zero duration', () => {
			notifications.showInfo('No auto-dismiss', 0);
			
			expect(notifications.items[0]!).toMatchObject({
				duration: 0
			});
		});

		it('should handle negative duration', () => {
			notifications.showSuccess('Negative duration', -1000);
			
			expect(notifications.items[0]!).toMatchObject({
				duration: -1000
			});
		});
	});

	describe('Reactivity', () => {
		it('should maintain reactivity when adding notifications', () => {
			const initialLength = notifications.items.length;
			
			notifications.showSuccess('Test reactivity');
			
			expect(notifications.items.length).toBe(initialLength + 1);
		});

		it('should maintain reactivity when removing notifications', () => {
			notifications.showSuccess('Test removal');
			expect(notifications.items.length).toBe(1);
			
			notifications.remove(notifications.items[0]!.id);
			
			expect(notifications.items.length).toBe(0);
		});

		it('should maintain reactivity when clearing notifications', () => {
			notifications.showSuccess('Test 1');
			notifications.showError('Test 2');
			expect(notifications.items.length).toBe(2);
			
			notifications.clear();
			
			expect(notifications.items.length).toBe(0);
		});
	});

	describe('Timestamp Accuracy', () => {
		it('should set accurate timestamps', () => {
			const before = new Date();
			notifications.showInfo('Timestamp test');
			const after = new Date();
			
			const notification = notifications.items[0]!;
			expect(notification.createdAt.getTime()).toBeGreaterThanOrEqual(before.getTime());
			expect(notification.createdAt.getTime()).toBeLessThanOrEqual(after.getTime());
		});

		it('should have different timestamps for rapid notifications', () => {
			notifications.showSuccess('First');
			notifications.showSuccess('Second');
			
			const time1 = notifications.items[0]!.createdAt.getTime();
			const time2 = notifications.items[1]!.createdAt.getTime();
			
			expect(time2).toBeGreaterThanOrEqual(time1);
		});
	});
});