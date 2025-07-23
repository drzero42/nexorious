import { describe, expect, it, vi } from 'vitest';
import { resolveImageUrl } from './image-url';

// Mock the config module
vi.mock('$lib/env', () => ({
  config: {
    apiUrl: 'http://localhost:8000/api',
    staticUrl: 'http://localhost:8000'
  }
}));

describe('resolveImageUrl', () => {
  it('should return empty string for null or undefined input', () => {
    expect(resolveImageUrl(null)).toBe('');
    expect(resolveImageUrl(undefined)).toBe('');
    expect(resolveImageUrl('')).toBe('');
  });

  it('should convert relative /static/ URLs to absolute URLs', () => {
    const relativeUrl = '/static/cover_art/121688.jpg';
    const expected = 'http://localhost:8000/static/cover_art/121688.jpg';
    expect(resolveImageUrl(relativeUrl)).toBe(expected);
  });

  it('should return absolute URLs unchanged', () => {
    const absoluteUrl = 'https://images.igdb.com/igdb/image/upload/cover.jpg';
    expect(resolveImageUrl(absoluteUrl)).toBe(absoluteUrl);
  });

  it('should return other relative URLs unchanged', () => {
    const relativeUrl = '/other/path/image.jpg';
    expect(resolveImageUrl(relativeUrl)).toBe(relativeUrl);
  });

  it('should handle URLs with query parameters', () => {
    const urlWithParams = '/static/cover_art/121688.jpg?v=1';
    const expected = 'http://localhost:8000/static/cover_art/121688.jpg?v=1';
    expect(resolveImageUrl(urlWithParams)).toBe(expected);
  });
});