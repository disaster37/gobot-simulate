<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useSimulator } from '../composables/useSimulator'
import type { DeviceFault, DeviceState, DisplaySnapshot } from '../types/simulator'
import DisplayView from './DisplayView.vue'
import WiringBadges from './WiringBadges.vue'

const props = defineProps<{ boardId: string; device: DeviceState }>()
const sim = useSimulator()

const held = ref(false)
const numericValue = ref(0)
const parameters = ref('')
const returnCode = ref(0)
const functionResult = ref<number | null>(null)
const selectedFault = ref<DeviceFault>('stuck-high')
const actionError = ref('')
const rgb = ref({ red: 0, green: 0, blue: 0 })

const faults: DeviceFault[] = [
  'stuck-high',
  'stuck-low',
  'welded-contact',
  'open-coil',
  'disconnected-load',
  'noisy',
]
const busFaults: DeviceFault[] = ['bus-nak', 'bus-timeout']
const availableFaults = computed(() =>
  ['expander', 'adc', 'imu', 'temperature', 'rfid', 'display'].includes(props.device.kind)
    ? [...faults, ...busFaults]
    : faults,
)

const isButton = computed(() =>
  ['button', 'captor', 'pir'].includes(props.device.kind),
)
const isOutput = computed(() =>
  ['relay', 'led', 'rgb-led', 'buzzer', 'motor', 'stepper', 'servo', 'pwm', 'direct-pin', 'analog-actuator'].includes(
    props.device.kind,
  ),
)
const isSensor = computed(() =>
  ['sensor', 'analog', 'temperature', 'distance', 'imu', 'adc', 'rfid', 'expander'].includes(
    props.device.kind,
  ),
)
const isDisplay = computed(() => props.device.kind === 'display')
const isRGB = computed(() => props.device.kind === 'rgb-led')
const showNumeric = computed(
  () => isOutput.value && !['relay', 'buzzer', 'rgb-led'].includes(props.device.kind),
)
const isValue = computed(() => props.device.kind === 'value')
const isFunction = computed(() => props.device.kind === 'function')
const booleanValue = computed(() => typeof props.device.state.value === 'boolean')
const maxValue = computed(() => props.device.capabilities.maxValue ?? 1023)
const minValue = computed(() => props.device.capabilities.minValue ?? 0)
const warnings = computed(() =>
  (props.device.warnings ?? []).filter((warning) => warning.startsWith('WIRING_MISMATCH')),
)
const pinLevel = computed(() => Number(props.device.state.pinValue ?? 0))
const displaySnapshot = computed(() => {
  const value = props.device.state.display
  if (!value || typeof value !== 'object') {
    return undefined
  }
  return value as DisplaySnapshot
})

watch(
  () => props.device,
  (device) => {
    held.value = device.active
    parameters.value = String(device.state.lastParameters ?? '')
    returnCode.value = Number(device.state.returnCode ?? 0)
    if (device.kind === 'servo') {
      numericValue.value = Number(device.state.angle ?? 0)
    } else if (device.kind === 'pwm' || device.kind === 'direct-pin') {
      numericValue.value = Number(device.state.byteValue ?? 0)
    } else {
      numericValue.value = Number(
        device.state.speed ?? device.state.value ?? device.state.analogValue ?? 0,
      )
    }
  },
  { immediate: true, deep: true },
)

async function run(action: () => Promise<unknown>) {
  actionError.value = ''
  try {
    await action()
  } catch (err) {
    actionError.value = err instanceof Error ? err.message : String(err)
  }
}

async function command(action: string, value?: number) {
  await run(() => sim.commandDevice(props.boardId, props.device.id, action, value))
}

async function press() {
  held.value = true
  await run(() => sim.pressDevice(props.boardId, props.device.id))
}

async function release() {
  held.value = false
  await run(() => sim.releaseDevice(props.boardId, props.device.id))
}

async function pulse() {
  await run(() => sim.pressDevice(props.boardId, props.device.id, 200))
}

async function setLatch(active: boolean) {
  await run(() => sim.patchDevice(props.boardId, props.device.id, { active }))
}

async function applyNumeric() {
  await command(commandForKind(), numericValue.value)
}

async function applyRGB() {
  const packed = (rgb.value.red << 16) | (rgb.value.green << 8) | rgb.value.blue
  await command('setrgb', packed)
}

async function applyBoolean(value: boolean) {
  await run(() => sim.setValue(props.boardId, props.device.valueName ?? props.device.name, value))
}

async function injectFault() {
  await run(() => sim.setDeviceFault(props.boardId, props.device.id, selectedFault.value))
}

async function clearFaults() {
  await run(() => sim.clearDeviceFaults(props.boardId, props.device.id))
}

async function invoke() {
  await run(async () => {
    const result = await sim.callFunction(
      props.boardId,
      props.device.functionName ?? props.device.name,
      parameters.value,
      returnCode.value,
    )
    functionResult.value = result.result
  })
}

function commandForKind(): string {
  switch (props.device.kind) {
    case 'servo':
      return 'angle'
    case 'motor':
      return 'speed'
    case 'stepper':
      return 'step'
    default:
      return 'brightness'
  }
}

function commandLabel(): string {
  switch (props.device.kind) {
    case 'servo':
      return 'Angle'
    case 'motor':
      return 'Speed'
    case 'stepper':
      return 'Steps'
    default:
      return 'Level'
  }
}
</script>

<template>
  <UCard>
    <div class="flex items-start justify-between gap-3">
      <div class="min-w-0">
        <div class="flex flex-wrap items-center gap-2">
          <h3 class="truncate font-medium">{{ device.label || device.name }}</h3>
          <UBadge color="neutral" variant="subtle">{{ device.kind }}</UBadge>
          <UBadge v-if="device.source" color="neutral" variant="outline" size="sm">
            {{ device.source }}
          </UBadge>
        </div>
        <p class="truncate text-xs text-muted">
          {{ device.id }} <span v-if="device.pins.length">· pin {{ device.pins.join(', ') }}</span>
        </p>
      </div>
      <UBadge :color="device.active ? 'success' : 'neutral'" variant="subtle">
        {{ device.active ? 'active' : 'idle' }}
      </UBadge>
    </div>

    <div class="mt-3">
      <WiringBadges :device="device" />
    </div>

    <UAlert
      v-if="warnings.length"
      class="mt-3"
      color="warning"
      icon="i-lucide-triangle-alert"
      title="Wiring mismatch"
      :description="warnings.join(' · ')"
    />

    <div class="mt-3 flex flex-wrap items-center gap-2 text-xs text-muted">
      <span>pin level {{ pinLevel }}</span>
      <span>expected idle {{ device.expectedIdleLevel }}</span>
      <span>driver default {{ device.defaultState }}</span>
    </div>

    <div v-if="isButton" class="mt-4 flex flex-wrap items-center gap-2">
      <UButton
        :color="held ? 'error' : 'primary'"
        icon="i-lucide-hand"
        @pointerdown.prevent="press"
        @pointerup.prevent="release"
        @pointerleave="held && release()"
        @keydown.space.prevent="press"
        @keyup.space.prevent="release"
        @keydown.enter.prevent="press"
        @keyup.enter.prevent="release"
      >
        Hold / Push
      </UButton>
      <USwitch :model-value="device.active" label="Latch" @update:model-value="setLatch" />
      <UButton color="neutral" variant="soft" size="sm" icon="i-lucide-zap" @click="pulse">
        Pulse 200ms
      </UButton>
    </div>

    <div v-else-if="isOutput" class="mt-4 space-y-3">
      <div class="flex flex-wrap items-center gap-2">
        <UButton
          v-if="!['motor', 'stepper'].includes(device.kind)"
          size="sm"
          icon="i-lucide-power"
          @click="command('on')"
        >
          On
        </UButton>
        <UButton
          v-if="!['motor', 'stepper'].includes(device.kind)"
          size="sm"
          color="neutral"
          variant="soft"
          icon="i-lucide-power-off"
          @click="command('off')"
        >
          Off
        </UButton>
        <UButton
          size="sm"
          color="neutral"
          variant="soft"
          icon="i-lucide-refresh-cw"
          @click="command('toggle')"
        >
          Toggle
        </UButton>
      </div>
      <div v-if="showNumeric" class="flex items-center gap-3">
        <USlider v-model="numericValue" :min="minValue" :max="maxValue" class="flex-1" />
        <UInput v-model.number="numericValue" type="number" class="w-24" />
        <UButton size="sm" @click="applyNumeric">{{ commandLabel() }}</UButton>
      </div>
      <div v-if="isRGB" class="flex flex-wrap items-center gap-2">
        <UInput v-model.number="rgb.red" type="number" :min="0" :max="255" class="w-24" placeholder="R" />
        <UInput v-model.number="rgb.green" type="number" :min="0" :max="255" class="w-24" placeholder="G" />
        <UInput v-model.number="rgb.blue" type="number" :min="0" :max="255" class="w-24" placeholder="B" />
        <UButton size="sm" @click="applyRGB">Set RGB</UButton>
      </div>
      <p v-if="showNumeric" class="text-xs text-muted">
        {{ minValue }}–{{ maxValue }} {{ device.capabilities.unit || '' }}
      </p>
    </div>

    <div v-else-if="isSensor" class="mt-4 space-y-3">
      <div class="flex items-center gap-3">
        <USlider v-model="numericValue" :min="minValue" :max="maxValue" class="flex-1" />
        <UInput v-model.number="numericValue" type="number" class="w-24" />
        <UButton size="sm" @click="applyNumeric">Set</UButton>
      </div>
      <p class="text-xs text-muted">
        current: {{ device.state.value ?? device.state.analogValue ?? 0 }}
        {{ device.capabilities.unit || '' }}
      </p>
    </div>

    <div v-else-if="isDisplay" class="mt-4">
      <DisplayView v-if="displaySnapshot" :display="displaySnapshot" :zoom="3" />
      <p v-else class="text-xs text-muted">Waiting for display data…</p>
    </div>

    <div v-else-if="isValue" class="mt-4 space-y-3">
      <USwitch
        v-if="booleanValue"
        :model-value="Boolean(device.state.value)"
        :label="device.valueName || device.name"
        @update:model-value="applyBoolean"
      />
      <div v-else class="flex items-center gap-3">
        <USlider v-model="numericValue" :min="0" :max="maxValue" class="flex-1" />
        <UInput v-model.number="numericValue" type="number" class="w-24" />
        <UButton size="sm" @click="applyNumeric">Apply</UButton>
      </div>
      <p class="text-xs text-muted">
        current: {{ device.state.value }} · {{ device.valueName || device.name }}
      </p>
    </div>

    <div v-else-if="isFunction" class="mt-4 space-y-3">
      <div class="flex flex-wrap items-center gap-2">
        <UInput v-model="parameters" placeholder="parameters" class="min-w-40 flex-1" />
        <UInput v-model.number="returnCode" type="number" class="w-28" />
        <UButton icon="i-lucide-play" @click="invoke">Invoke</UButton>
        <UBadge color="neutral" variant="subtle">
          calls {{ device.state.callCount ?? 0 }}
        </UBadge>
      </div>
      <p v-if="functionResult !== null" class="text-xs text-muted">result: {{ functionResult }}</p>
    </div>

    <p v-else class="mt-4 text-xs text-muted">{{ device.driver }}</p>

    <div class="mt-4 flex flex-wrap items-center gap-2 border-t border-default pt-3">
      <USelect
        v-model="selectedFault"
        :items="availableFaults"
        size="xs"
        class="w-44"
        aria-label="Device fault"
      />
      <UButton size="xs" color="error" variant="soft" @click="injectFault">Inject fault</UButton>
      <UButton size="xs" color="neutral" variant="ghost" @click="clearFaults">Clear faults</UButton>
    </div>

    <UAlert
      v-if="actionError"
      class="mt-3"
      color="error"
      icon="i-lucide-circle-alert"
      :title="actionError"
    />
  </UCard>
</template>
