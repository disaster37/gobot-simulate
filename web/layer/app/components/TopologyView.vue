<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useSimulator } from '../composables/useSimulator'
import type { TopologySpec } from '../types/simulator'

const sim = useSimulator()
const spec = ref<TopologySpec | null>(null)
const source = ref('')
const error = ref('')
const loading = ref(false)

async function refresh() {
  loading.value = true
  try {
    spec.value = await sim.fetchTopology()
    source.value = await sim.fetchTopologySource().catch(() => '')
    error.value = ''
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    loading.value = false
  }
}

onMounted(refresh)
</script>

<template>
  <section class="space-y-3">
    <div class="flex items-center gap-2">
      <UIcon name="i-lucide-network" class="size-4 text-primary" />
      <h2 class="text-base font-semibold">Effective topology</h2>
      <UBadge v-if="spec" color="neutral" variant="subtle">schema {{ spec.schema }}</UBadge>
      <UButton
        class="ml-auto"
        size="xs"
        color="neutral"
        variant="ghost"
        icon="i-lucide-refresh-cw"
        :loading="loading"
        @click="refresh"
      >
        Refresh
      </UButton>
    </div>

    <UAlert v-if="error" color="error" icon="i-lucide-circle-alert" :title="error" />

    <UCard v-if="spec">
      <template #header>
        <div class="flex items-center gap-2">
          <h3 class="font-medium">Boards</h3>
          <UBadge color="neutral" variant="subtle">{{ spec.boards.length }}</UBadge>
        </div>
      </template>
      <div class="space-y-2">
        <div
          v-for="(board, index) in spec.boards"
          :key="index"
          class="rounded-lg bg-elevated p-3 text-sm"
        >
          <div class="flex flex-wrap items-center gap-2">
            <span class="font-mono">{{ board.id }}</span>
            <UBadge v-if="board.platform" color="info" variant="subtle" size="sm">
              {{ board.platform }}
            </UBadge>
            <UBadge v-if="board.profile" color="neutral" variant="subtle" size="sm">
              profile {{ board.profile }}
            </UBadge>
          </div>
        </div>
      </div>
    </UCard>

    <UCard v-if="source">
      <template #header>
        <h3 class="font-medium">Source document</h3>
      </template>
      <pre class="max-h-96 overflow-auto font-mono text-xs leading-5">{{ source }}</pre>
    </UCard>
  </section>
</template>
