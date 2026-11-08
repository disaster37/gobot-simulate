<script setup lang="ts">
import type { DeviceState } from '../types/simulator'
import DeviceControl from '../components/DeviceControl.vue'

defineProps<{
  title: string
  boardId: string
  devices: DeviceState[]
  icon?: string
}>()
</script>

<template>
  <section v-if="devices.length" class="space-y-3">
    <div class="flex items-center gap-2">
      <UIcon v-if="icon" :name="icon" class="size-4 text-primary" />
      <h2 class="text-base font-semibold">{{ title }}</h2>
      <UBadge color="neutral" variant="subtle">{{ devices.length }}</UBadge>
    </div>
    <div class="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
      <DeviceControl
        v-for="device in devices"
        :key="device.id"
        :board-id="boardId"
        :device="device"
      />
    </div>
  </section>
</template>
