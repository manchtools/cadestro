import { defineConfig } from 'vitest/config';

export default defineConfig({
  test: {
    include: ['test/ts/**/*.test.ts', 'ts/**/*.test.ts'],
    environment: 'node',
    reporters: ['default'],
    coverage: {
      provider: 'v8',
      reporter: ['text', 'lcov'],
      include: ['ts/**/*.ts'],
      exclude: ['ts/**/*.test.ts'],
    },
  },
});
