<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useSimulator } from '../composables/useSimulator'
import type { ChipEntry, DisplayChipValues } from '../types/simulator'
import DisplayView from './DisplayView.vue'

const props = defineProps<{ boardId: string }>()
const sim = useSimulator()

const chips = ref<ChipEntry[]>([])
const values = ref<Record<string, DisplayChipValues>>({})
const fieldInputs = ref<Record<string, string>>({})
const rawWrite = ref<Record<string, string>>({})
const rawReadLen = ref<Record<string, number>>({})
const errors = ref<Record<string, string>>({})
const loading = ref(false)

const buses = computed(() => {
  const seen = new Set<string>()
  return chips.value
    .map((chip) => chip.bus)
    .filter((bus) => (seen.has(bus) ? false : (seen.add(bus), true)))
})

async function refresh() {
  loading.value = true
  try {
    chips.value = await sim.fetchChips(props.boardId)
    for (const chip of chips.value) {
      await refreshChip(chip.id)
    }
  } catch (err) {
    errors.value.__all = err instanceof Error ? err.message : String(err)
  } finally {
    loading.value = false
  }
}

async function refreshChip(chipID: string) {
  try {
    values.value = { ...values.value, [chipID]: await sim.fetchChip(chipID) }
    const display = values.value[chipID]?.values ?? {}
    for (const [name, value] of Object.entries(display)) {
      const key = `${chipID}.${name}`
      if (fieldInputs.value[key] === undefined) {
        fieldInputs.value[key] = String(value)
      }
    }
  } catch (err) {
    errors.value[chipID] = err instanceof Error ? err.message : String(err)
  }
}

async function applyField(chipID: string, field: string, widget?: string) {
  const key = `${chipID}.${field}`
  const raw = fieldInputs.value[key] ?? ''
  let value: unknown = raw
  if (widget === 'switch') {
    value = raw === 'true' || raw === '1'
  } else if (widget === 'number' || widget === 'register') {
    const number = Number(raw)
    value = Number.isFinite(number) ? number : raw
  }
  try {
    values.value = { ...values.value, [chipID]: await sim.patchChip(chipID, field, value) }
    errors.value[chipID] = ''
  } catch (err) {
    errors.value[chipID] = err instanceof Error ? err.message : String(err)
  }
}

async function sendRaw(chipID: string) {
  const raw = (rawWrite.value[chipID] ?? '').trim()
  const write = raw
    .split(/[\s,]+/)
    .filter(Boolean)
    .map((part) => Number.parseInt(part, 16))
    .filter((value) => Number.isFinite(value) && value >= 0 && value <= 255)
  try {
    const response = await sim.rawChip(chipID, write, rawReadLen.value[chipID] ?? 0)
    errors.value[chipID] = `response: ${JSON.stringify(response)}`
  } catch (err) {
    errors.value[chipID] = err instanceof Error ? err.message : String(err)
  }
}

onMounted(refresh)
watch(() => props.boardId, refresh)
</script>

<template>
  <section class="space-y-3">
    <div class="flex items-center gap-2">
      <UIcon name="i-lucide-circuit-board" class="size-4 text-primary" />
      <h2 class="text-base font-semibold">Buses & chips</h2>
      <UBadge color="neutral" variant="subtle">{{ chips.length }}</UBadge>
      <UBadge v-for="bus in buses" :key="bus" color="info" variant="subtle" size="sm">
        {{ bus }}
      </UBadge>
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

    <UAlert
      v-if="errors.__all"
      color="error"
      icon="i-lucide-circle-alert"
      :title="errors.__all"
    />

    <div v-if="chips.length" class="grid gap-3 xl:grid-cols-2">
      <UCard v-for="chip in chips" :key="chip.id">
        <template #header>
          <div class="flex flex-wrap items-center gap-2">
            <span class="font-medium">{{ chip.id }}</span>
            <UBadge color="neutral" variant="subtle">{{ chip.model }}</UBadge>
            <UBadge color="info" variant="subtle" size="sm">
              {{ chip.bus }}{{ chip.address !== undefined ? ` @ 0x${chip.address.toString(16)}` : '' }}
            </UBadge>
            <UBadge
              v-for="fault in chip.faults ?? []"
              :key="fault"
              color="error"
              variant="subtle"
              size="sm"
            >
              {{ fault }}
            </UBadge>
            <UButton
              class="ml-auto"
              size="xs"
              color="neutral"
              variant="ghost"
              icon="i-lucide-refresh-cw"
              @click="refreshChip(chip.id)"
            />
          </div>
        </template>

        <div class="space-y-3">
          <DisplayView
            v-if="values[chip.id]?.display"
            :display="values[chip.id]?.display"
          />

          <div
            v-for="field in values[chip.id]?.descriptor?.fields ?? []"
            :key="field.name"
            class="flex flex-wrap items-center gap-2"
          >
            <span class="w-36 truncate text-xs text-muted">
              {{ field.name }}
              <span v-if="field.unit">({{ field.unit }})</span>
            </span>
            <UInput
              v-model="fieldInputs[`${chip.id}.${field.name}`]"
              :type="field.widget === 'number' ? 'number' : 'text'"
              size="xs"
              class="min-w-32 flex-1"
              :placeholder="String(values[chip.id]?.values?.[field.name] ?? '')"
            />
            <UButton
              size="xs"
              @click="applyField(chip.id, field.name, field.widget)"
            >
              Set
            </UButton>
          </div>

          <div class="flex flex-wrap items-center gap-2 border-t border-default pt-3">
            <UInput
              v-model="rawWrite[chip.id]"
              size="xs"
              placeholder="raw hex, e.g. 01 00"
              class="min-w-40 flex-1"
            />
            <UInput
              v-model.number="rawReadLen[chip.id]"
              type="number"
              size="xs"
              class="w-24"
              placeholder="read len"
            />
            <UButton size="xs" color="neutral" variant="soft" @click="sendRaw(chip.id)">
              Transact
            </UButton>
          </div>

          <p v-if="errors[chip.id]" class="font-mono text-xs text-muted">
            {{ errors[chip.id] }}
          </p>
        </div>
      </UCard>
    </div>

    <UAlert
      v-else
      color="neutral"
      icon="i-lucide-cable"
      title="No bus chips"
      description="Declare chips in the topology file to expose register-accurate models."
    />
  </section>
</template>
