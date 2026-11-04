<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { SimulatorEvent } from '~/types/simulator'

const props = defineProps<{ events: SimulatorEvent[] }>()

const source = ref<'all' | 'hardware-code' | 'ui' | 'simulation'>('all')
const paused = ref(false)
const clearedSeq = ref(0)
const frozen = ref<SimulatorEvent[]>([])

watch(
  () => props.events,
  (events) => {
    if (!paused.value) {
      frozen.value = events.slice(-100)
    }
  },
  { immediate: true, deep: true },
)

const rows = computed(() =>
  frozen.value
    .filter((event) => event.seq > clearedSeq.value)
    .filter((event) => source.value === 'all' || event.source === source.value)
    .slice()
    .reverse(),
)

const columns = [
  { accessorKey: 'seq', header: 'Seq' },
  { accessorKey: 'timestamp', header: 'Time' },
  { accessorKey: 'source', header: 'Source' },
  { accessorKey: 'boardId', header: 'Board' },
  { accessorKey: 'type', header: 'Type' },
  { accessorKey: 'target', header: 'Target' },
  { accessorKey: 'change', header: 'Change' },
]

function togglePause() {
  paused.value = !paused.value
  if (paused.value) {
    frozen.value = props.events.slice(-100)
  }
}

function clear() {
  clearedSeq.value = props.events.at(-1)?.seq ?? 0
}

function target(row: SimulatorEvent) {
  return row.pin || row.deviceId || row.valueName || '—'
}

function change(row: SimulatorEvent) {
  if (row.oldValue === undefined && row.newValue === undefined) {
    return row.type === 'heartbeat' ? '' : '—'
  }
  return `${JSON.stringify(row.oldValue ?? null)} → ${JSON.stringify(row.newValue ?? null)}`
}
</script>

<template>
  <UCard>
    <template #header>
      <div class="flex flex-wrap items-center gap-2">
        <UIcon name="i-lucide-activity" class="size-4 text-primary" />
        <h2 class="font-semibold">Live event feed</h2>
        <UBadge color="neutral" variant="subtle">{{ rows.length }}</UBadge>
        <div class="ml-auto flex items-center gap-2">
          <USelect
            v-model="source"
            :items="[
              { label: 'All sources', value: 'all' },
              { label: 'Hardware code', value: 'hardware-code' },
              { label: 'UI', value: 'ui' },
              { label: 'Simulation', value: 'simulation' },
            ]"
            value-key="value"
            size="xs"
          />
          <UButton
            size="xs"
            color="neutral"
            variant="soft"
            :icon="paused ? 'i-lucide-play' : 'i-lucide-pause'"
            @click="togglePause"
          >
            {{ paused ? 'Resume' : 'Pause' }}
          </UButton>
          <UButton size="xs" color="neutral" variant="ghost" icon="i-lucide-eraser" @click="clear">
            Clear
          </UButton>
        </div>
      </div>
    </template>

    <div class="max-h-96 overflow-auto">
      <UTable :data="rows" :columns="columns">
        <template #timestamp-cell="{ row }">
          <span class="text-xs text-muted">
            {{ new Date(row.original.timestamp).toLocaleTimeString() }}
          </span>
        </template>
        <template #source-cell="{ row }">
          <UBadge color="neutral" variant="subtle">{{ row.original.source }}</UBadge>
        </template>
        <template #type-cell="{ row }">
          <span class="font-mono text-xs">{{ row.original.type }}</span>
        </template>
        <template #target-cell="{ row }">
          <span class="font-mono text-xs">{{ target(row.original) }}</span>
        </template>
        <template #change-cell="{ row }">
          <span class="font-mono text-xs">{{ change(row.original) }}</span>
        </template>
      </UTable>
    </div>
  </UCard>
</template>
