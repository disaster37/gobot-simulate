<script setup lang="ts">
import { ref } from 'vue'
import { useSimulator } from '../composables/useSimulator'
import type { BoardState } from '../types/simulator'

const props = defineProps<{ board: BoardState }>()
const sim = useSimulator()
const message = ref('simulated hardware fault')

async function inject() {
  await sim.setFault(props.board.id, message.value)
}

async function clear() {
  await sim.clearFault(props.board.id)
}
</script>

<template>
  <UAlert
    :color="board.fault.active ? 'error' : 'neutral'"
    :icon="board.fault.active ? 'i-lucide-triangle-alert' : 'i-lucide-shield-check'"
    :title="board.fault.active ? 'Fault injected' : 'No active fault'"
    :description="board.fault.active ? board.fault.message : 'All reads and writes behave normally.'"
  >
    <template #actions>
      <div v-if="board.fault.active" class="flex items-center gap-2">
        <UButton color="neutral" variant="soft" size="sm" @click="clear">Clear fault</UButton>
      </div>
      <div v-else class="flex items-center gap-2">
        <UInput v-model="message" size="sm" placeholder="Fault message" />
        <UButton color="error" variant="soft" size="sm" @click="inject">Inject fault</UButton>
      </div>
    </template>
  </UAlert>
</template>
