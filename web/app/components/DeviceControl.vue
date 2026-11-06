<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useSimulator } from '~/composables/useSimulator'
import type { DeviceState } from '~/types/simulator'

const props = defineProps<{ boardId: string; device: DeviceState }>()
const sim = useSimulator()

const held = ref(false)
const numericValue = ref(0)
const parameters = ref('')
const returnCode = ref(0)
const functionResult = ref<number | null>(null)

const isButton = computed(() => props.device.kind === 'button' || props.device.kind === 'captor')
const isOutput = computed(() => props.device.kind === 'relay' || props.device.kind === 'led')
const isAnalog = computed(() =>
  ['sensor', 'analog', 'pwm', 'servo'].includes(props.device.kind),
)
const isValue = computed(() => props.device.kind === 'value')
const isFunction = computed(() => props.device.kind === 'function')
const booleanValue = computed(() => typeof props.device.state.value === 'boolean')
const maxValue = computed(() => {
  if (isValue.value) {
    return 250
  }
  return props.device.capabilities.maxValue ?? 1023
})
const minValue = computed(() => props.device.capabilities.minValue ?? 0)

watch(
  () => props.device,
  (device) => {
    held.value = device.active
    parameters.value = String(device.state.lastParameters ?? '')
    returnCode.value = Number(device.state.returnCode ?? 0)
    if (device.kind === 'servo') {
      numericValue.value = Number(device.state.angle ?? 0)
    } else if (device.kind === 'pwm') {
      numericValue.value = Number(device.state.byteValue ?? 0)
    } else {
      numericValue.value = Number(device.state.value ?? device.state.analogValue ?? 0)
    }
  },
  { immediate: true, deep: true },
)

async function setActive(active: boolean) {
  await sim.patchDevice(props.boardId, props.device.id, { active })
}

async function press() {
  held.value = true
  await sim.pressDevice(props.boardId, props.device.id)
}

async function release() {
  held.value = false
  await sim.releaseDevice(props.boardId, props.device.id)
}

async function pulse() {
  await sim.pressDevice(props.boardId, props.device.id, 200)
}

async function applyNumeric() {
  await sim.patchDevice(props.boardId, props.device.id, { value: numericValue.value })
}

async function applyBoolean(value: boolean) {
  await sim.setValue(props.boardId, props.device.valueName ?? props.device.name, value)
}

async function invoke() {
  const result = await sim.callFunction(
    props.boardId,
    props.device.functionName ?? props.device.name,
    parameters.value,
    returnCode.value,
  )
  functionResult.value = result.result
}
</script>

<template>
  <UCard>
    <div class="flex items-start justify-between gap-3">
      <div class="min-w-0">
        <div class="flex items-center gap-2">
          <h3 class="truncate font-medium">{{ device.label || device.name }}</h3>
          <UBadge color="neutral" variant="subtle">{{ device.kind }}</UBadge>
          <UBadge v-if="device.inverted" color="warning" variant="subtle">NC</UBadge>
        </div>
        <p class="truncate text-xs text-muted">
          {{ device.id }} <span v-if="device.pins.length">· pin {{ device.pins.join(', ') }}</span>
        </p>
      </div>
      <UBadge :color="device.active ? 'success' : 'neutral'" variant="subtle">
        {{ device.active ? 'active' : 'idle' }}
      </UBadge>
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
      <USwitch
        :model-value="device.active"
        label="Latch"
        @update:model-value="setActive"
      />
      <UButton color="neutral" variant="soft" size="sm" icon="i-lucide-zap" @click="pulse">
        Pulse 200ms
      </UButton>
      <span class="text-xs text-muted">
        pin {{ device.state.pinValue ?? 0 }} / default {{ device.defaultState }}
      </span>
    </div>

    <div v-else-if="isOutput" class="mt-4 flex items-center gap-3">
      <span
        class="size-4 rounded-full transition"
        :class="device.active ? 'bg-success shadow-[0_0_10px] shadow-success' : 'bg-elevated'"
      />
      <USwitch :model-value="device.active" label="Manual override" @update:model-value="setActive" />
      <span class="text-xs text-muted">pin {{ device.state.pinValue ?? 0 }}</span>
    </div>

    <div v-else-if="isAnalog" class="mt-4 space-y-2">
      <div class="flex items-center gap-3">
        <USlider
          v-model="numericValue"
          :min="minValue"
          :max="maxValue"
          class="flex-1"
          @update:model-value="applyNumeric"
        />
        <UInput v-model.number="numericValue" type="number" class="w-24" />
        <UButton size="sm" @click="applyNumeric">Apply</UButton>
      </div>
      <UProgress :model-value="numericValue" :max="maxValue" />
      <p class="text-xs text-muted">
        {{ minValue }}–{{ maxValue }} {{ device.capabilities.unit || '' }}
      </p>
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
  </UCard>
</template>
