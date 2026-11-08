import { configureSimulator } from '../composables/useSimulator'

export default defineNuxtPlugin(() => {
  const config = useRuntimeConfig()
  configureSimulator({
    baseURL: String(config.public.simulatorApiBase || '/api/v1'),
    token: String(config.public.simulatorToken || ''),
  })
})
