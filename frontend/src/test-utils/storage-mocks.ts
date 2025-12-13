import { vi } from 'vitest';

/**
 * Mock localStorage implementation for testing.
 * Provides a clean, resettable mock that can be used across all tests.
 */
export interface MockStorage {
	getItem: ReturnType<typeof vi.fn>;
	setItem: ReturnType<typeof vi.fn>;
	removeItem: ReturnType<typeof vi.fn>;
	clear: ReturnType<typeof vi.fn>;
	/** Internal storage map for testing assertions */
	_store: Map<string, string>;
}

/**
 * Creates a new localStorage mock with an internal store.
 * The mock tracks all calls and maintains an internal store for realistic behavior.
 */
export function createLocalStorageMock(): MockStorage {
	const store = new Map<string, string>();

	const mock: MockStorage = {
		_store: store,
		getItem: vi.fn((key: string) => store.get(key) ?? null),
		setItem: vi.fn((key: string, value: string) => {
			store.set(key, value);
		}),
		removeItem: vi.fn((key: string) => {
			store.delete(key);
		}),
		clear: vi.fn(() => {
			store.clear();
		}),
	};

	return mock;
}

/**
 * Shared localStorage mock instance.
 * Use this in tests that need localStorage.
 */
export const localStorageMock = createLocalStorageMock();

/**
 * Installs the localStorage mock on the window object.
 * Call this in your test setup or beforeEach.
 */
export function installLocalStorageMock(mock: MockStorage = localStorageMock): void {
	Object.defineProperty(window, 'localStorage', {
		value: mock,
		writable: true,
		configurable: true,
	});
}

/**
 * Resets the localStorage mock to its initial state.
 * Clears all stored data and resets all mock function call histories.
 */
export function resetLocalStorageMock(mock: MockStorage = localStorageMock): void {
	mock._store.clear();
	mock.getItem.mockClear();
	mock.setItem.mockClear();
	mock.removeItem.mockClear();
	mock.clear.mockClear();
}

/**
 * Sets up the localStorage mock with initial data.
 * Useful for testing scenarios where localStorage already has data.
 */
export function seedLocalStorage(
	data: Record<string, unknown>,
	mock: MockStorage = localStorageMock
): void {
	for (const [key, value] of Object.entries(data)) {
		const stringValue = typeof value === 'string' ? value : JSON.stringify(value);
		mock._store.set(key, stringValue);
	}
}

/**
 * Creates a sessionStorage mock (same interface as localStorage).
 */
export function createSessionStorageMock(): MockStorage {
	return createLocalStorageMock();
}

export const sessionStorageMock = createSessionStorageMock();

export function installSessionStorageMock(mock: MockStorage = sessionStorageMock): void {
	Object.defineProperty(window, 'sessionStorage', {
		value: mock,
		writable: true,
		configurable: true,
	});
}

export function resetSessionStorageMock(mock: MockStorage = sessionStorageMock): void {
	resetLocalStorageMock(mock);
}
