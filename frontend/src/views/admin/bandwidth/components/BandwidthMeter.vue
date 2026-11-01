<template>
  <div class="space-y-2" data-testid="bandwidth-meter">
    <!-- 编辑上限 -->
    <form
      v-if="editing"
      class="flex flex-wrap items-center gap-2"
      @submit.prevent="save"
    >
      <label class="sr-only" :for="inputId">{{ t('bandwidth.limit.title') }}</label>
      <div class="relative">
        <input
          :id="inputId"
          ref="inputRef"
          v-model="draft"
          type="number"
          inputmode="decimal"
          min="0"
          step="any"
          class="input h-9 w-36 pr-12 text-sm"
          :placeholder="t('bandwidth.limit.placeholder')"
          :aria-invalid="Boolean(invalid)"
          @keydown.esc.prevent="cancel"
        />
        <span class="pointer-events-none absolute inset-y-0 right-3 flex items-center text-xs text-gray-400 dark:text-dark-400">
          Mbps
        </span>
      </div>
      <button type="submit" class="btn btn-primary btn-sm">{{ t('bandwidth.limit.save') }}</button>
      <button v-if="limitMbps" type="button" class="btn btn-secondary btn-sm" @click="clear">
        {{ t('bandwidth.limit.clear') }}
      </button>
      <button type="button" class="btn btn-ghost btn-sm" @click="cancel">{{ t('bandwidth.limit.cancel') }}</button>
      <p class="basis-full text-[11px]" :class="invalid ? 'text-red-600 dark:text-red-400' : 'text-gray-400 dark:text-dark-500'">
        {{ invalid || t('bandwidth.limit.hint') }}
      </p>
    </form>

    <!-- 未设置上限 -->
    <div
      v-else-if="!limitMbps"
      class="flex flex-wrap items-center justify-between gap-2 rounded-xl border border-dashed border-gray-300 px-3 py-2.5 dark:border-dark-600"
    >
      <span class="text-xs text-gray-500 dark:text-dark-400">{{ emptyText }}</span>
      <button type="button" class="btn btn-secondary btn-sm" @click="startEdit">
        {{ t('bandwidth.limit.set') }}
      </button>
    </div>

    <!-- 峰值占上限 -->
    <template v-else>
      <div class="flex flex-wrap items-center justify-between gap-x-3 gap-y-1 text-xs">
        <div class="flex flex-wrap items-center gap-x-2.5 gap-y-1">
          <span class="font-medium text-gray-700 dark:text-dark-100">
            {{ ratio === null ? t('bandwidth.limit.usageUnknown') : t('bandwidth.limit.usage', { percent: percentText(ratio) }) }}
          </span>
          <span v-if="level" class="inline-flex items-center gap-1 font-medium" :class="levelTextClass" data-testid="meter-level">
            <Icon :name="levelIcon" size="xs" class="h-3.5 w-3.5" />
            {{ levelText }}
          </span>
        </div>
        <div class="flex items-center gap-2 text-gray-500 dark:text-dark-400">
          <span>{{ t('bandwidth.limit.limitValue', { value: formatLimit(limitMbps) }) }}</span>
          <button
            type="button"
            class="rounded font-medium text-primary-600 hover:text-primary-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/40 dark:text-primary-400"
            @click="startEdit"
          >
            {{ t('bandwidth.limit.edit') }}
          </button>
        </div>
      </div>
      <div
        class="relative h-3 rounded-full bg-gray-100 dark:bg-dark-700"
        role="meter"
        :aria-valuemin="0"
        :aria-valuemax="100"
        :aria-valuenow="ratio === null ? undefined : Math.round(Math.min(ratio, 1) * 100)"
        :aria-label="t('bandwidth.limit.title')"
      >
        <div
          v-if="ratio !== null"
          class="absolute inset-y-0 left-0 rounded-full motion-safe:transition-[width] motion-safe:duration-500"
          :class="levelFillClass"
          :style="{ width: `${Math.min(ratio, 1) * 100}%` }"
        ></div>
        <!-- 当前速率位置：细竖线，不与峰值填充抢颜色 -->
        <div
          v-if="currentRatio !== null"
          class="absolute -top-1 -bottom-1 w-0.5 -translate-x-1/2 rounded-full bg-gray-900 ring-2 ring-white motion-safe:transition-[left] motion-safe:duration-700 dark:bg-white dark:ring-dark-800"
          :style="{ left: `${Math.min(currentRatio, 1) * 100}%` }"
          :title="t('bandwidth.limit.current', { value: formatLimit(currentMbps ?? 0) })"
          data-testid="meter-current"
        ></div>
      </div>
      <div class="flex justify-between text-[11px] tabular-nums text-gray-400 dark:text-dark-500">
        <span>0</span>
        <span v-if="currentRatio !== null" class="inline-flex items-center gap-1.5">
          <span class="h-2.5 w-0.5 rounded-full bg-gray-900 dark:bg-white"></span>
          {{ t('bandwidth.limit.current', { value: formatLimit(currentMbps ?? 0) }) }}
        </span>
        <span>{{ formatLimit(limitMbps) }}</span>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, useId } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { headroomLevel } from '@/utils/bandwidthSeries'
import { normalizeLimit } from '@/utils/bandwidthLimits'

const props = defineProps<{
  peakMbps: number | null
  currentMbps?: number | null
  limitMbps: number | null
  emptyText: string
}>()

const emit = defineEmits<{ 'update:limit': [value: number | null] }>()

const { t } = useI18n()
const inputId = useId()
const inputRef = ref<HTMLInputElement | null>(null)
const editing = ref(false)
const draft = ref('')
const invalid = ref('')

const ratio = computed(() =>
  props.limitMbps && props.peakMbps !== null ? props.peakMbps / props.limitMbps : null
)

const currentRatio = computed(() =>
  props.limitMbps && typeof props.currentMbps === 'number' ? props.currentMbps / props.limitMbps : null
)

const level = computed(() => (ratio.value === null ? null : headroomLevel(ratio.value)))

const levelText = computed(() => {
  if (ratio.value !== null && ratio.value > 1) return t('bandwidth.limit.levelOver')
  if (level.value === 'critical') return t('bandwidth.limit.levelCritical')
  if (level.value === 'warning') return t('bandwidth.limit.levelWarning')
  return t('bandwidth.limit.levelNormal')
})

const levelIcon = computed(() =>
  level.value === 'critical' ? 'exclamationCircle' : level.value === 'warning' ? 'exclamationTriangle' : 'checkCircle'
)

const levelTextClass = computed(() =>
  level.value === 'critical'
    ? 'text-red-600 dark:text-red-400'
    : level.value === 'warning'
      ? 'text-amber-600 dark:text-amber-400'
      : 'text-emerald-600 dark:text-emerald-400'
)

const levelFillClass = computed(() =>
  level.value === 'critical' ? 'bg-red-500' : level.value === 'warning' ? 'bg-amber-500' : 'bg-primary-500'
)

function percentText(value: number) {
  const percent = value * 100
  return `${percent < 10 ? parseFloat(percent.toFixed(1)) : Math.round(percent)}%`
}

function formatLimit(value: number) {
  const digits = value < 10 ? 2 : value < 100 ? 1 : 0
  return `${parseFloat(value.toFixed(digits))} Mbps`
}

async function startEdit() {
  draft.value = props.limitMbps ? String(props.limitMbps) : ''
  invalid.value = ''
  editing.value = true
  await nextTick()
  inputRef.value?.focus()
}

function save() {
  const value = normalizeLimit(draft.value)
  if (value === null) {
    invalid.value = t('bandwidth.limit.invalid')
    return
  }
  emit('update:limit', value)
  editing.value = false
}

function clear() {
  emit('update:limit', null)
  editing.value = false
}

function cancel() {
  editing.value = false
}
</script>
