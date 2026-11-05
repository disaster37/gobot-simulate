export default defineNuxtConfig({
  compatibilityDate: '2025-01-01',
  buildId: 'gobot-simulate',
  ssr: false,
  modules: ['@nuxt/ui'],
  css: ['~/assets/css/main.css'],
  icon: {
    provider: 'clientBundle',
    clientBundle: {
      scan: true,
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
