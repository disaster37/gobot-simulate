export default defineNuxtConfig({
  compatibilityDate: '2025-01-01',
  extends: ['../layer'],
  ssr: false,
  buildId: 'gobot-simulate-layer-smoke',
  app: {
    head: {
      title: 'Host app consuming the simulator layer',
    },
  },
  runtimeConfig: {
    public: {
      simulatorApiBase: '/api/v1',
      simulatorToken: '',
    },
  },
})
