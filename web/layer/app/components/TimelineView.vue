<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { useSimulator } from '../composables/useSimulator'
import type { TraceKind, TraceRecord, WaveformSeries } from '../types/simulator'

const props = defineProps<{ boardId: string }>()
const sim = useSimulator()

const kinds: TraceKind[] = [
  'pin-read',
  'pin-write',
  'pwm',
  'servo',
  'analog',
  'bus-txn',
  'value-read',
  'value-write',
  'function-call',
  'driver-event',
  'fault',
  'step',
]
const selectedKinds = ref<TraceKind[]>([...kinds])
const target = ref('')
const paused = ref(false)
const follow = ref(true)
const frozen = ref<TraceRecord[]>([])
const waveforms = ref<WaveformSeries[]>([])
const tableWrap = ref<HTMLElement | null>(null)
const error = ref('')
const loading = ref(false)

const boardID = computed(() => props.boardId)

const source = computed(() =>
  paused.value ? frozen.value : sim.trace.value.filter((record) => record.boardId === boardID.value),
)

const records = computed(() =>
  source.value
    .filter((record) => selectedKinds.value.includes(record.kind))
    .filter((record) => !target.value || record.target === target.value)
    .slice(-1000),
)

const targets = computed(() => {
  const seen = new Set<string>()
  for (const record of sim.trace.value) {
    if (record.target) {
      seen.add(record.target)
    }
  }
  return [...seen].sort()
})

const steps = computed(() => records.value.filter((record) => record.kind === 'step'))

watch(
  () => records.value.length,
  async () => {
    if (!follow.value) {
      return
    }
    await nextTick()
    if (tableWrap.value) {
      tableWrap.value.scrollTop = tableWrap.value.scrollHeight
    }
  },
)

const columns = [
  { accessorKey: 'seq', header: 'Seq' },
  { accessorKey: 'timestamp', header: 'Time' },
  { accessorKey: 'kind', header: 'Kind' },
  { accessorKey: 'target', header: 'Target' },
  { accessorKey: 'operation', header: 'Op' },
  { accessorKey: 'value', header: 'Value' },
  { accessorKey: 'source', header: 'Source' },
  { accessorKey: 'durationNs', header: 'Duration' },
  { accessorKey: 'error', header: 'Error' },
]

function togglePause() {
  paused.value = !paused.value
  if (paused.value) {
    frozen.value = sim.trace.value.slice()
  }
}

async function loadTrace() {
  loading.value = true
  try {
    await sim.fetchTrace(boardID.value, { kinds: selectedKinds.value })
    if (paused.value) {
      frozen.value = sim.trace.value.slice()
    }
    error.value = ''
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    loading.value = false
  }
}

async function loadWaveform() {
  const pins = [...new Set(records.value.map((record) => record.target).filter(Boolean))].slice(0, 8)
  try {
    waveforms.value = await sim.fetchWaveform(boardID.value, {
      pins: pins as string[],
      maxPoints: 500,
    })
    error.value = ''
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  }
}

function formatValue(value: unknown): string {
  if (value === undefined || value === null) {
    return '—'
  }
  if (typeof value === 'object') {
    return JSON.stringify(value)
  }
  return String(value)
}

function waveformPoints(series: WaveformSeries): string {
  if (!series.points.length) {
    return ''
  }
  const values = series.points.map((point) => point.value)
  const min = Math.min(...values)
  const max = Math.max(...values)
  const span = max - min || 1
  return series.points
    .map((point, index) => {
      const x = (index / Math.max(1, series.points.length - 1)) * 100
      const y = 30 - ((point.value - min) / span) * 28
      return `${x.toFixed(2)},${y.toFixed(2)}`
    })
    .join(' ')
}

onMounted(async () => {
  await loadTrace()
  await loadWaveform()
})
</script>

<template>
  <section class="space-y-4">
    <div class="flex flex-wrap items-center gap-2">
      <UIcon name="i-lucide-activity" class="size-4 text-primary" />
      <h2 class="text-base font-semibold">I/O timeline</h2>
      <UBadge color="neutral" variant="subtle">{{ records.length }}</UBadge>
      <UButton
        size="xs"
        color="neutral"
        variant="soft"
        :icon="paused ? 'i-lucide-play' : 'i-lucide-pause'"
        @click="togglePause"
      >
        {{ paused ? 'Resume' : 'Pause' }}
      </UButton>
      <USwitch v-model="follow" label="Follow" />
      <UButton
        size="xs"
        color="neutral"
        variant="ghost"
        icon="i-lucide-refresh-cw"
        :loading="loading"
        @click="loadTrace"
      >
        Refresh
      </UButton>
      <UButton size="xs" color="neutral" variant="ghost" icon="i-lucide-waves" @click="loadWaveform">
        Waveform
      </UButton>
    </div>

    <div class="flex flex-wrap items-center gap-2">
      <USelect
        v-model="target"
        :items="[{ label: 'All targets', value: '' }, ...targets.map((value) => ({ label: value, value }))]"
        value-key="value"
        size="xs"
        class="w-48"
      />
      <div class="flex flex-wrap gap-1">
        <UButton
          v-for="kind in kinds"
          :key="kind"
          size="xs"
          :color="selectedKinds.includes(kind) ? 'primary' : 'neutral'"
          :variant="selectedKinds.includes(kind) ? 'subtle' : 'ghost'"
          @click="
            selectedKinds = selectedKinds.includes(kind)
              ? selectedKinds.filter((item) => item !== kind)
              : [...selectedKinds, kind]
          "
        >
          {{ kind }}
        </UButton>
      </div>
    </div>

    <UAlert v-if="error" color="error" icon="i-lucide-circle-alert" :title="error" />

    <UCard v-if="steps.length">
      <template #header>
        <div class="flex items-center gap-2">
          <UIcon name="i-lucide-flag" class="size-4 text-primary" />
          <h3 class="font-medium">Step markers</h3>
          <UBadge color="neutral" variant="subtle">{{ steps.length }}</UBadge>
        </div>
      </template>
      <div class="flex flex-wrap gap-2">
        <UBadge
          v-for="step in steps"
          :key="step.seq"
          color="info"
          variant="subtle"
          :title="new Date(step.timestamp).toLocaleTimeString()"
        >
          {{ step.target }} · {{ step.operation || step.value }}
        </UBadge>
      </div>
    </UCard>

    <div v-if="waveforms.length" class="grid gap-3 md:grid-cols-2">
      <UCard v-for="series in waveforms" :key="series.pin">
        <template #header>
          <div class="flex items-center gap-2">
            <span class="font-mono text-sm">{{ series.pin }}</span>
            <UBadge v-if="series.downsampled" color="neutral" variant="subtle" size="sm">
              downsampled
            </UBadge>
            <span class="ml-auto text-xs text-muted">
              {{ series.points.length }} points
            </span>
          </div>
        </template>
        <svg viewBox="0 0 100 32" preserveAspectRatio="none" class="h-24 w-full">
          <polyline
            v-if="series.points.length"
            :points="waveformPoints(series)"
            fill="none"
            stroke="currentColor"
            stroke-width="0.8"
            class="text-primary"
          />
        </svg>
      </UCard>
    </div>

    <UCard>
      <div ref="tableWrap" class="max-h-[32rem] overflow-auto">
        <UTable :data="records" :columns="columns">
          <template #timestamp-cell="{ row }">
            <span class="text-xs text-muted">
              {{ new Date(row.original.timestamp).toLocaleTimeString() }}
            </span>
          </template>
          <template #kind-cell="{ row }">
            <UBadge color="neutral" variant="subtle" size="sm">{{ row.original.kind }}</UBadge>
          </template>
          <template #target-cell="{ row }">
            <span class="font-mono text-xs">{{ row.original.target || '—' }}</span>
          </template>
          <template #value-cell="{ row }">
            <span class="font-mono text-xs">{{ formatValue(row.original.value) }}</span>
          </template>
          <template #durationNs-cell="{ row }">
            <span class="text-xs text-muted">{{ row.original.durationNs ?? 0 }}ns</span>
          </template>
          <template #error-cell="{ row }">
            <span class="text-xs text-error">{{ row.original.error || '' }}</span>
          </template>
        </UTable>
      </div>
    </UCard>
  </section>
</template>
