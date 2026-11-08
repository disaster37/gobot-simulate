<script setup lang="ts">
import { computed } from 'vue'
import { useSimulator } from '../composables/useSimulator'
import type { BoardState, PinMode, PinState } from '../types/simulator'

const props = defineProps<{ board: BoardState }>()
const sim = useSimulator()

const columns = [
  { accessorKey: 'pin', header: 'Pin' },
  { accessorKey: 'label', header: 'Label' },
  { accessorKey: 'mode', header: 'Mode' },
  { accessorKey: 'pullup', header: 'Pull-up' },
  { accessorKey: 'invertInitial', header: 'Inverted' },
  { accessorKey: 'digitalValue', header: 'Digital' },
  { accessorKey: 'byteValue', header: 'Byte' },
  { accessorKey: 'analogValue', header: 'Analog' },
  { accessorKey: 'updatedAt', header: 'Updated' },
]

const rows = computed(() =>
  Object.values(props.board.pins).sort((a, b) =>
    a.pin.localeCompare(b.pin, undefined, { numeric: true }),
  ),
)

function modeColor(mode: PinMode) {
  switch (mode) {
    case 'output':
      return 'primary'
    case 'input-pullup':
      return 'warning'
    case 'pwm':
    case 'servo':
    case 'analog':
      return 'info'
    default:
      return 'neutral'
  }
}

function formatTime(value: string) {
  return new Date(value).toLocaleTimeString()
}

async function toggle(pin: PinState) {
  await sim.setPin(props.board.id, pin.pin, { digitalValue: pin.digitalValue ? 0 : 1 })
}
</script>

<template>
  <UCard>
    <template #header>
      <div class="flex items-center gap-2">
        <UIcon name="i-lucide-table-properties" class="size-4 text-primary" />
        <h2 class="font-semibold">Pin matrix</h2>
        <UBadge color="neutral" variant="subtle">{{ rows.length }}</UBadge>
      </div>
    </template>

    <UTable :data="rows" :columns="columns" class="w-full">
      <template #mode-cell="{ row }">
        <UBadge :color="modeColor(row.original.mode)" variant="subtle">
          {{ row.original.mode }}
        </UBadge>
      </template>
      <template #pullup-cell="{ row }">
        <UBadge :color="row.original.pullup ? 'warning' : 'neutral'" variant="subtle">
          {{ row.original.pullup ? 'on' : 'off' }}
        </UBadge>
      </template>
      <template #invertInitial-cell="{ row }">
        <UBadge :color="row.original.invertInitial ? 'info' : 'neutral'" variant="subtle">
          {{ row.original.invertInitial ? 'yes' : 'no' }}
        </UBadge>
      </template>
      <template #digitalValue-cell="{ row }">
        <UButton
          size="xs"
          :color="row.original.digitalValue ? 'success' : 'neutral'"
          variant="subtle"
          @click="toggle(row.original)"
        >
          {{ row.original.digitalValue }}
        </UButton>
      </template>
      <template #updatedAt-cell="{ row }">
        <span class="text-xs text-muted">{{ formatTime(row.original.updatedAt) }}</span>
      </template>
    </UTable>
  </UCard>
</template>
