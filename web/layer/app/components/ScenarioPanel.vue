<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useSimulator } from '../composables/useSimulator'
import type { ScenarioRule } from '../types/simulator'

const props = defineProps<{ boardId: string }>()
const sim = useSimulator()

const scenarios = ref<ScenarioRule[]>([])
const error = ref('')
const loading = ref(false)

async function refresh() {
  loading.value = true
  try {
    scenarios.value = await sim.fetchScenarios()
    error.value = ''
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    loading.value = false
  }
}

async function toggle(rule: ScenarioRule, enabled: boolean) {
  try {
    const updated = await sim.setScenarioEnabled(rule.id, enabled)
    scenarios.value = scenarios.value.map((item) => (item.id === updated.id ? updated : item))
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  }
}

async function trigger(rule: ScenarioRule) {
  try {
    await sim.triggerScenario(rule.id)
    error.value = ''
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  }
}

onMounted(refresh)
</script>

<template>
  <section class="space-y-3">
    <div class="flex items-center gap-2">
      <UIcon name="i-lucide-workflow" class="size-4 text-primary" />
      <h2 class="text-base font-semibold">Scenarios</h2>
      <UBadge color="neutral" variant="subtle">{{ scenarios.length }}</UBadge>
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

    <div v-if="scenarios.length" class="grid gap-3 xl:grid-cols-2">
      <UCard v-for="rule in scenarios" :key="rule.id">
        <template #header>
          <div class="flex flex-wrap items-center gap-2">
            <span class="font-medium">{{ rule.name || rule.id }}</span>
            <UBadge color="neutral" variant="subtle" size="sm">
              {{ rule.trigger.type }}
            </UBadge>
            <UBadge v-if="rule.board" color="info" variant="subtle" size="sm">
              {{ rule.board }}
            </UBadge>
            <USwitch
              class="ml-auto"
              :model-value="rule.enabled !== false"
              @update:model-value="toggle(rule, $event)"
            />
          </div>
        </template>

        <p class="text-xs text-muted">
          {{ rule.actions.length }} action<span v-if="rule.actions.length !== 1">s</span>
          <span v-if="rule.conditions?.length">
            · {{ rule.conditions.length }} condition<span v-if="rule.conditions.length !== 1">s</span>
          </span>
        </p>
        <ul class="mt-2 list-inside list-disc font-mono text-xs text-muted">
          <li v-for="(action, index) in rule.actions" :key="index">
            {{ action.type }}
            <span v-if="action.target">→ {{ action.target }}</span>
            <span v-if="action.pin">pin {{ action.pin }}</span>
            <span v-if="action.name">· {{ action.name }}</span>
            <span v-if="action.command">· {{ action.command }}</span>
            <span v-if="action.delayMs">· +{{ action.delayMs }}ms</span>
          </li>
        </ul>

        <template #footer>
          <UButton
            size="xs"
            icon="i-lucide-play"
            :disabled="rule.enabled === false"
            @click="trigger(rule)"
          >
            Trigger
          </UButton>
        </template>
      </UCard>
    </div>

    <UAlert
      v-else
      color="neutral"
      icon="i-lucide-workflow"
      title="No scenarios"
      description="Add scenarios to the topology file to drive inputs without manual clicks."
    />
  </section>
</template>
