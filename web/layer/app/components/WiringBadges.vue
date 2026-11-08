<script setup lang="ts">
import { computed } from 'vue'
import type { DeviceState } from '../types/simulator'

const props = defineProps<{ device: DeviceState }>()

const wiring = computed(() => props.device.wiring ?? {})
const badges = computed(() => {
  const out: Array<{ label: string; color: string }> = []
  if (wiring.value.drive === 'active-low') {
    out.push({ label: 'active-low', color: 'warning' })
  }
  if (wiring.value.contact) {
    out.push({ label: wiring.value.contact === 'nc' ? 'NC' : 'NO', color: 'neutral' })
  }
  if (wiring.value.pull && wiring.value.pull !== 'none') {
    out.push({ label: `pull-${wiring.value.pull}`, color: 'info' })
  }
  if (wiring.value.mechanism && wiring.value.mechanism !== 'none') {
    out.push({ label: wiring.value.mechanism, color: 'neutral' })
  }
  return out
})
</script>

<template>
  <div class="flex flex-wrap items-center gap-1">
    <UBadge
      v-for="badge in badges"
      :key="badge.label"
      :color="badge.color"
      variant="subtle"
      size="sm"
    >
      {{ badge.label }}
    </UBadge>
    <UBadge
      v-if="wiring.loadLabel"
      color="neutral"
      variant="outline"
      size="sm"
    >
      load: {{ wiring.loadLabel }}
    </UBadge>
    <UBadge
      v-if="device.energised"
      color="warning"
      variant="subtle"
      size="sm"
      title="Coil / drive energised"
    >
      energised
    </UBadge>
    <UBadge
      v-if="device.loadPowered"
      color="success"
      variant="subtle"
      size="sm"
      title="Contact passes power to the load"
    >
      load powered
    </UBadge>
    <UBadge
      v-for="fault in device.faults ?? []"
      :key="fault"
      color="error"
      variant="subtle"
      size="sm"
    >
      {{ fault }}
    </UBadge>
  </div>
</template>
