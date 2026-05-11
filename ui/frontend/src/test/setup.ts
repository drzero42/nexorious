import "@testing-library/jest-dom/vitest";
import { cleanup } from "@testing-library/react";
import { afterEach, beforeAll, afterAll, vi } from "vitest";
import type React from "react";
import { server } from "./mocks/server";

// Establish API mocking before all tests
beforeAll(() => {
  server.listen({ onUnhandledRequest: "error" });
});

// Reset any request handlers that we may add during the tests,
// so they don't affect other tests
afterEach(() => {
  cleanup();
  server.resetHandlers();
});

// Clean up after the tests are finished
afterAll(() => {
  server.close();
});

// Mock @tanstack/react-router for tests
vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>();
  return {
    ...actual,
    useNavigate: () => vi.fn(),
    useParams: () => ({}),
    useSearch: () => ({}),
    useRouterState: vi.fn((opts?: { select?: (s: unknown) => unknown }) => {
      const state = { location: { pathname: '/', search: '', hash: '' } };
      return opts?.select ? opts.select(state) : state;
    }),
    Link: ({ children, to, params, ...props }: { children: React.ReactNode; to: string; params?: Record<string, string>; [key: string]: unknown }) => {
      // eslint-disable-next-line @typescript-eslint/no-require-imports
      const React = require('react');
      let href = to;
      if (params && typeof params === 'object') {
        for (const [key, value] of Object.entries(params)) {
          href = href.replace(`$${key}`, String(value));
        }
      }
      return React.createElement('a', { href, ...props }, children);
    },
  };
});

// Mock localStorage
const localStorageMock = {
  getItem: vi.fn(),
  setItem: vi.fn(),
  removeItem: vi.fn(),
  clear: vi.fn(),
  length: 0,
  key: vi.fn(),
};

Object.defineProperty(window, "localStorage", {
  value: localStorageMock,
});

// Mock matchMedia
Object.defineProperty(window, "matchMedia", {
  writable: true,
  value: vi.fn().mockImplementation((query: string) => ({
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

// Mock ResizeObserver
class ResizeObserverMock {
  observe = vi.fn();
  unobserve = vi.fn();
  disconnect = vi.fn();
}

Object.defineProperty(window, "ResizeObserver", {
  value: ResizeObserverMock,
});

// Mock IntersectionObserver
class IntersectionObserverMock {
  observe = vi.fn();
  unobserve = vi.fn();
  disconnect = vi.fn();
  root = null;
  rootMargin = "";
  thresholds = [];
}

Object.defineProperty(window, "IntersectionObserver", {
  value: IntersectionObserverMock,
});

// Mock scrollIntoView for cmdk and other components that use it
Element.prototype.scrollIntoView = vi.fn();

// Mock hasPointerCapture for Radix UI components
Element.prototype.hasPointerCapture = vi.fn(() => false);
Element.prototype.setPointerCapture = vi.fn();
Element.prototype.releasePointerCapture = vi.fn();

// Export the localStorage mock for test access
export { localStorageMock };
