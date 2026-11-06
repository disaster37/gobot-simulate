<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import ConnectionStatus from '~/components/ConnectionStatus.vue'
import { useSimulator } from '~/composables/useSimulator'
import { useSSE } from '~/composables/useSSE'

const sim = useSimulator()
const colorMode = useColorMode()
const tokenModal = ref(false)
const tokenInput = ref('')

const sse = useSSE({
  onEvent: sim.applyEvent,
  onSnapshot: sim.applySnapshot,
  getToken: () => sim.authToken.value,
})

const onlineBoards = computed(() => sim.boards.value.filter((board) => board.online).length)
const activeFaults = computed(() => sim.boards.value.filter((board) => board.fault.active).length)

onMounted(async () => {
  try {
    await sim.fetchSnapshot()
  } catch {
    // The connection badge communicates the failure; the UI remains usable.
  }
  sse.start()
})

onBeforeUnmount(() => sse.stop())

function toggleMode() {
  colorMode.preference = colorMode.value === 'dark' ? 'light' : 'dark'
}

function saveToken() {
  sim.authToken.value = tokenInput.value.trim()
  tokenModal.value = false
  sse.reconnect()
}
</script>

<template>
  <UApp>
    <div class="min-h-screen bg-default text-default">
      <header class="sticky top-0 z-20 border-b border-default bg-default/90 backdrop-blur">
        <div class="mx-auto flex max-w-7xl flex-wrap items-center gap-3 px-4 py-3">
          <NuxtLink to="/" class="flex items-center gap-2 text-lg font-semibold">
            <UIcon name="i-lucide-cpu" class="size-5 text-primary" />
            Gobot Simulator
          </NuxtLink>
          <UBadge color="neutral" variant="subtle">{{ sim.boards.value.length }} boards</UBadge>
          <UBadge color="success" variant="subtle">{{ onlineBoards }} online</UBadge>
          <UBadge v-if="activeFaults" color="error" variant="subtle">{{ activeFaults }} faults</UBadge>
          <div class="ml-auto flex items-center gap-2">
            <ConnectionStatus
              :status="sse.status.value"
              :seq="sim.lastSeq.value"
              :dropped="sim.meta.value?.droppedEvents ?? 0"
              :clients="sim.meta.value?.connectedClients ?? 0"
              @reconnect="sse.reconnect()"
            />
            <UButton
              color="neutral"
              variant="ghost"
              :icon="colorMode.value === 'dark' ? 'i-lucide-sun' : 'i-lucide-moon'"
              aria-label="Toggle color mode"
              @click="toggleMode"
            />
            <UButton
              color="neutral"
              variant="ghost"
              icon="i-lucide-key-round"
              aria-label="Set auth token"
              @click="tokenModal = true"
            />
          </div>
        </div>
      </header>

      <main class="mx-auto max-w-7xl px-4 py-6">
        <NuxtPage />
      </main>

      <UModal v-model:open="tokenModal" title="Bearer token">
        <template #body>
          <p class="mb-3 text-sm text-muted">
            Required only when the simulator was started with an auth token.
          </p>
          <UInput v-model="tokenInput" type="password" placeholder="Token" class="w-full" />
        </template>
        <template #footer>
          <UButton color="neutral" variant="ghost" @click="tokenModal = false">Cancel</UButton>
          <UButton @click="saveToken">Save</UButton>
        </template>
      </UModal>
    </div>
  </UApp>
</template>
