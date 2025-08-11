import '@testing-library/jest-dom';
import { vi } from 'vitest';

// Mock Date and Date.now() to return consistent timestamps for tests
// Use this approach instead of fake timers to avoid breaking async operations
const originalDate = Date;

Date.now = vi.fn(() => new Date('2023-01-01T00:00:00.000Z').getTime());

// Override the global Date constructor
(global as any).Date = class extends originalDate {
  constructor(...args: [] | [string | number | Date]) {
    if (args.length === 0) {
      super('2023-01-01T00:00:00.000Z');
    } else {
      super(...args);
    }
  }
  
  static override now() {
    return new Date('2023-01-01T00:00:00.000Z').getTime();
  }
  
  static override UTC(...args: Parameters<typeof originalDate.UTC>) {
    return originalDate.UTC(...args);
  }
  
  static override parse(dateString: string) {
    return originalDate.parse(dateString);
  }
};

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
			message.includes('Network timeout occurred') ||
			message.includes('Failed to create game') ||
			message.includes('Failed to add game to collection') ||
			message.includes('Failed to update progress') ||
			message.includes('Failed to update game details') ||
			message.includes('Failed to load platforms and storefronts') ||
			message.includes('Failed to check setup status') ||
			message.includes('Failed to refresh auth') ||
			message.includes('Setup status check failed') ||
			message.includes('Refresh failed')
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