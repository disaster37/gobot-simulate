<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import DeviceGroup from '~/components/DeviceGroup.vue'
import FaultPanel from '~/components/FaultPanel.vue'
import PinMatrix from '~/components/PinMatrix.vue'
import { useSimulator } from '~/composables/useSimulator'
import type { DeviceState } from '~/types/simulator'

const route = useRoute()
const sim = useSimulator()
const valueInputs = ref<Record<string, string>>({})

const boardID = computed(() => String(route.params.id))
const board = computed(
  () =>
    sim.boards.value.find(
      (item) => item.id === boardID.value || item.name === boardID.value,
    ) ?? null,
)

const devices = computed<DeviceState[]>(() =>
  board.value
    ? Object.values(board.value.devices).sort((a, b) => a.label.localeCompare(b.label))
    : [],
)

const contactDevices = computed(() =>
  devices.value.filter((device) => ['relay', 'led'].includes(device.kind)),
)
const inputDevices = computed(() =>
  devices.value.filter((device) => ['button', 'captor'].includes(device.kind)),
)
const analogDevices = computed(() =>
  devices.value.filter((device) => ['sensor', 'analog', 'pwm', 'servo'].includes(device.kind)),
)
const extraDevices = computed(() =>
  devices.value.filter((device) => ['value', 'function'].includes(device.kind)),
)

const faults = computed(() => (board.value?.fault.active ? 1 : 0))

watch(
  board,
  (current) => {
    if (!current) {
      return
    }
    for (const [name, entry] of Object.entries(current.values)) {
      if (valueInputs.value[name] === undefined) {
        valueInputs.value[name] = JSON.stringify(entry.value)
      }
    }
  },
  { immediate: true },
)

onMounted(async () => {
  if (!sim.boards.value.length) {
    try {
      await sim.fetchSnapshot()
    } catch {
      // Error alert is rendered from the shared composable state.
    }
  }
})

async function applyValue(name: string, fallback: unknown) {
  const raw = valueInputs.value[name]
  let value: unknown = fallback
  if (raw !== undefined && raw !== '') {
    try {
      value = JSON.parse(raw)
    } catch {
      value = raw
    }
  }
  if (board.value) {
    await sim.setValue(board.value.id, name, value)
  }
}
</script>

<template>
  <div class="space-y-6">
    <div v-if="board" class="flex flex-wrap items-start justify-between gap-3">
      <div>
        <div class="flex items-center gap-2">
          <UButton
            to="/"
            color="neutral"
            variant="ghost"
            size="xs"
            icon="i-lucide-arrow-left"
            aria-label="Back to dashboard"
          />
          <h1 class="text-2xl font-semibold">{{ board.name }}</h1>
          <UBadge :color="board.online ? 'success' : 'neutral'" variant="subtle">
            {{ board.online ? 'online' : 'offline' }}
          </UBadge>
          <UBadge v-if="faults" color="error" variant="subtle">fault</UBadge>
        </div>
        <p class="text-sm text-muted">
          {{ board.id }} · {{ board.platform || 'simulated' }} · {{ board.connected ? 'connected' : 'disconnected' }}
        </p>
      </div>
      <div class="text-right text-xs text-muted">
        <div>reconnects {{ board.reconnectCount }}</div>
        <div>updated {{ new Date(board.updatedAt).toLocaleTimeString() }}</div>
      </div>
    </div>

    <UAlert
      v-if="!board"
      color="warning"
      icon="i-lucide-search-x"
      title="Board not found"
      :description="`No board named ${boardID} is currently registered.`"
    />

    <template v-else>
      <FaultPanel :board="board" />

      <DeviceGroup
        title="Buttons & captors"
        icon="i-lucide-hand"
        :board-id="board.id"
        :devices="inputDevices"
      />
      <DeviceGroup
        title="Relays & LEDs"
        icon="i-lucide-toggle-right"
        :board-id="board.id"
        :devices="contactDevices"
      />
      <DeviceGroup
        title="Analog, PWM & servos"
        icon="i-lucide-sliders-horizontal"
        :board-id="board.id"
        :devices="analogDevices"
      />
      <DeviceGroup
        title="Extra values & functions"
        icon="i-lucide-braces"
        :board-id="board.id"
        :devices="extraDevices"
      />

      <UCard v-if="Object.keys(board.values).length">
        <template #header>
          <h2 class="font-semibold">Raw aREST values</h2>
        </template>
        <div class="grid gap-3 md:grid-cols-2">
          <div v-for="(entry, name) in board.values" :key="name" class="flex items-center gap-2">
            <UInput v-model="valueInputs[name]" :placeholder="JSON.stringify(entry.value)" class="flex-1" />
            <UButton size="sm" @click="applyValue(name, entry.value)">Apply</UButton>
          </div>
        </div>
      </UCard>

      <PinMatrix :board="board" />
    </template>
  </div>
</template>
