import '@testing-library/jest-dom';
import { vi } from 'vitest';

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

// Mock custom events
global.CustomEvent = class MockCustomEvent extends Event {
	detail: any;
	constructor(type: string, eventInitDict?: CustomEventInit) {
		super(type, eventInitDict);
		this.detail = eventInitDict?.detail;
	}
} as any;