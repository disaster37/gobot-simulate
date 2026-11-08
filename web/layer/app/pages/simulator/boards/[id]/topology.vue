<script setup lang="ts">
import { computed } from 'vue'
import TopologyView from '../../../../components/TopologyView.vue'
import { useSimulator } from '../../../../composables/useSimulator'

definePageMeta({ layout: 'simulator' })

const route = useRoute()
const sim = useSimulator()
const boardID = computed(() => String(route.params.id))
const board = computed(
  () => sim.boards.value.find((item) => item.id === boardID.value || item.name === boardID.value) ?? null,
)
</script>

<template>
  <div class="space-y-4">
    <div class="flex flex-wrap items-center gap-2">
      <UButton
        :to="`/simulator/boards/${boardID}`"
        color="neutral"
        variant="ghost"
        size="xs"
        icon="i-lucide-arrow-left"
      >
        Board
      </UButton>
      <h1 class="text-2xl font-semibold">Topology · {{ board?.name || boardID }}</h1>
    </div>
    <TopologyView />
  </div>
</template>
