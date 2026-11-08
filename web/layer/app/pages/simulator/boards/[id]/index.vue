<script setup lang="ts">
import { computed, onMounted } from 'vue'
import ChipPanel from '../../../../components/ChipPanel.vue'
import DeviceGroup from '../../../../components/DeviceGroup.vue'
import FaultPanel from '../../../../components/FaultPanel.vue'
import PinMatrix from '../../../../components/PinMatrix.vue'
import { useSimulator } from '../../../../composables/useSimulator'
import type { DeviceGroup as Group, DeviceState } from '../../../../types/simulator'


const route = useRoute()
const sim = useSimulator()

const boardID = computed(() => String(route.params.id))
const board = computed(
  () =>
    sim.boards.value.find(
      (item) => item.id === boardID.value || item.name === boardID.value,
    ) ?? null,
)

const groups: Array<{ key: Group; title: string; icon: string }> = [
  { key: 'inputs', title: 'Buttons, captors & PIR', icon: 'i-lucide-hand' },
  { key: 'outputs', title: 'Relays, LEDs & buzzers', icon: 'i-lucide-toggle-right' },
  { key: 'actuators', title: 'Motors, servos & PWM', icon: 'i-lucide-sliders-horizontal' },
  { key: 'sensors', title: 'Sensors & ADC', icon: 'i-lucide-gauge' },
  { key: 'displays', title: 'Displays', icon: 'i-lucide-monitor' },
  { key: 'expanders', title: 'Expanders', icon: 'i-lucide-circuit-board' },
  { key: 'extras', title: 'Extra values & functions', icon: 'i-lucide-braces' },
]

const devicesByGroup = computed(() => {
  const map = new Map<string, DeviceState[]>()
  for (const device of board.value ? Object.values(board.value.devices) : []) {
    const key = device.group || 'extras'
    map.set(key, [...(map.get(key) ?? []), device])
  }
  for (const list of map.values()) {
    list.sort((a, b) => a.label.localeCompare(b.label))
  }
  return map
})

const faults = computed(() => (board.value?.fault.active ? 1 : 0))

onMounted(async () => {
  if (!sim.boards.value.length) {
    try {
      await sim.fetchSnapshot()
    } catch {
      // Error alert is rendered from the shared composable state.
    }
  }
})
</script>

<template>
  <div class="space-y-6">
    <div v-if="board" class="flex flex-wrap items-start justify-between gap-3">
      <div>
        <div class="flex flex-wrap items-center gap-2">
          <UButton
            to="/simulator"
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
          {{ board.id }} · {{ board.platform || 'simulated' }} ·
          {{ board.connected ? 'connected' : 'disconnected' }}
        </p>
      </div>
      <div class="flex flex-wrap items-center gap-2">
        <UButton
          size="sm"
          color="neutral"
          variant="soft"
          icon="i-lucide-activity"
          :to="`/simulator/boards/${board.id}/timeline`"
        >
          Timeline
        </UButton>
        <UButton
          size="sm"
          color="neutral"
          variant="soft"
          icon="i-lucide-network"
          :to="`/simulator/boards/${board.id}/topology`"
        >
          Topology
        </UButton>
        <UButton
          size="sm"
          color="neutral"
          variant="soft"
          icon="i-lucide-workflow"
          :to="`/simulator/boards/${board.id}/scenarios`"
        >
          Scenarios
        </UButton>
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
        v-for="group in groups"
        :key="group.key"
        :title="group.title"
        :icon="group.icon"
        :board-id="board.id"
        :devices="devicesByGroup.get(group.key) ?? []"
      />

      <ChipPanel :board-id="board.id" />
      <PinMatrix :board="board" />
    </template>
  </div>
</template>
