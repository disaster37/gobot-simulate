<script setup lang="ts">
import { computed } from 'vue'
import type { SSEStatus } from '../composables/useSSE'

const props = defineProps<{
  status: SSEStatus
  seq: number
  dropped: number
  clients: number
}>()

defineEmits<{ reconnect: [] }>()

const color = computed(() => {
  switch (props.status) {
    case 'connected':
      return 'success'
    case 'connecting':
    case 'reconnecting':
      return 'warning'
    default:
      return 'error'
  }
})

const label = computed(() => {
  switch (props.status) {
    case 'connected':
      return 'Live'
    case 'connecting':
      return 'Connecting'
    case 'reconnecting':
      return 'Reconnecting'
    default:
      return 'Offline'
  }
})
</script>

<template>
  <div class="flex items-center gap-2 text-xs">
    <UBadge :color="color" variant="subtle">{{ label }}</UBadge>
    <span class="text-muted">seq {{ seq }}</span>
    <span v-if="clients" class="text-muted">{{ clients }} clients</span>
    <span v-if="dropped" class="text-warning">{{ dropped }} dropped</span>
    <UButton
      color="neutral"
      variant="ghost"
      size="xs"
      icon="i-lucide-refresh-cw"
      aria-label="Reconnect event stream"
      @click="$emit('reconnect')"
    />
  </div>
</template>
