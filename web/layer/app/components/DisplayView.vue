<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import type { DisplaySnapshot } from '../types/simulator'

const props = withDefaults(
  defineProps<{ display?: DisplaySnapshot; zoom?: number }>(),
  { zoom: 2 },
)

const canvas = ref<HTMLCanvasElement | null>(null)

function draw() {
  const element = canvas.value
  const display = props.display
  if (!element || !display?.pixels?.length) {
    return
  }
  const scale = props.zoom || 2
  element.width = display.width
  element.height = display.height
  element.style.width = `${display.width * scale}px`
  element.style.height = `${display.height * scale}px`
  const context = element.getContext('2d')
  if (!context) {
    return
  }
  context.fillStyle = '#000'
  context.fillRect(0, 0, display.width, display.height)
  context.fillStyle = display.enabled === false ? '#444' : '#7dd3fc'
  const pages = Math.ceil(display.height / 8)
  for (let page = 0; page < pages; page++) {
    for (let column = 0; column < display.width; column++) {
      const value = display.pixels[page * display.width + column] ?? 0
      for (let bit = 0; bit < 8; bit++) {
        if ((value & (1 << bit)) !== 0) {
          context.fillRect(column, page * 8 + bit, 1, 1)
        }
      }
    }
  }
}

onMounted(draw)
watch(() => [props.display, props.zoom], draw, { deep: true })
</script>

<template>
  <div class="flex flex-col gap-2">
    <div class="overflow-auto rounded-lg border border-default bg-black p-2">
      <canvas ref="canvas" class="block" />
    </div>
    <pre
      v-if="display?.lines?.length"
      class="rounded-lg bg-elevated p-2 font-mono text-xs leading-5"
    >{{ display.lines.join('\n') }}</pre>
    <p v-if="display" class="text-xs text-muted">
      {{ display.model }} · {{ display.width }}×{{ display.height }} ·
      {{ display.enabled === false ? 'display off' : 'enabled' }}
    </p>
  </div>
</template>
