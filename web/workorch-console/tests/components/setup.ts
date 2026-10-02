import { cleanup } from '@testing-library/react';
import { afterEach, vi } from 'vitest';

Object.defineProperty(window, 'matchMedia', {
  writable: true,
  value: (query: string) => ({
    matches: false, media: query, onchange: null,
    addListener() {}, removeListener() {},
    addEventListener() {}, removeEventListener() {}, dispatchEvent: () => false,
  }),
});

// jsdom 无布局引擎，Ant Design 仅需要订阅接口。
globalThis.ResizeObserver = class {
  observe() {}
  unobserve() {}
  disconnect() {}
};
const getComputedStyle = window.getComputedStyle.bind(window);
window.getComputedStyle = element => getComputedStyle(element);

afterEach(() => {
  cleanup();
  localStorage.clear();
  vi.restoreAllMocks();
});
