export default defineNuxtConfig({
  compatibilityDate: '2025-01-01',
  extends: ['./layer'],
  buildId: 'gobot-simulate',
  ssr: false,
  icon: {
    provider: 'clientBundle',
    clientBundle: {
      scan: true,
    },
  },
  runtimeConfig: {
    public: {
      simulatorApiBase: process.env.NUXT_PUBLIC_SIMULATOR_API_BASE || '/api/v1',
      simulatorToken: process.env.NUXT_PUBLIC_SIMULATOR_TOKEN || '',
    },
  },
  nitro: {
    devProxy: {
      '/api/v1': {
        target: process.env.NUXT_PUBLIC_API_BASE || 'http://127.0.0.1:8080/api/v1',
        changeOrigin: true,
      },
    },
  },
  app: {
    baseURL: '/',
    head: {
      title: 'Gobot Simulator',
      meta: [{ name: 'viewport', content: 'width=device-width, initial-scale=1' }],
    },
  },
})
