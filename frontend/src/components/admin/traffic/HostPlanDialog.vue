<template>
  <BaseDialog :show="true" :title="isEdit ? t('traffic.host.form.editTitle') : t('traffic.host.form.createTitle')" width="wide" @close="emit('close')">
    <form id="host-plan-form" class="space-y-6" novalidate data-testid="host-plan-form" @submit.prevent="submit">
      <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('traffic.host.form.intro') }}</p>

      <p
        v-if="submitError"
        role="alert"
        class="rounded-2xl bg-red-50 px-4 py-3 text-sm text-red-700 ring-1 ring-red-200/70 dark:bg-red-950/30 dark:text-red-300 dark:ring-red-900/40"
        data-testid="host-plan-error"
      >
        {{ submitError }}
      </p>

      <div class="grid gap-5 sm:grid-cols-2">
        <!-- 套餐额度 -->
        <div>
          <label for="host-quota" class="input-label">{{ t('traffic.host.form.quota') }}</label>
          <div class="flex gap-2">
            <input
              id="host-quota"
              v-model="quotaValue"
              class="input min-w-0 flex-1 tabular-nums"
              :class="{ 'input-error': errors.quota }"
              inputmode="decimal"
              autocomplete="off"
              placeholder="1.5"
              data-testid="host-plan-quota"
            />
            <select v-model="quotaUnit" class="input w-24 shrink-0" :aria-label="t('traffic.host.form.quota')" data-testid="host-plan-quota-unit">
              <option value="GB">{{ unitLabel('GB') }}</option>
              <option value="TB">{{ unitLabel('TB') }}</option>
            </select>
          </div>
          <p v-if="errors.quota" class="input-error-text">{{ errors.quota }}</p>
        </div>

        <!-- 下次重置日期 -->
        <div>
          <label for="host-next-reset" class="input-label">{{ t('traffic.host.form.nextReset') }}</label>
          <input
            id="host-next-reset"
            v-model="nextReset"
            type="date"
            class="input"
            :class="{ 'input-error': errors.date }"
            data-testid="host-plan-next-reset"
          />
          <p v-if="errors.date" class="input-error-text">{{ errors.date }}</p>
          <p v-else-if="ruleText" class="input-hint" data-testid="host-plan-rule">{{ t('traffic.host.form.derivedRule', { rule: ruleText }) }}</p>
          <p v-else class="input-hint">{{ t('traffic.host.form.nextResetHint') }}</p>
          <p v-if="maybeClamped" class="mt-1 text-xs text-amber-600 dark:text-amber-400" data-testid="host-plan-clamped">
            {{ t('traffic.host.form.clampedHint') }}
          </p>
        </div>
      </div>

      <!-- 计费方式 -->
      <fieldset>
        <legend class="input-label">{{ t('traffic.host.form.billing') }}</legend>
        <div class="grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
          <label
            v-for="option in BILLING_MODES"
            :key="option"
            class="flex cursor-pointer flex-col rounded-xl border px-3 py-2.5 transition-colors focus-within:ring-2 focus-within:ring-primary-500/40"
            :class="
              billingMode === option
                ? 'border-primary-500 bg-primary-50/60 dark:border-primary-500/70 dark:bg-primary-900/20'
                : 'border-gray-200 hover:border-gray-300 dark:border-dark-600 dark:hover:border-dark-500'
            "
            :data-testid="`host-plan-billing-${option}`"
          >
            <input v-model="billingMode" type="radio" name="host-billing" :value="option" class="sr-only" />
            <span class="text-sm font-medium text-gray-900 dark:text-white">{{ t(`traffic.host.billing.${option}`) }}</span>
            <span class="mt-0.5 text-xs text-gray-500 dark:text-dark-400">{{ t(`traffic.host.billingHint.${option}`) }}</span>
          </label>
        </div>
      </fieldset>

      <div class="grid gap-5 sm:grid-cols-2">
        <!-- 时区 -->
        <div>
          <label for="host-timezone" class="input-label">{{ t('traffic.host.form.timezone') }}</label>
          <select id="host-timezone" v-model="timezone" class="input" :class="{ 'input-error': errors.timezone }" data-testid="host-plan-timezone">
            <optgroup :label="t('traffic.host.form.timezoneCommon')">
              <option v-for="zone in commonZones" :key="`common-${zone}`" :value="zone">{{ zone }}</option>
            </optgroup>
            <optgroup v-if="otherZones.length" :label="t('traffic.host.form.timezoneAll')">
              <option v-for="zone in otherZones" :key="zone" :value="zone">{{ zone }}</option>
            </optgroup>
          </select>
          <p v-if="errors.timezone" class="input-error-text">{{ errors.timezone }}</p>
          <p v-else class="input-hint">{{ t('traffic.host.form.timezoneHint') }}</p>
        </div>

        <!-- 首次设置时可选补齐本期已用 -->
        <div v-if="!isEdit">
          <label for="host-current-used" class="input-label">{{ t('traffic.host.form.currentUsed') }}</label>
          <div class="flex gap-2">
            <input
              id="host-current-used"
              v-model="currentUsedValue"
              class="input min-w-0 flex-1 tabular-nums"
              :class="{ 'input-error': errors.currentUsed }"
              inputmode="decimal"
              autocomplete="off"
              placeholder="102.92"
              data-testid="host-plan-current-used"
            />
            <select v-model="currentUsedUnit" class="input w-24 shrink-0" :aria-label="t('traffic.host.form.currentUsed')">
              <option value="MB">{{ unitLabel('MB') }}</option>
              <option value="GB">{{ unitLabel('GB') }}</option>
              <option value="TB">{{ unitLabel('TB') }}</option>
            </select>
          </div>
          <p v-if="errors.currentUsed" class="input-error-text">{{ errors.currentUsed }}</p>
          <p v-else class="input-hint">{{ t('traffic.host.form.currentUsedHint') }}</p>
        </div>
      </div>

      <!-- 用量提醒 -->
      <section class="rounded-2xl border border-gray-200 p-4 dark:border-dark-700" data-testid="host-plan-alerts">
        <div class="flex items-start justify-between gap-4">
          <div>
            <h4 class="text-sm font-medium text-gray-900 dark:text-white">{{ t('traffic.host.form.alerts') }}</h4>
            <p class="mt-0.5 text-xs text-gray-500 dark:text-dark-400">{{ t('traffic.host.form.alertsHint') }}</p>
          </div>
          <Toggle v-model="alertsEnabled" :aria-label="t('traffic.host.form.alerts')" data-testid="host-plan-alerts-toggle" />
        </div>
        <div v-if="alertsEnabled" class="mt-4 flex flex-wrap items-end gap-x-5 gap-y-3">
          <div>
            <label for="host-warning" class="input-label">{{ t('traffic.host.form.warningPercent') }}</label>
            <div class="flex items-center gap-1.5">
              <input id="host-warning" v-model.number="warningPercent" type="number" min="1" max="100" class="input w-20 tabular-nums" :class="{ 'input-error': errors.percent }" />
              <span class="text-sm text-gray-500">%</span>
            </div>
          </div>
          <div>
            <label for="host-critical" class="input-label">{{ t('traffic.host.form.criticalPercent') }}</label>
            <div class="flex items-center gap-1.5">
              <input id="host-critical" v-model.number="criticalPercent" type="number" min="1" max="100" class="input w-20 tabular-nums" :class="{ 'input-error': errors.percent }" />
              <span class="text-sm text-gray-500">%</span>
            </div>
          </div>
          <label class="flex items-center gap-2 pb-2 text-sm text-gray-700 dark:text-dark-200">
            <input v-model="projectionAlert" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
            {{ t('traffic.host.form.projection') }}
          </label>
        </div>
        <p v-if="alertsEnabled && errors.percent" class="input-error-text">{{ errors.percent }}</p>
        <p v-if="alertsEnabled && !emailEnabled" class="mt-2 text-xs text-amber-600 dark:text-amber-400">{{ t('traffic.host.alerts.emailOff') }}</p>
      </section>

      <!-- 高级设置 -->
      <details class="group rounded-2xl border border-gray-200 px-4 py-3 dark:border-dark-700" :open="advancedOpen" data-testid="host-plan-advanced">
        <summary class="flex cursor-pointer list-none items-center justify-between text-sm font-medium text-gray-700 dark:text-dark-100">
          {{ t('traffic.host.form.advanced') }}
          <Icon name="chevronDown" size="sm" class="text-gray-400 transition-transform duration-200 group-open:rotate-180" />
        </summary>
        <div class="mt-4 space-y-5">
          <div class="grid gap-5 sm:grid-cols-2">
            <div>
              <label for="host-reset-day" class="input-label">{{ t('traffic.host.form.resetDay') }}</label>
              <input
                id="host-reset-day"
                v-model.number="resetDay"
                type="number"
                min="1"
                max="31"
                class="input w-28 tabular-nums"
                :class="{ 'input-error': errors.resetDay }"
                data-testid="host-plan-reset-day"
                @input="dayTouched = true"
              />
              <p v-if="errors.resetDay" class="input-error-text">{{ errors.resetDay }}</p>
              <p v-else class="input-hint">{{ t('traffic.host.form.resetDayHint') }}</p>
            </div>
            <div>
              <label for="host-reset-time" class="input-label">{{ t('traffic.host.form.resetTime') }}</label>
              <select id="host-reset-time" v-model="resetTime" class="input w-32 tabular-nums" data-testid="host-plan-reset-time">
                <option v-for="time in RESET_TIMES" :key="time" :value="time">{{ time }}</option>
              </select>
            </div>
          </div>

          <fieldset>
            <legend class="input-label">{{ t('traffic.host.form.unitBase') }}</legend>
            <div class="flex flex-wrap gap-x-6 gap-y-2">
              <label v-for="base in UNIT_BASES" :key="base" class="flex items-center gap-2 text-sm text-gray-700 dark:text-dark-200">
                <input v-model="unitBase" type="radio" name="host-unit-base" :value="base" class="h-4 w-4 border-gray-300 text-primary-600 focus:ring-primary-500" :data-testid="`host-plan-base-${base}`" />
                {{ t(base === 1000 ? 'traffic.host.form.unitBase1000' : 'traffic.host.form.unitBase1024') }}
              </label>
            </div>
            <p class="input-hint">{{ t('traffic.host.form.unitBaseHint') }}</p>
          </fieldset>

          <fieldset v-if="interfaces.length">
            <legend class="input-label">{{ t('traffic.host.form.interfaces') }}</legend>
            <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-dark-200">
              <input v-model="interfaceAuto" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" data-testid="host-plan-interfaces-auto" />
              {{ t('traffic.host.form.interfacesAuto') }}
            </label>
            <ul v-if="!interfaceAuto" class="mt-2 space-y-1.5" data-testid="host-plan-interfaces">
              <li v-for="item in interfaces" :key="item.name">
                <label class="flex items-center gap-2 rounded-lg px-2 py-1.5 text-sm hover:bg-gray-50 dark:hover:bg-dark-700/40">
                  <input v-model="selectedNames" type="checkbox" :value="item.name" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
                  <span class="font-medium text-gray-900 dark:text-white">{{ item.name }}</span>
                  <span v-if="item.default_route?.ipv4 || item.default_route?.ipv6" class="badge badge-primary">{{ t('traffic.host.form.defaultRoute') }}</span>
                  <span class="ml-auto text-xs tabular-nums text-gray-500 dark:text-dark-400">{{ interfaceRate(item) }}</span>
                </label>
              </li>
            </ul>
            <p v-if="errors.interfaces" class="input-error-text">{{ errors.interfaces }}</p>
            <p v-else class="input-hint">{{ t('traffic.host.form.interfacesHint') }}</p>
          </fieldset>
        </div>
      </details>
    </form>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button type="button" class="btn btn-secondary" :disabled="saving" @click="emit('close')">{{ t('common.cancel') }}</button>
        <button type="submit" form="host-plan-form" class="btn btn-primary" :disabled="saving" data-testid="host-plan-save">
          {{ saving ? t('traffic.host.form.saving') : t('traffic.host.form.save') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Toggle from '@/components/common/Toggle.vue'
import Icon from '@/components/icons/Icon.vue'
import {
  saveHostTrafficPlan,
  type HostBillingMode,
  type HostInterface,
  type HostQuotaUnit,
  type HostTrafficPlanInput,
  type HostTrafficStatus,
  type HostUnitBase,
  type HostUsedUnit
} from '@/api/admin/hostTraffic'
import { isAmountText, isValidTimeZone, nextResetDate, resetDayFromDate, resetRuleText } from './hostFormat'
import { hostErrorMessage } from './useHostTraffic'
import { formatMbps } from '@/utils/trafficFormat'

const props = defineProps<{
  status: HostTrafficStatus
  // 例如对账提示建议切换进制时，打开表单并预选。
  patch?: { unit_base: HostUnitBase } | null
}>()

const emit = defineEmits<{
  close: []
  saved: [status: HostTrafficStatus, filled: boolean]
}>()

const { t } = useI18n()

const BILLING_MODES: HostBillingMode[] = ['both', 'outbound', 'inbound', 'max']
const UNIT_BASES: HostUnitBase[] = [1000, 1024]
const RESET_TIMES = Array.from({ length: 96 }, (_, index) => {
  const hour = String(Math.floor(index / 4)).padStart(2, '0')
  const minute = String((index % 4) * 15).padStart(2, '0')
  return `${hour}:${minute}`
})
const COMMON_ZONES = [
  'UTC',
  'Asia/Shanghai',
  'Asia/Hong_Kong',
  'Asia/Tokyo',
  'Asia/Singapore',
  'Europe/London',
  'Europe/Berlin',
  'Europe/Amsterdam',
  'America/New_York',
  'America/Chicago',
  'America/Los_Angeles',
  'Australia/Sydney'
]

const plan = props.status.plan
const isEdit = plan !== null
const interfaces = computed<HostInterface[]>(() => props.status.interfaces)
const emailEnabled = computed(() => props.status.alerts.email_enabled)

const quotaValue = ref(plan?.quota_value ?? '')
const quotaUnit = ref<HostQuotaUnit>(plan?.quota_unit ?? 'TB')
const billingMode = ref<HostBillingMode>(plan?.billing_mode ?? 'both')
const nextReset = ref(nextResetDate(props.status.period, plan))
const resetDay = ref<number | ''>(plan?.reset_day ?? '')
const dayTouched = ref(false)
const resetTime = ref(plan?.reset_time ?? '00:00')
const timezone = ref(plan?.timezone ?? 'UTC')
const unitBase = ref<HostUnitBase>(props.patch?.unit_base ?? plan?.unit_base ?? 1000)
const interfaceAuto = ref(!plan || plan.interfaces.length === 0)
const selectedNames = ref<string[]>(
  plan?.interfaces.length ? [...plan.interfaces] : props.status.interfaces.filter((item) => item.selected).map((item) => item.name)
)
const alertsEnabled = ref(plan?.alerts.enabled ?? true)
const warningPercent = ref<number | ''>(plan?.alerts.warning_percent ?? 80)
const criticalPercent = ref<number | ''>(plan?.alerts.critical_percent ?? 95)
const projectionAlert = ref(plan?.alerts.projection ?? true)
const currentUsedValue = ref('')
const currentUsedUnit = ref<HostUsedUnit>('GB')

// 进制被预选、网卡有歧义时默认展开高级设置。
const advancedOpen = Boolean(props.patch) || props.status.interface_selection.ambiguous

const saving = ref(false)
const submitError = ref('')
const errors = reactive<Record<string, string>>({})

const commonZones = computed(() => {
  const zones = [...COMMON_ZONES]
  if (!zones.includes(timezone.value)) zones.unshift(timezone.value)
  return zones
})

const otherZones = computed(() => {
  const intl = Intl as unknown as { supportedValuesOf?: (key: string) => string[] }
  const all = typeof intl.supportedValuesOf === 'function' ? intl.supportedValuesOf('timeZone') : []
  return all.filter((zone) => !commonZones.value.includes(zone))
})

const derived = computed(() => resetDayFromDate(nextReset.value))
// 月末日期可能是短月提前后的结果；用户已改成更大的重置日时不再提示。
const maybeClamped = computed(() => Boolean(derived.value?.maybeClamped && !dayTouched.value && resetDay.value === derived.value.day))

// 日期变化时同步每月重置日，除非用户已在高级设置中手动修改。
watch(derived, (value) => {
  if (value && !dayTouched.value) resetDay.value = value.day
})

const ruleText = computed(() => {
  if (typeof resetDay.value !== 'number' || resetDay.value < 1 || resetDay.value > 31) return ''
  return resetRuleText({ reset_day: resetDay.value, reset_time: resetTime.value, timezone: timezone.value }, t)
})

function unitLabel(unit: 'MB' | 'GB' | 'TB') {
  if (unitBase.value === 1000) return unit
  return `${unit[0]}iB`
}

function interfaceRate(item: HostInterface) {
  if (item.rx_bytes_per_second === null || item.tx_bytes_per_second === null) return ''
  return t('traffic.host.interfaceRate', { rx: formatMbps(item.rx_bytes_per_second), tx: formatMbps(item.tx_bytes_per_second) })
}

function validate(): boolean {
  for (const key of Object.keys(errors)) delete errors[key]
  if (!isAmountText(quotaValue.value)) errors.quota = t('traffic.host.form.errors.quota')
  if (!isEdit || nextReset.value) {
    if (!derived.value) errors.date = t('traffic.host.form.errors.date')
  }
  if (typeof resetDay.value !== 'number' || !Number.isInteger(resetDay.value) || resetDay.value < 1 || resetDay.value > 31) {
    errors.resetDay = t('traffic.host.form.errors.resetDay')
  }
  if (!isValidTimeZone(timezone.value)) errors.timezone = t('traffic.host.form.errors.timezone')
  if (alertsEnabled.value) {
    const warning = warningPercent.value
    const critical = criticalPercent.value
    const valid = (value: number | '') => typeof value === 'number' && Number.isInteger(value) && value >= 1 && value <= 100
    if (!valid(warning) || !valid(critical) || (warning as number) >= (critical as number)) {
      errors.percent = t('traffic.host.form.errors.percent')
    }
  }
  if (!isEdit && currentUsedValue.value.trim() && !isAmountText(currentUsedValue.value, true)) {
    errors.currentUsed = t('traffic.host.form.errors.currentUsed')
  }
  if (!interfaceAuto.value && selectedNames.value.length === 0) errors.interfaces = t('traffic.host.form.errors.interfaces')
  return Object.keys(errors).length === 0
}

const FIELD_ERRORS: Record<string, string> = {
  quota: 'quota',
  reset_day: 'resetDay',
  timezone: 'timezone',
  alerts: 'percent',
  current_used: 'currentUsed',
  interfaces: 'interfaces'
}

async function submit() {
  submitError.value = ''
  if (!validate() || saving.value) return
  const filled = !isEdit && currentUsedValue.value.trim() !== ''
  const input: HostTrafficPlanInput = {
    version: plan?.version ?? null,
    quota_value: quotaValue.value.trim(),
    quota_unit: quotaUnit.value,
    billing_mode: billingMode.value,
    reset_day: resetDay.value as number,
    reset_time: resetTime.value,
    timezone: timezone.value,
    unit_base: unitBase.value,
    interfaces: interfaceAuto.value ? [] : [...selectedNames.value],
    alerts: {
      enabled: alertsEnabled.value,
      warning_percent: typeof warningPercent.value === 'number' ? warningPercent.value : 80,
      critical_percent: typeof criticalPercent.value === 'number' ? criticalPercent.value : 95,
      projection: projectionAlert.value
    },
    current_used: filled ? { value: currentUsedValue.value.trim(), unit: currentUsedUnit.value } : null
  }
  saving.value = true
  try {
    const result = await saveHostTrafficPlan(input)
    emit('saved', result, filled)
  } catch (e) {
    const field = (e as { metadata?: { field?: string } } | null)?.metadata?.field
    if (field && FIELD_ERRORS[field]) errors[FIELD_ERRORS[field]] = t('traffic.host.errors.invalid')
    submitError.value = hostErrorMessage(e, t)
  } finally {
    saving.value = false
  }
}
</script>
