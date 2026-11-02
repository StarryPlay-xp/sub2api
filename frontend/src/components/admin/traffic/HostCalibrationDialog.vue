<template>
  <BaseDialog :show="true" :title="isFill ? t('traffic.host.calibrate.fillTitle') : t('traffic.host.calibrate.title')" width="normal" @close="emit('close')">
    <form id="host-calibration-form" class="space-y-5" novalidate data-testid="host-calibration-form" @submit.prevent="submit">
      <p class="text-sm text-gray-600 dark:text-dark-300">{{ t('traffic.host.calibrate.intro') }}</p>
      <p
        class="rounded-xl px-3.5 py-2.5 text-xs leading-relaxed"
        :class="isFill ? 'bg-amber-50 text-amber-800 dark:bg-amber-950/20 dark:text-amber-300' : 'bg-gray-50 text-gray-600 dark:bg-dark-900/40 dark:text-dark-300'"
        data-testid="host-calibration-explain"
      >
        {{ explainText }}
      </p>

      <p
        v-if="submitError"
        role="alert"
        class="rounded-2xl bg-red-50 px-4 py-3 text-sm text-red-700 ring-1 ring-red-200/70 dark:bg-red-950/30 dark:text-red-300 dark:ring-red-900/40"
        data-testid="host-calibration-error"
      >
        {{ submitError }}
      </p>

      <div>
        <label for="host-provider-used" class="input-label">{{ t('traffic.host.calibrate.value') }}</label>
        <div class="flex gap-2">
          <input
            id="host-provider-used"
            v-model="value"
            class="input min-w-0 flex-1 tabular-nums"
            :class="{ 'input-error': valueError }"
            inputmode="decimal"
            autocomplete="off"
            placeholder="102.92"
            data-testid="host-calibration-value"
          />
          <select v-model="unit" class="input w-24 shrink-0" :aria-label="t('traffic.host.calibrate.value')">
            <option v-for="item in UNITS" :key="item" :value="item">{{ unitLabel(item) }}</option>
          </select>
        </div>
        <p v-if="valueError" class="input-error-text">{{ valueError }}</p>
        <p v-else-if="measuredText" class="input-hint">{{ measuredText }}</p>
      </div>

      <label v-if="!isFill" class="flex items-start gap-2.5 rounded-xl border border-gray-200 px-3.5 py-3 dark:border-dark-700">
        <input v-model="override" type="checkbox" class="mt-0.5 h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" data-testid="host-calibration-override" />
        <span>
          <span class="block text-sm font-medium text-gray-900 dark:text-white">{{ t('traffic.host.calibrate.override') }}</span>
          <span class="mt-0.5 block text-xs text-gray-500 dark:text-dark-400">{{ t('traffic.host.calibrate.overrideHint') }}</span>
        </span>
      </label>
    </form>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button type="button" class="btn btn-secondary" :disabled="saving" @click="emit('close')">{{ t('common.cancel') }}</button>
        <button type="submit" form="host-calibration-form" class="btn btn-primary" :disabled="saving" data-testid="host-calibration-submit">
          {{ saving ? t('traffic.host.form.saving') : t('traffic.host.calibrate.submit') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import {
  calibrateHostTraffic,
  type HostCalibrationKind,
  type HostTrafficStatus,
  type HostUsedUnit
} from '@/api/admin/hostTraffic'
import { boundPrefix, formatLocalShort, formatQuotaBytes, isAmountText } from './hostFormat'
import { hostErrorMessage } from './useHostTraffic'

const props = defineProps<{ status: HostTrafficStatus }>()
const emit = defineEmits<{
  close: []
  saved: [status: HostTrafficStatus, kind: HostCalibrationKind]
}>()

const { t } = useI18n()
const UNITS: HostUsedUnit[] = ['MB', 'GB', 'TB']

const plan = computed(() => props.status.plan)
const period = computed(() => props.status.period)
const base = computed(() => plan.value?.unit_base ?? 1000)

// 期初缺失且尚未补齐时作为补齐；其余情况默认只记录对账。
const isFill = computed(() => period.value?.coverage === 'partial' && period.value.adjustment_bytes === null)

const explainText = computed(() =>
  isFill.value
    ? t('traffic.host.calibrate.fillExplain', { time: formatLocalShort(period.value?.observed_from) })
    : t('traffic.host.calibrate.reconcileExplain')
)

const measuredText = computed(() => {
  if (!period.value) return ''
  const prefix = boundPrefix(period.value.bound, t)
  return t('traffic.host.calibrate.current', { value: `${prefix ? `${prefix} ` : ''}${formatQuotaBytes(period.value.billed_bytes, base.value)}` })
})

const value = ref('')
const unit = ref<HostUsedUnit>('GB')
const override = ref(false)
const valueError = ref('')
const submitError = ref('')
const saving = ref(false)

function unitLabel(item: HostUsedUnit) {
  return base.value === 1000 ? item : `${item[0]}iB`
}

async function submit() {
  submitError.value = ''
  valueError.value = isAmountText(value.value, true) ? '' : t('traffic.host.form.errors.currentUsed')
  if (valueError.value || saving.value || !plan.value) return
  saving.value = true
  try {
    const result = await calibrateHostTraffic({
      value: value.value.trim(),
      unit: unit.value,
      mode: override.value && !isFill.value ? 'override' : 'auto',
      plan_version: plan.value.version
    })
    emit('saved', result.status, result.kind)
  } catch (e) {
    submitError.value = hostErrorMessage(e, t)
  } finally {
    saving.value = false
  }
}
</script>
