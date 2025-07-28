import { vi } from 'vitest';

// Mock Svelte 5 runes for testing environment
// This prevents "proxy is not a function" errors when stores use $state
// This file runs before other setup files to ensure runes are available early

// Create a mock state function that doesn't rely on Svelte's proxy implementation
const createMockState = (initialValue: any) => {
	if (typeof initialValue === 'object' && initialValue !== null) {
		// For objects, return a simple reactive-like object
		const state = { ...initialValue };
		// Create a simple proxy that doesn't use Svelte's internals
		const handler = {
			get(target: any, prop: string | symbol) {
				return target[prop];
			},
			set(target: any, prop: string | symbol, value: any) {
				target[prop] = value;
				return true;
			}
		};
		
		try {
			return new Proxy(state, handler);
		} catch (e) {
			// If Proxy fails, just return the plain object
			return state;
		}
	}
	// For primitives, return a simple object wrapper
	return { value: initialValue };
};

// Override the global runes before any Svelte code loads
if (typeof globalThis !== 'undefined') {
	// Mock $state - the main rune causing issues
	const $stateMock = vi.fn(createMockState);

	// Set up the global $state function
	(globalThis as any).$state = $stateMock;

	// Mock other common Svelte 5 runes that might be used
	(globalThis as any).$derived = vi.fn((fn: () => any) => {
		try {
			return fn();
		} catch {
			return undefined;
		}
	});

	(globalThis as any).$effect = vi.fn(() => {
		// Effects don't need to do anything in tests
		return () => {}; // Return cleanup function
	});

	(globalThis as any).$props = vi.fn(() => ({}));

	// Also ensure these are available on the global object (for Node.js compatibility)
	if (typeof global !== 'undefined') {
		(global as any).$state = $stateMock;
		(global as any).$derived = (globalThis as any).$derived;
		(global as any).$effect = (globalThis as any).$effect;
		(global as any).$props = (globalThis as any).$props;
	}

	// Override any Svelte internals that might be causing issues
	try {
		// Try to prevent Svelte from using its internal proxy function
		if (typeof window !== 'undefined') {
			(window as any).$state = $stateMock;
		}
	} catch (e) {
		// Ignore any errors in setting up window globals
	}
}