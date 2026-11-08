<script setup lang="ts">
import { computed } from 'vue'
import BoardCard from '~/components/BoardCard.vue'
import EventFeed from '~/components/EventFeed.vue'
import { useSimulator } from '~/composables/useSimulator'

const sim = useSimulator()

const online = computed(() => sim.boards.value.filter((board) => board.online).length)
const faults = computed(() => sim.boards.value.filter((board) => board.fault.active).length)

const kpis = computed(() => [
  { label: 'Boards', value: sim.boards.value.length, icon: 'i-lucide-layout-grid' },
  { label: 'Online', value: online.value, icon: 'i-lucide-wifi' },
  { label: 'Faults', value: faults.value, icon: 'i-lucide-triangle-alert' },
  { label: 'Sequence', value: sim.lastSeq.value, icon: 'i-lucide-list-ordered' },
  { label: 'Dropped', value: sim.meta.value?.droppedEvents ?? 0, icon: 'i-lucide-inbox' },
])
</script>

<template>
  <div class="space-y-6">
    <UAlert
      v-if="sim.error.value"
      color="error"
      icon="i-lucide-circle-alert"
      title="Simulator request failed"
      :description="sim.error.value"
    />

    <section class="grid gap-3 sm:grid-cols-2 lg:grid-cols-5">
      <UCard v-for="kpi in kpis" :key="kpi.label">
        <div class="flex items-center gap-3">
          <UIcon :name="kpi.icon" class="size-5 text-primary" />
          <div>
            <div class="text-2xl font-semibold">{{ kpi.value }}</div>
            <div class="text-xs text-muted">{{ kpi.label }}</div>
          </div>
        </div>
      </UCard>
    </section>

    <section class="space-y-3">
      <div class="flex items-center gap-2">
        <h1 class="text-xl font-semibold">Boards</h1>
        <UBadge color="neutral" variant="subtle">{{ sim.boards.value.length }}</UBadge>
      </div>
      <div v-if="sim.boards.value.length" class="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
        <BoardCard v-for="board in sim.boards.value" :key="board.id" :board="board" />
      </div>
      <UAlert
        v-else
        color="neutral"
        icon="i-lucide-plug-zap"
        title="No boards registered"
        description="Create a simulated adaptor or run the demo binary to see boards here."
      />
    </section>

    <EventFeed :events="sim.events.value" />
  </div>
</template>
