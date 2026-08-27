<script setup lang="ts">
import { materialSymbolPaths, type MaterialSymbolName } from '../materialSymbols'

withDefaults(defineProps<{
  name: MaterialSymbolName
  size?: number
  label?: string
}>(), {
  size: 24,
  label: '',
})
</script>

<template>
  <svg
    class="material-icon"
    xmlns="http://www.w3.org/2000/svg"
    :width="size"
    :height="size"
    viewBox="0 -960 960 960"
    :aria-hidden="label ? undefined : 'true'"
    :aria-label="label || undefined"
    focusable="false"
  >
    <path :d="materialSymbolPaths[name]" />
  </svg>
</template>

<style scoped>
.material-icon {
  display: block;
  flex: 0 0 auto;
  fill: currentColor;
}
</style>
