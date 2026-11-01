<template>
  <!-- 统一控制栏：范围作用于本页全部内容 -->
  <div class="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
    <div
      class="inline-flex self-start rounded-xl border border-gray-200 bg-white p-1 shadow-sm dark:border-dark-700 dark:bg-dark-800"
      role="group"
      :aria-label="t('bandwidth.rangeLabel')"
    >
      <button
        v-for="option in options"
        :key="option.value"
        type="button"
        class="rounded-lg px-3.5 py-1.5 text-sm font-medium transition-colors duration-150 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/40"
        :class="
          modelValue === option.value
            ? 'bg-primary-600 text-white shadow-sm dark:bg-primary-500'
            : 'text-gray-600 hover:bg-gray-50 hover:text-gray-900 dark:text-dark-300 dark:hover:bg-dark-700 dark:hover:text-white'
        "
        :aria-pressed="modelValue === option.value"
        @click="emit('update:modelValue', option.value)"
      >
        {{ option.label }}
      </button>
    </div>

    <div class="flex flex-wrap items-center gap-x-4 gap-y-2 text-xs text-gray-500 dark:text-dark-400">
      <span v-if="window" class="inline-flex flex-wrap items-center gap-x-2" data-testid="range-text">
        <Icon name="clock" size="xs" class="h-3.5 w-3.5" />
        <span class="font-medium text-gray-700 dark:text-dark-200">{{ rangeText }}</span>
        <span :title="t('bandwidth.timeZone')">{{ timezoneText }}</span>
        <span>{{ window.interval === 'minute' ? t('bandwidth.perMinute') : t('bandwidth.perHour') }}</span>
        <span v-if="window.end_clamped" class="text-amber-600 dark:text-amber-400">{{ t('bandwidth.endClamped') }}</span>
      </span>
      <button type="button" class="btn btn-secondary btn-sm" :disabled="refreshDisabled" @click="emit('refresh')">
        <Icon name="refresh" size="xs" class="h-3.5 w-3.5" :class="{ 'motion-safe:animate-spin': loading }" />
        {{ t('bandwidth.refresh') }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import type { TrafficInterval } from '@/api/admin/wireguard'
import { formatDateTimeLabel, timeZoneName, utcOffsetLabel } from '@/utils/wireguardTraffic'
import type { PresetRange } from './usePresetRange'

export interface RangeWindow {
  start: string
  end: string
  interval: TrafficInterval
  end_clamped: boolean
}

const props = defineProps<{
  modelValue: PresetRange
  options: { value: PresetRange; label: string }[]
  window: RangeWindow | null
  loading?: boolean
  refreshDisabled?: boolean
}>()

const emit = defineEmits<{
  'update:modelValue': [value: PresetRange]
  refresh: []
}>()

const { t } = useI18n()

const rangeText = computed(() =>
  props.window ? `${formatDateTimeLabel(props.window.start)} – ${formatDateTimeLabel(props.window.end)}` : ''
)

const timezoneText = computed(() => {
  const name = timeZoneName()
  if (!props.window) return name
  const startOffset = utcOffsetLabel(new Date(props.window.start))
  const endOffset = utcOffsetLabel(new Date(props.window.end))
  return startOffset === endOffset ? `${name} (${startOffset})` : `${name} (${startOffset} → ${endOffset})`
})
</script>
