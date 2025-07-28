import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

// Mock localStorage
const localStorageMock = {
	getItem: vi.fn(),
	setItem: vi.fn(),
	removeItem: vi.fn(),
	clear: vi.fn(),
};
Object.defineProperty(window, 'localStorage', {
	value: localStorageMock
});

// Mock the app environment with explicit vi.doMock
vi.doMock('$app/environment', () => ({
	browser: true,
	dev: false
}));

describe('UI Store', () => {
	let ui: any;

	beforeEach(async () => {
		vi.clearAllMocks();
		vi.useFakeTimers();
		localStorageMock.getItem.mockReturnValue(null);
		
		// Mock browser environment before importing module
		vi.doMock('$app/environment', () => ({
			browser: true,
			dev: false
		}));
		
		// Clear module cache and reimport
		vi.resetModules();
		const module = await import('./ui.svelte');
		ui = module.ui;
	});

	afterEach(() => {
		vi.useRealTimers();
		vi.restoreAllMocks();
	});

	describe('Store Structure', () => {
		it('should have correct initial state', () => {
			const state = ui.value;
			
			expect(state).toMatchObject({
				notifications: [],
				modals: [],
				isLoading: false,
				loadingMessage: undefined,
				sidebar: {
					isOpen: false,
					isPinned: false
				},
				preferences: {
					density: 'comfortable',
					animations: true,
					pageSize: 20
				}
			});
		});

		it('should have all required methods', () => {
			const requiredMethods = [
				'addNotification',
				'removeNotification',
				'clearNotifications',
				'showSuccess',
				'showError',
				'showWarning',
				'showInfo',
				'openModal',
				'closeModal',
				'closeAllModals',
				'setLoading',
				'toggleSidebar',
				'openSidebar',
				'closeSidebar',
				'toggleSidebarPin',
				'setDensity',
				'setAnimations',
				'setPageSize'
			];

			requiredMethods.forEach(method => {
				expect(typeof ui[method]).toBe('function');
			});
		});
	});

	describe('LocalStorage Initialization', () => {
		it('should load preferences from localStorage', async () => {
			const storedPreferences = {
				sidebarPinned: true,
				preferences: {
					density: 'compact',
					animations: false,
					pageSize: 50
				}
			};

			localStorageMock.getItem.mockImplementation((key) => {
				if (key === 'ui-preferences') return JSON.stringify(storedPreferences);
				return null;
			});

			// Reimport to trigger initialization
			vi.resetModules();
			const module = await import('./ui.svelte');
			const freshUI = module.ui;

			expect(freshUI.value.sidebar.isPinned).toBe(true);
			expect(freshUI.value.preferences).toEqual({
				density: 'compact',
				animations: false,
				pageSize: 50
			});
		});

		it('should handle localStorage parsing errors gracefully', async () => {
			const consoleSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
			localStorageMock.getItem.mockReturnValue('invalid json');

			// Reimport to trigger initialization
			vi.resetModules();
			const module = await import('./ui.svelte');
			const freshUI = module.ui;

			expect(freshUI.value.preferences).toEqual({
				density: 'comfortable',
				animations: true,
				pageSize: 20
			});
			expect(consoleSpy).toHaveBeenCalled();

			consoleSpy.mockRestore();
		});

		it('should handle non-browser environment gracefully', async () => {
			// Mock browser as false
			vi.doMock('$app/environment', () => ({
				browser: false,
				dev: false
			}));

			// Reimport to test initialization without browser
			vi.resetModules();
			const module = await import('./ui.svelte');
			const nonBrowserUI = module.ui;

			// Should not crash and should have initial state
			expect(nonBrowserUI.value.preferences).toEqual({
				density: 'comfortable',
				animations: true,
				pageSize: 20
			});
		});
	});

	describe('Notification Management', () => {
		it('should add notification with auto-generated ID', () => {
			const notification = {
				type: 'success' as const,
				title: 'Test Success',
				message: 'Test message'
			};

			const id = ui.addNotification(notification);

			expect(typeof id).toBe('string');
			expect(ui.value.notifications).toHaveLength(1);
			expect(ui.value.notifications[0]).toMatchObject({
				id,
				type: 'success',
				title: 'Test Success',
				message: 'Test message',
				duration: 5000
			});
		});

		it('should add notification with custom duration', () => {
			const notification = {
				type: 'error' as const,
				title: 'Test Error',
				duration: 0
			};

			ui.addNotification(notification);

			expect(ui.value.notifications[0].duration).toBe(0);
		});

		it('should auto-remove notification after duration', () => {
			const notification = {
				type: 'info' as const,
				title: 'Auto Remove',
				duration: 1000
			};

			ui.addNotification(notification);
			expect(ui.value.notifications).toHaveLength(1);

			// Fast forward time
			vi.advanceTimersByTime(1000);

			expect(ui.value.notifications).toHaveLength(0);
		});

		it('should not auto-remove notification with 0 duration', () => {
			const notification = {
				type: 'error' as const,
				title: 'Permanent',
				duration: 0
			};

			ui.addNotification(notification);
			expect(ui.value.notifications).toHaveLength(1);

			// Fast forward time
			vi.advanceTimersByTime(10000);

			expect(ui.value.notifications).toHaveLength(1);
		});

		it('should remove specific notification', () => {
			const id1 = ui.addNotification({ type: 'success' as const, title: 'First' });
			const id2 = ui.addNotification({ type: 'info' as const, title: 'Second' });

			expect(ui.value.notifications).toHaveLength(2);

			ui.removeNotification(id1);

			expect(ui.value.notifications).toHaveLength(1);
			expect(ui.value.notifications[0].id).toBe(id2);
		});

		it('should clear all notifications', () => {
			ui.addNotification({ type: 'success' as const, title: 'First' });
			ui.addNotification({ type: 'error' as const, title: 'Second' });

			expect(ui.value.notifications).toHaveLength(2);

			ui.clearNotifications();

			expect(ui.value.notifications).toHaveLength(0);
		});

		it('should add notification with actions', () => {
			const mockAction = vi.fn();
			const notification = {
				type: 'warning' as const,
				title: 'With Actions',
				actions: [
					{ label: 'Confirm', action: mockAction }
				]
			};

			ui.addNotification(notification);

			expect(ui.value.notifications[0].actions).toHaveLength(1);
			expect(ui.value.notifications[0].actions![0].label).toBe('Confirm');
		});
	});

	describe('Notification Shortcuts', () => {
		it('should show success notification', () => {
			const id = ui.showSuccess('Success Title', 'Success message');

			expect(typeof id).toBe('string');
			expect(ui.value.notifications).toHaveLength(1);
			expect(ui.value.notifications[0]).toMatchObject({
				type: 'success',
				title: 'Success Title',
				message: 'Success message',
				duration: 3000
			});
		});

		it('should show error notification with permanent duration', () => {
			ui.showError('Error Title', 'Error message');

			expect(ui.value.notifications[0]).toMatchObject({
				type: 'error',
				title: 'Error Title',
				message: 'Error message',
				duration: 0
			});

			// Should not auto-remove
			vi.advanceTimersByTime(10000);
			expect(ui.value.notifications).toHaveLength(1);
		});

		it('should show warning notification', () => {
			ui.showWarning('Warning Title');

			expect(ui.value.notifications[0]).toMatchObject({
				type: 'warning',
				title: 'Warning Title',
				duration: 5000
			});
		});

		it('should show info notification', () => {
			ui.showInfo('Info Title', 'Info message');

			expect(ui.value.notifications[0]).toMatchObject({
				type: 'info',
				title: 'Info Title',
				message: 'Info message',
				duration: 4000
			});
		});

		it('should handle shortcuts without message', () => {
			ui.showSuccess('Just Title');
			ui.showError('Error Only');
			ui.showWarning('Warning Only');
			ui.showInfo('Info Only');

			expect(ui.value.notifications).toHaveLength(4);
			ui.value.notifications.forEach((notification: any) => {
				expect(notification.title).toBeTruthy();
			});
		});
	});

	describe('Modal Management', () => {
		it('should open modal with auto-generated ID', () => {
			const modal = {
				component: 'TestModal',
				props: { test: 'value' }
			};

			const id = ui.openModal(modal);

			expect(typeof id).toBe('string');
			expect(ui.value.modals).toHaveLength(1);
			expect(ui.value.modals[0]).toMatchObject({
				id,
				component: 'TestModal',
				props: { test: 'value' },
				size: 'md',
				closable: true
			});
		});

		it('should open modal with custom options', () => {
			const modal = {
				component: 'CustomModal',
				size: 'lg' as const,
				closable: false
			};

			ui.openModal(modal);

			expect(ui.value.modals[0]).toMatchObject({
				component: 'CustomModal',
				size: 'lg',
				closable: false
			});
		});

		it('should close specific modal', () => {
			const id1 = ui.openModal({ component: 'Modal1' });
			const id2 = ui.openModal({ component: 'Modal2' });

			expect(ui.value.modals).toHaveLength(2);

			ui.closeModal(id1);

			expect(ui.value.modals).toHaveLength(1);
			expect(ui.value.modals[0].id).toBe(id2);
		});

		it('should close all modals', () => {
			ui.openModal({ component: 'Modal1' });
			ui.openModal({ component: 'Modal2' });

			expect(ui.value.modals).toHaveLength(2);

			ui.closeAllModals();

			expect(ui.value.modals).toHaveLength(0);
		});

		it('should handle all modal sizes', () => {
			const sizes = ['sm', 'md', 'lg', 'xl', 'full'] as const;
			
			sizes.forEach(size => {
				ui.openModal({ component: 'TestModal', size });
			});

			expect(ui.value.modals).toHaveLength(5);
			sizes.forEach((size, index) => {
				expect(ui.value.modals[index].size).toBe(size);
			});
		});
	});

	describe('Loading State Management', () => {
		it('should set loading state', () => {
			ui.setLoading(true, 'Loading data...');

			expect(ui.value.isLoading).toBe(true);
			expect(ui.value.loadingMessage).toBe('Loading data...');
		});

		it('should clear loading state', () => {
			ui.setLoading(true, 'Loading...');
			ui.setLoading(false);

			expect(ui.value.isLoading).toBe(false);
			expect(ui.value.loadingMessage).toBeUndefined();
		});

		it('should set loading without message', () => {
			ui.setLoading(true);

			expect(ui.value.isLoading).toBe(true);
			expect(ui.value.loadingMessage).toBeUndefined();
		});
	});

	describe('Sidebar Management', () => {
		it('should toggle sidebar open/close', () => {
			expect(ui.value.sidebar.isOpen).toBe(false);

			ui.toggleSidebar();
			expect(ui.value.sidebar.isOpen).toBe(true);

			ui.toggleSidebar();
			expect(ui.value.sidebar.isOpen).toBe(false);
		});

		it('should open sidebar', () => {
			ui.openSidebar();
			expect(ui.value.sidebar.isOpen).toBe(true);
		});

		it('should close sidebar', () => {
			ui.openSidebar(); // First open it
			ui.closeSidebar();
			expect(ui.value.sidebar.isOpen).toBe(false);
		});

		it('should toggle sidebar pin and save to localStorage', () => {
			expect(ui.value.sidebar.isPinned).toBe(false);

			ui.toggleSidebarPin();

			expect(ui.value.sidebar.isPinned).toBe(true);
			// Should have called setItem for saving preferences
			expect(localStorageMock.setItem).toHaveBeenCalled();

			ui.toggleSidebarPin();
			expect(ui.value.sidebar.isPinned).toBe(false);
		});
	});

	describe('Preferences Management', () => {
		it('should set density and save to localStorage', () => {
			ui.setDensity('compact');

			expect(ui.value.preferences.density).toBe('compact');
			expect(localStorageMock.setItem).toHaveBeenCalled();
		});

		it('should set all density options', () => {
			const densities = ['compact', 'comfortable', 'spacious'] as const;

			densities.forEach(density => {
				ui.setDensity(density);
				expect(ui.value.preferences.density).toBe(density);
			});
		});

		it('should set animations and save to localStorage', () => {
			ui.setAnimations(false);

			expect(ui.value.preferences.animations).toBe(false);
			expect(localStorageMock.setItem).toHaveBeenCalled();

			ui.setAnimations(true);
			expect(ui.value.preferences.animations).toBe(true);
		});

		it('should set page size with clamping', () => {
			// Test normal value
			ui.setPageSize(50);
			expect(ui.value.preferences.pageSize).toBe(50);

			// Test minimum clamping
			ui.setPageSize(1);
			expect(ui.value.preferences.pageSize).toBe(5);

			// Test maximum clamping
			ui.setPageSize(200);
			expect(ui.value.preferences.pageSize).toBe(100);
		});

		it('should save preferences to localStorage after changes', () => {
			ui.setPageSize(30);

			expect(localStorageMock.setItem).toHaveBeenCalled();
		});
	});

	describe('Complex State Interactions', () => {
		it('should handle multiple notifications with different durations', () => {
			// Add permanent notification
			ui.addNotification({ type: 'error' as const, title: 'Error', duration: 0 });
			
			// Add temporary notification
			ui.addNotification({ type: 'success' as const, title: 'Success', duration: 1000 });

			expect(ui.value.notifications).toHaveLength(2);

			// Fast forward to remove temporary notification
			vi.advanceTimersByTime(1000);

			expect(ui.value.notifications).toHaveLength(1);
			expect(ui.value.notifications[0].type).toBe('error');
		});

		it('should handle multiple modals stacking', () => {
			const id1 = ui.openModal({ component: 'Modal1' });
			const id2 = ui.openModal({ component: 'Modal2' });
			const id3 = ui.openModal({ component: 'Modal3' });

			expect(ui.value.modals).toHaveLength(3);

			// Close middle modal
			ui.closeModal(id2);

			expect(ui.value.modals).toHaveLength(2);
			expect(ui.value.modals.map((m: any) => m.id)).toEqual([id1, id3]);
		});

		it('should handle sidebar state while loading', () => {
			ui.setLoading(true, 'Loading...');
			ui.openSidebar();
			ui.toggleSidebarPin();

			expect(ui.value.isLoading).toBe(true);
			expect(ui.value.sidebar.isOpen).toBe(true);
			expect(ui.value.sidebar.isPinned).toBe(true);
		});
	});

	describe('Edge Cases', () => {
		it('should handle removing non-existent notification', () => {
			ui.addNotification({ type: 'info' as const, title: 'Test' });
			
			expect(() => ui.removeNotification('nonexistent')).not.toThrow();
			expect(ui.value.notifications).toHaveLength(1);
		});

		it('should handle closing non-existent modal', () => {
			ui.openModal({ component: 'TestModal' });
			
			expect(() => ui.closeModal('nonexistent')).not.toThrow();
			expect(ui.value.modals).toHaveLength(1);
		});

		it('should handle setting loading with empty message', () => {
			ui.setLoading(true, '');

			expect(ui.value.isLoading).toBe(true);
			expect(ui.value.loadingMessage).toBe('');
		});

		it('should preserve other preferences when updating one', () => {
			ui.setDensity('compact');
			ui.setAnimations(false);
			
			ui.setPageSize(50);

			expect(ui.value.preferences.density).toBe('compact');
			expect(ui.value.preferences.animations).toBe(false);
			expect(ui.value.preferences.pageSize).toBe(50);
		});
	});

	describe('Non-browser Environment', () => {
		it('should handle preferences saving in non-browser environment', async () => {
			// Mock browser as false
			vi.doMock('$app/environment', () => ({
				browser: false,
				dev: false
			}));

			// Reimport with non-browser environment
			vi.resetModules();
			const module = await import('./ui.svelte');
			const nonBrowserUI = module.ui;

			// Should not crash when trying to save preferences
			expect(() => nonBrowserUI.setDensity('compact')).not.toThrow();
			expect(() => nonBrowserUI.toggleSidebarPin()).not.toThrow();
			
			// localStorage should not be called
			expect(localStorageMock.setItem).not.toHaveBeenCalled();
		});
	});
});