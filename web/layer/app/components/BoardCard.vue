<script setup lang="ts">
import { computed } from 'vue'
import type { BoardState } from '~/types/simulator'

const props = defineProps<{ board: BoardState }>()

const deviceCount = computed(() => Object.keys(props.board.devices).length)
const pinCount = computed(() => Object.keys(props.board.pins).length)
const activeOutputs = computed(
  () =>
    Object.values(props.board.devices).filter(
      (device) => (device.kind === 'relay' || device.kind === 'led') && device.active,
    ).length,
)
</script>

<template>
  <UCard class="h-full">
    <div class="flex items-start justify-between gap-3">
      <div>
        <h2 class="text-lg font-semibold">{{ board.name }}</h2>
        <p class="text-sm text-muted">{{ board.id }} · {{ board.platform || 'simulated' }}</p>
      </div>
      <div class="flex flex-col items-end gap-1">
        <UBadge :color="board.online ? 'success' : 'neutral'" variant="subtle">
          {{ board.online ? 'online' : 'offline' }}
        </UBadge>
        <UBadge v-if="board.fault.active" color="error" variant="subtle">fault</UBadge>
      </div>
    </div>

    <div class="mt-4 grid grid-cols-3 gap-2 text-center text-sm">
      <div class="rounded-lg bg-elevated p-2">
        <div class="text-lg font-semibold">{{ deviceCount }}</div>
        <div class="text-xs text-muted">devices</div>
      </div>
      <div class="rounded-lg bg-elevated p-2">
        <div class="text-lg font-semibold">{{ pinCount }}</div>
        <div class="text-xs text-muted">pins</div>
      </div>
      <div class="rounded-lg bg-elevated p-2">
        <div class="text-lg font-semibold">{{ activeOutputs }}</div>
        <div class="text-xs text-muted">active</div>
      </div>
    </div>

    <template #footer>
      <div class="flex items-center justify-between">
        <span class="text-xs text-muted">reconnects: {{ board.reconnectCount }}</span>
        <UButton :to="`/boards/${board.id}`" size="sm" trailing-icon="i-lucide-arrow-right">
          Open board
        </UButton>
      </div>
    </template>
  </UCard>
</template>
