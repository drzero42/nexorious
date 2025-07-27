import '@testing-library/jest-dom';
import { vi } from 'vitest';

// Store original console methods
const originalConsoleError = console.error;
const originalConsoleWarn = console.warn;

// Mock console.error to suppress expected error messages during tests
console.error = vi.fn((message, ...args) => {
	// Suppress specific expected error messages
	if (
		typeof message === 'string' && (
			message.includes('Search failed') ||
			message.includes('Platform load failed') ||
			message.includes('Progress update failed') ||
			message.includes('Rating update failed') ||
			message.includes('Collection add failed') ||
			message.includes('Game creation failed') ||
			message.includes('IGDB import failed') ||
			message.includes('Network timeout occurred')
		)
	) {
		return; // Suppress these expected errors in tests
	}
	// Still log other errors
	originalConsoleError(message, ...args);
});

// Mock console.warn to suppress JSDOM navigation warnings
console.warn = vi.fn((message, ...args) => {
	// Suppress JSDOM navigation warnings
	if (
		typeof message === 'string' && (
			message.includes('Not implemented: navigation') ||
			message.includes('Error: Not implemented: navigation')
		)
	) {
		return; // Suppress these JSDOM warnings
	}
	// Still log other warnings
	originalConsoleWarn(message, ...args);
});

// Mock browser globals
Object.defineProperty(window, 'matchMedia', {
	writable: true,
	value: vi.fn().mockImplementation(query => ({
		matches: false,
		media: query,
		onchange: null,
		addListener: vi.fn(),
		removeListener: vi.fn(),
		addEventListener: vi.fn(),
		removeEventListener: vi.fn(),
		dispatchEvent: vi.fn(),
	})),
});

// Mock navigator.serviceWorker
Object.defineProperty(navigator, 'serviceWorker', {
	writable: true,
	configurable: true,
	value: {
		register: vi.fn().mockResolvedValue({
			installing: null,
			waiting: null,
			active: null,
			addEventListener: vi.fn(),
			removeEventListener: vi.fn(),
		}),
		ready: Promise.resolve({
			installing: null,
			waiting: null,
			active: null,
		}),
		addEventListener: vi.fn(),
		removeEventListener: vi.fn(),
	},
});

// Mock navigator.onLine
Object.defineProperty(navigator, 'onLine', {
	writable: true,
	configurable: true,
	value: true,
});

// Mock navigator.standalone (iOS Safari)
Object.defineProperty(navigator, 'standalone', {
	writable: true,
	configurable: true,
	value: false,
});

// Mock window.location.reload
delete (window as any).location;
(window as any).location = {
	reload: vi.fn(),
	href: 'http://localhost:3000',
	origin: 'http://localhost:3000',
	pathname: '/',
	search: '',
	hash: ''
};

// Mock JSDOM navigation to prevent "Not implemented" errors
Object.defineProperty(window, 'navigation', {
	writable: true,
	configurable: true,
	value: {
		navigate: vi.fn(),
		addEventListener: vi.fn(),
		removeEventListener: vi.fn(),
	},
});

// Mock window.history for better navigation support
Object.defineProperty(window.history, 'pushState', {
	writable: true,
	configurable: true,
	value: vi.fn(),
});

Object.defineProperty(window.history, 'replaceState', {
	writable: true,
	configurable: true,
	value: vi.fn(),
});

// Mock custom events
global.CustomEvent = class MockCustomEvent extends Event {
	detail: any;
	constructor(type: string, eventInitDict?: CustomEventInit) {
		super(type, eventInitDict);
		this.detail = eventInitDict?.detail;
	}
} as any;