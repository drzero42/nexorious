import { vi } from 'vitest';

// Mock SvelteKit navigation functions
export const mockGoto = vi.fn();
export const mockInvalidateAll = vi.fn();

// Mock $app/stores
export const mockPage = {
  subscribe: vi.fn((callback) => {
    callback({
      params: {},
      url: new URL('http://localhost:3000'),
      route: { id: '/' },
      status: 200,
      error: null,
      data: {},
      form: null
    });
    return () => {};
  }),
  params: {},
  url: new URL('http://localhost:3000'),
  route: { id: '/' },
  status: 200,
  error: null,
  data: {},
  form: null
};

vi.mock('$app/stores', () => ({
  page: {
    subscribe: (callback: any) => {
      callback(mockPage);
      return () => {};
    }
  },
  navigating: {
    subscribe: (callback: any) => {
      callback(null);
      return () => {};
    }
  },
  updated: {
    subscribe: (callback: any) => {
      callback(false);
      return () => {};
    }
  },
  goto: mockGoto,
  mockGoto: mockGoto
}));

// Mock $app/navigation
vi.mock('$app/navigation', () => ({
  goto: mockGoto,
  invalidateAll: mockInvalidateAll,
  replaceState: vi.fn(),
  pushState: vi.fn(),
  beforeNavigate: vi.fn(),
  afterNavigate: vi.fn(),
  preloadData: vi.fn(),
  onNavigate: vi.fn(),
  page: mockPage,  // Add page export just in case
  mockGoto: mockGoto  // Export mockGoto for test access
}));

// Export everything needed for alias imports
export const page = mockPage;
export const goto = mockGoto;
export const invalidateAll = mockInvalidateAll;
export const navigating = {
  subscribe: (callback: any) => {
    callback(null);
    return () => {};
  }
};
export const updated = {
  subscribe: (callback: any) => {
    callback(false);
    return () => {};
  }
};

// Reset functions for test cleanup
export function resetNavigationMocks() {
  mockGoto.mockClear();
  mockInvalidateAll.mockClear();
  mockPage.params = {};
  mockPage.url = new URL('http://localhost:3000');
  mockPage.route = { id: '/' };
  mockPage.status = 200;
  mockPage.error = null;
  mockPage.data = {};
  mockPage.form = null;
}