import { fileURLToPath } from 'node:url'
import { defineConfig } from 'vitest/config'

const layerApp = fileURLToPath(new URL('./layer/app', import.meta.url))

export default defineConfig({
  resolve: {
    alias: {
      '~': layerApp,
      '@': layerApp,
    },
  },
  test: {
    environment: 'node',
    include: ['layer/tests/unit/**/*.spec.ts'],
    globals: true,
  },
})
