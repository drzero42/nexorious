import { render, type RenderResult } from '@testing-library/svelte';
import { vi } from 'vitest';

// Re-export testing utilities for convenience
export * from '@testing-library/svelte';
export { vi } from 'vitest';

// Helper to render components with common setup
export function renderComponent(
  component: any,
  props: Record<string, any> = {},
  options: Record<string, any> = {}
): RenderResult<any> {
  return render(component, {
    props,
    ...options
  });
}

// Helper to simulate user interactions
export function createUserEvent() {
  return {
    click: async (element: Element) => {
      element.dispatchEvent(new MouseEvent('click', { bubbles: true }));
      
      // If clicking a submit button, also trigger form submission
      if (element instanceof HTMLButtonElement && element.type === 'submit') {
        const form = element.closest('form');
        if (form) {
          form.dispatchEvent(new Event('submit', { bubbles: true }));
        }
      }
    },
    type: async (element: Element, text: string) => {
      if (element instanceof HTMLInputElement || element instanceof HTMLTextAreaElement) {
        element.value = text;
        element.dispatchEvent(new Event('input', { bubbles: true }));
      }
    },
    keyDown: async (element: Element, key: string) => {
      element.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true }));
      
      // Handle Tab navigation
      if (key === 'Tab') {
        const focusableElements = Array.from(
          document.querySelectorAll(
            'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])'
          )
        ) as HTMLElement[];
        
        const currentIndex = focusableElements.indexOf(element as HTMLElement);
        const nextElement = focusableElements[currentIndex + 1];
        
        if (nextElement) {
          nextElement.focus();
        }
      }
    },
    submit: async (form: Element) => {
      form.dispatchEvent(new Event('submit', { bubbles: true }));
    }
  };
}

// Helper to wait for element to appear
export function waitForElement(
  container: HTMLElement,
  selector: string,
  timeout: number = 1000
): Promise<Element> {
  return new Promise((resolve, reject) => {
    const element = container.querySelector(selector);
    if (element) {
      resolve(element);
      return;
    }

    const observer = new MutationObserver(() => {
      const element = container.querySelector(selector);
      if (element) {
        observer.disconnect();
        resolve(element);
      }
    });

    observer.observe(container, {
      childList: true,
      subtree: true
    });

    setTimeout(() => {
      observer.disconnect();
      reject(new Error(`Element ${selector} not found within ${timeout}ms`));
    }, timeout);
  });
}

// Helper to simulate responsive design testing
export function setViewport(width: number, height: number) {
  Object.defineProperty(window, 'innerWidth', {
    writable: true,
    configurable: true,
    value: width
  });
  Object.defineProperty(window, 'innerHeight', {
    writable: true,
    configurable: true,
    value: height
  });
  
  // Trigger resize event
  window.dispatchEvent(new Event('resize'));
}

// Helper to simulate mobile viewport
export function setMobileViewport() {
  setViewport(375, 667); // iPhone dimensions
}

// Helper to simulate desktop viewport
export function setDesktopViewport() {
  setViewport(1920, 1080);
}

// Helper to simulate form submission
export async function submitForm(form: HTMLFormElement, userEvent: ReturnType<typeof createUserEvent>) {
  await userEvent.submit(form);
}

// Helper to fill form fields
export async function fillFormField(
  input: HTMLInputElement | HTMLTextAreaElement,
  value: string,
  userEvent: ReturnType<typeof createUserEvent>
) {
  await userEvent.type(input, value);
}

// Helper to test accessibility
export function testAccessibility(container: HTMLElement) {
  // Test for basic accessibility attributes
  const links = container.querySelectorAll('a');
  const buttons = container.querySelectorAll('button');
  const inputs = container.querySelectorAll('input, textarea, select');

  // Check links have accessible text
  links.forEach((link, index) => {
    const hasText = link.textContent?.trim() || link.getAttribute('aria-label') || link.getAttribute('title');
    if (!hasText) {
      console.warn(`Link ${index} missing accessible text`);
    }
  });

  // Check buttons have accessible text
  buttons.forEach((button, index) => {
    const hasText = button.textContent?.trim() || button.getAttribute('aria-label') || button.getAttribute('title');
    if (!hasText) {
      console.warn(`Button ${index} missing accessible text`);
    }
  });

  // Check form inputs have labels
  inputs.forEach((input, index) => {
    const hasLabel = input.getAttribute('aria-label') || 
                    input.getAttribute('aria-labelledby') ||
                    container.querySelector(`label[for="${input.id}"]`) ||
                    input.closest('label');
    if (!hasLabel) {
      console.warn(`Input ${index} missing label`);
    }
  });
}

// Helper to test keyboard navigation
export async function testKeyboardNavigation(
  container: HTMLElement,
  userEvent: ReturnType<typeof createUserEvent>
) {
  const focusableElements = container.querySelectorAll(
    'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])'
  );

  // Test Tab navigation
  for (let i = 0; i < focusableElements.length; i++) {
    const element = focusableElements[i] as HTMLElement;
    element.focus();
    await userEvent.keyDown(element, 'Tab');
  }

  // Test Enter key on buttons and links
  const interactiveElements = container.querySelectorAll('button, [href]');
  for (const element of interactiveElements) {
    await userEvent.keyDown(element as HTMLElement, 'Enter');
  }
}

// Helper to mock fetch responses
export function mockFetch(responses: Record<string, any>) {
  const mockFetch = vi.fn();
  
  Object.entries(responses).forEach(([url, response]) => {
    mockFetch.mockImplementation((requestUrl: string) => {
      if (requestUrl.includes(url)) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve(response),
          text: () => Promise.resolve(JSON.stringify(response))
        });
      }
      return Promise.reject(new Error(`No mock response for ${requestUrl}`));
    });
  });

  global.fetch = mockFetch;
  return mockFetch;
}

// Helper to restore fetch
export function restoreFetch() {
  delete (global as any).fetch;
}