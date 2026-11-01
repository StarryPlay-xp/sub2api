<template>
  <div
    v-if="warnings.length"
    :data-testid="testId"
    class="space-y-1.5 rounded-2xl bg-amber-50/80 px-4 py-3 text-xs leading-relaxed text-amber-800 ring-1 ring-amber-200/60 dark:bg-amber-950/25 dark:text-amber-300 dark:ring-amber-900/40"
  >
    <p v-for="warning in warnings" :key="warning.key" class="flex items-start gap-2">
      <Icon name="exclamationTriangle" size="xs" class="mt-px h-3.5 w-3.5 shrink-0" />
      <span>{{ warning.text }}</span>
    </p>
  </div>
</template>

<script setup lang="ts">
import Icon from '@/components/icons/Icon.vue'
import type { ReportWarning } from './reportWarnings'

withDefaults(defineProps<{ warnings: ReportWarning[]; testId?: string }>(), { testId: 'host-warnings' })
</script>
