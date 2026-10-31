<template>
  <section
    class="overflow-hidden rounded-2xl border border-gray-200/80 bg-white shadow-card dark:border-dark-700 dark:bg-dark-800/60 dark:shadow-none"
  >
    <!-- 标题与时间范围 -->
    <header
      class="flex flex-col gap-3 border-b border-gray-100 px-4 py-3.5 dark:border-dark-700/70 sm:flex-row sm:items-center sm:justify-between"
    >
      <div class="flex min-w-0 items-center gap-3">
        <span
          class="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-primary-50 text-primary-600 dark:bg-primary-900/25 dark:text-primary-400"
        >
          <Icon name="chartBar" size="sm" />
        </span>
        <div class="min-w-0">
          <div class="flex flex-wrap items-center gap-2">
            <h4 class="text-sm font-semibold text-gray-900 dark:text-white">
              {{ t('wireguard.traffic') }}
            </h4>
            <span class="badge" :class="running ? 'badge-success' : 'badge-gray'">
              <span
                class="h-1.5 w-1.5 rounded-full"
                :class="running ? 'bg-emerald-500 motion-safe:animate-pulse' : 'bg-gray-400 dark:bg-dark-400'"
              ></span>
              {{ running ? t('wireguard.running') : t('wireguard.notRunning') }}
            </span>
          </div>
          <p class="mt-0.5 truncate text-[11px] text-gray-400 dark:text-dark-400">
            {{ t('wireguard.saved') }} · {{ fullDate(node.saved_at) }}
          </p>
        </div>
      </div>

      <div
        class="inline-flex shrink-0 flex-wrap gap-1 self-start rounded-xl border border-gray-200 bg-gray-50 p-1 dark:border-dark-700 dark:bg-dark-900/60 sm:self-auto"
        role="group"
        :aria-label="t('wireguard.history')"
      >
        <button
          v-for="option in rangeOptions"
          :key="option.value"
          type="button"
          class="rounded-lg px-2.5 py-1.5 text-xs font-medium transition-colors duration-150 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/40"
          :class="
            isRangeActive(option.value)
              ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-700 dark:text-white'
              : 'text-gray-500 hover:text-gray-800 dark:text-dark-400 dark:hover:text-dark-200'
          "
          :aria-pressed="isRangeActive(option.value)"
          @click="selectRange(option.value)"
        >
          {{ option.label }}
        </button>
      </div>
    </header>

    <!-- 自选日期 -->
    <div
      v-if="customOpen"
      class="border-b border-gray-100 bg-gray-50/50 px-4 py-3 dark:border-dark-700/70 dark:bg-dark-900/20"
    >
      <div class="flex flex-wrap items-end gap-3">
        <label class="flex flex-col gap-1 text-[11px] font-medium text-gray-500 dark:text-dark-300">
          {{ t('wireguard.customStart') }}
          <input
            v-model="customStart"
            type="date"
            class="input h-9 w-40 text-xs"
            :min="minStartDate"
            :max="todayDate"
            :aria-label="t('wireguard.customStart')"
          />
        </label>
        <label class="flex h-9 items-center gap-2 text-xs text-gray-600 dark:text-dark-300">
          <input v-model="customToNow" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
          {{ t('wireguard.customToNow') }}
        </label>
        <label
          class="flex flex-col gap-1 text-[11px] font-medium text-gray-500 dark:text-dark-300"
          :class="{ 'opacity-50': customToNow }"
        >
          {{ t('wireguard.customEnd') }}
          <input
            v-model="customEnd"
            type="date"
            :disabled="customToNow"
            :min="customStart || minStartDate"
            :max="todayDate"
            class="input h-9 w-40 text-xs"
            :aria-label="t('wireguard.customEnd')"
          />
        </label>
        <button
          type="button"
          class="btn btn-primary btn-sm w-full sm:w-auto"
          :disabled="Boolean(customValidation)"
          @click="applyCustom"
        >
          {{ t('wireguard.customApply') }}
        </button>
      </div>

      <div class="mt-2 flex flex-wrap items-center gap-x-4 gap-y-1 text-[11px] text-gray-400 dark:text-dark-400">
        <span>{{ dateTimezoneHint }}</span>
        <label class="inline-flex items-center gap-1.5">
          <input v-model="customUtc" type="checkbox" class="h-3.5 w-3.5 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
          {{ t('wireguard.customUtc') }}
        </label>
        <span v-if="minStartDate">{{ t('wireguard.retainedFromHint', { date: minStartDate }) }}</span>
        <span>{{ t('wireguard.customHint') }}</span>
      </div>

      <p v-if="customValidation" class="mt-2 text-[11px] text-amber-600 dark:text-amber-400">
        {{ customValidation.message }}
        <button
          v-if="customValidation.code === 'unaligned'"
          type="button"
          class="ml-1 font-medium underline underline-offset-2"
          @click="customUtc = true"
        >
          {{ t('wireguard.customUseUtc') }}
        </button>
      </p>
    </div>

    <div class="space-y-3 p-4">
      <p
        v-if="error"
        role="alert"
        class="rounded-xl bg-red-50 px-3 py-2.5 text-xs leading-relaxed text-red-700 ring-1 ring-red-200/70 dark:bg-red-950/30 dark:text-red-300 dark:ring-red-900/40"
      >
        {{ error }}
      </p>

      <!-- 实际统计区间与时区 -->
      <div
        v-if="report"
        class="flex flex-wrap items-center gap-x-2 gap-y-1 px-1 text-[11px] text-gray-500 dark:text-dark-400"
      >
        <Icon name="clock" size="xs" class="h-3.5 w-3.5 shrink-0" />
        <span class="font-medium text-gray-600 dark:text-dark-200">{{ rangeLabel }}</span>
        <span aria-hidden="true">·</span>
        <span :title="t('wireguard.timeZone')">{{ timezoneLabel }}</span>
        <span aria-hidden="true">·</span>
        <span>{{ intervalHint }}</span>
        <span v-if="report.has_records">· {{ t('wireguard.bucketsCount', { count: report.bucket_count }) }}</span>
        <span v-if="report.end_clamped" class="text-amber-600 dark:text-amber-400">
          · {{ t('wireguard.endClamped') }}
        </span>
      </div>

      <!-- 周期发送 / 接收 / 合计 -->
      <div v-if="loading && !report" class="grid gap-3 sm:grid-cols-3">
        <div
          v-for="index in 3"
          :key="index"
          class="h-[92px] animate-pulse rounded-xl border border-gray-100 bg-gray-100 dark:border-dark-700/70 dark:bg-dark-700/60"
        ></div>
      </div>
      <div v-else class="grid gap-3 sm:grid-cols-3">
        <div
          v-for="metric in metrics"
          :key="metric.key"
          class="rounded-xl border border-gray-100 bg-gray-50/70 p-3.5 transition-colors duration-200 hover:border-gray-200 dark:border-dark-700/70 dark:bg-dark-900/30 dark:hover:border-dark-600"
        >
          <div class="flex items-center gap-2 text-xs font-medium text-gray-500 dark:text-dark-300">
            <span class="h-2 w-2 rounded-full" :style="{ backgroundColor: metric.color }"></span>
            <span>{{ metric.label }}</span>
            <span
              v-if="metric.key === 'total'"
              class="ml-auto text-[10px] font-normal text-gray-400 dark:text-dark-500"
            >
              {{ t('wireguard.periodTraffic') }}
            </span>
          </div>
          <div class="mt-2 flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1">
            <span class="text-2xl font-semibold tracking-tight text-gray-900 tabular-nums dark:text-white">
              {{ metric.value }}
            </span>
            <span
              v-if="metric.rate !== null"
              class="inline-flex items-center gap-1 rounded-full bg-white px-2 py-0.5 text-[11px] font-medium text-gray-500 ring-1 ring-gray-200/80 tabular-nums dark:bg-dark-800 dark:text-dark-300 dark:ring-dark-700"
              :title="t('wireguard.rate')"
            >
              <Icon :name="metric.icon" size="xs" class="h-3 w-3" />
              {{ formatRate(metric.rate) }}
            </span>
          </div>
        </div>
      </div>

      <p
        v-if="report && !report.has_records"
        class="px-1 text-[11px] leading-relaxed text-gray-400 dark:text-dark-400"
      >
        {{ t('wireguard.noRecordsHint') }}
      </p>

      <!-- 流量历史 -->
      <div class="rounded-xl border border-gray-100 bg-white p-3 dark:border-dark-700/70 dark:bg-dark-900/20">
        <div class="mb-2 flex flex-wrap items-center justify-between gap-2 px-1">
          <div class="flex items-center gap-3">
            <span
              v-for="metric in seriesMetrics"
              :key="metric.key"
              class="inline-flex items-center gap-1.5 text-[11px] font-medium text-gray-500 dark:text-dark-300"
            >
              <span class="h-1 w-4 rounded-full" :style="{ backgroundColor: metric.color }"></span>
              {{ metric.label }}
            </span>
          </div>
          <span v-if="report?.has_records" class="text-[11px] text-gray-400 dark:text-dark-500">
            {{ t('wireguard.gapsHint') }}
          </span>
        </div>
        <div
          class="relative h-48 sm:h-52"
          :role="showChart ? 'img' : undefined"
          :aria-label="showChart ? chartAriaLabel : undefined"
        >
          <Transition
            enter-active-class="motion-safe:transition-opacity motion-safe:duration-200 motion-safe:ease-out motion-reduce:transition-none"
            enter-from-class="opacity-0"
            leave-active-class="motion-safe:transition-opacity motion-safe:duration-100 motion-safe:ease-in motion-reduce:transition-none"
            leave-to-class="opacity-0"
            mode="out-in"
          >
            <Line v-if="showChart" key="chart" :data="chartData" :options="chartOptions" />
            <div
              v-else-if="loading"
              key="loading"
              class="flex h-full flex-col justify-end gap-2 rounded-lg border border-gray-100 bg-gray-50/60 p-4 dark:border-dark-700 dark:bg-dark-900/30"
            >
              <div class="h-28 w-full animate-pulse rounded-lg bg-gray-100 dark:bg-dark-700/60"></div>
              <div class="h-3 w-1/2 animate-pulse rounded bg-gray-100 dark:bg-dark-700/60"></div>
            </div>
            <div
              v-else
              key="empty"
              class="flex h-full flex-col items-center justify-center gap-1.5 rounded-lg border border-dashed border-gray-200 bg-gray-50/60 px-4 text-center dark:border-dark-700 dark:bg-dark-900/30"
            >
              <Icon name="chart" size="md" class="text-gray-300 dark:text-dark-500" />
              <p class="text-sm font-medium text-gray-500 dark:text-dark-300">
                {{ emptyTitle }}
              </p>
              <p v-if="emptyHint" class="max-w-xs text-[11px] leading-relaxed text-gray-400 dark:text-dark-500">
                {{ emptyHint }}
              </p>
            </div>
          </Transition>
        </div>
      </div>

      <!-- 统计告警 -->
      <div
        v-if="report?.incomplete || telemetryGap"
        class="space-y-1.5 rounded-xl bg-amber-50/80 px-3 py-2.5 text-[11px] leading-relaxed text-amber-700 ring-1 ring-amber-200/60 dark:bg-amber-950/25 dark:text-amber-300 dark:ring-amber-900/40"
      >
        <p v-if="report?.incomplete" class="flex items-start gap-1.5">
          <Icon name="exclamationTriangle" size="xs" class="mt-px h-3.5 w-3.5 shrink-0" />
          <span>{{ t('wireguard.incomplete') }}</span>
        </p>
        <p v-if="telemetryGap" class="flex items-start gap-1.5">
          <Icon name="exclamationTriangle" size="xs" class="mt-px h-3.5 w-3.5 shrink-0" />
          <span>{{ t('wireguard.telemetryGap') }}</span>
        </p>
      </div>
    </div>

    <!-- 统计口径 -->
    <footer
      class="flex flex-wrap items-center justify-between gap-2 border-t border-gray-100 px-4 py-2.5 text-[11px] text-gray-400 dark:border-dark-700/70 dark:text-dark-400"
    >
      <span class="inline-flex items-center gap-1.5">
        {{ t('wireguard.metricBasis') }}
        <HelpTooltip :content="t('wireguard.basis')" width-class="w-72" />
      </span>
      <span v-if="rateAvailable" class="tabular-nums">
        {{ t('wireguard.sampledAt') }} · {{ clock(rate.sampled_at) }}
      </span>
    </footer>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  Chart as ChartJS,
  Filler,
  Legend,
  LineElement,
  LinearScale,
  PointElement,
  Tooltip,
  type ChartData,
  type ChartOptions,
  type ScriptableContext,
  type TooltipItem
} from 'chart.js'
import { Line } from 'vue-chartjs'
import Icon from '@/components/icons/Icon.vue'
import HelpTooltip from '@/components/common/HelpTooltip.vue'
import type {
  TrafficRange,
  TrafficReport,
  TrafficSelection,
  WGDetails,
  WGNode
} from '@/api/admin/wireguard'
import {
  firstSelectableDate,
  formatDateInput,
  formatDateLabel,
  formatDateTimeLabel,
  formatRFC3339,
  isUtcBucketAligned,
  nextCalendarDay,
  parseDateInput,
  timeZoneName,
  utcOffsetLabel,
  type DateMode
} from '@/utils/wireguardTraffic'

ChartJS.register(LinearScale, PointElement, LineElement, Filler, Legend, Tooltip)

type MetricIcon = 'arrowUp' | 'arrowDown' | 'chartBar'
type CustomIssueCode =
  | 'startRequired'
  | 'endRequired'
  | 'invalid'
  | 'order'
  | 'startFuture'
  | 'unaligned'
  | 'endTooFar'
  | 'retention'

interface Metric {
  key: 'sent' | 'received' | 'total'
  label: string
  value: string
  rate: number | null
  color: string
  icon: MetricIcon
}

interface LinePoint {
  x: number
  y: number | null
}

const props = defineProps<{
  node: WGNode
  rate: WGDetails['rate']
  report: TrafficReport | null
  selectedRange: TrafficRange
  loading?: boolean
  error?: string
  telemetryGap?: boolean
  retainedFrom?: string
}>()

const emit = defineEmits<{ select: [selection: TrafficSelection] }>()

const { t } = useI18n()

const reduceMotion =
  typeof window !== 'undefined' &&
  typeof window.matchMedia === 'function' &&
  window.matchMedia('(prefers-reduced-motion: reduce)').matches
const animateChart = ref(!reduceMotion)
let animationTimer: ReturnType<typeof setTimeout> | undefined
onMounted(() => {
  if (!reduceMotion) {
    animationTimer = setTimeout(() => {
      animateChart.value = false
    }, 800)
  }
})
onUnmounted(() => {
  if (animationTimer) clearTimeout(animationTimer)
})

// 系列配色经色盲可区分性校验：浅色 #0d9488/#6250d6，深色 #0d9488/#9085e9。
const palette = (() => {
  const dark = document.documentElement.classList.contains('dark')
  return {
    sent: '#0d9488',
    received: dark ? '#9085e9' : '#6250d6',
    sentFill: dark ? 'rgba(13, 148, 136, 0.30)' : 'rgba(13, 148, 136, 0.22)',
    receivedFill: dark ? 'rgba(144, 133, 233, 0.26)' : 'rgba(98, 80, 214, 0.16)',
    total: dark ? '#94a3b8' : '#64748b',
    surface: dark ? '#1e293b' : '#ffffff',
    grid: dark ? 'rgba(148, 163, 184, 0.14)' : 'rgba(15, 23, 42, 0.06)',
    ticks: dark ? '#94a3b8' : '#64748b',
    tooltipBg: dark ? '#0f172a' : '#ffffff',
    tooltipTitle: dark ? '#f8fafc' : '#0f172a',
    tooltipBody: dark ? '#cbd5e1' : '#475569',
    tooltipBorder: dark ? '#334155' : '#e2e8f0'
  }
})()

const running = computed(() => props.rate.state !== 'not_running')
const rateAvailable = computed(
  () => !['not_running', 'no_sample', 'counter_reset'].includes(props.rate.state)
)

const rangeOptions = computed<{ value: TrafficRange; label: string }[]>(() => [
  { value: '24h', label: t('wireguard.range24h') },
  { value: '7d', label: t('wireguard.range7d') },
  { value: '30d', label: t('wireguard.range30d') },
  { value: '90d', label: t('wireguard.range90d') },
  { value: 'custom', label: t('wireguard.customRange') }
])

const customOpen = ref(false)
const customToNow = ref(true)
const customEnd = ref('')
const customUtc = ref(false)
const nowTick = ref(Date.now())
const initialStart = new Date()
initialStart.setDate(initialStart.getDate() - 7)
const customStart = ref(formatDateInput(initialStart, 'local'))
let clockTimer: ReturnType<typeof setInterval> | undefined

const dateMode = computed<DateMode>(() => (customUtc.value ? 'utc' : 'local'))

watch(
  () => props.selectedRange,
  (range) => {
    if (range !== 'custom') customOpen.value = false
  }
)

watch(customOpen, (open) => {
  if (clockTimer) {
    clearInterval(clockTimer)
    clockTimer = undefined
  }
  if (open) {
    nowTick.value = Date.now()
    clockTimer = setInterval(() => {
      nowTick.value = Date.now()
    }, 60_000)
  }
})

onUnmounted(() => {
  if (clockTimer) clearInterval(clockTimer)
})

function isRangeActive(range: TrafficRange) {
  return customOpen.value ? range === 'custom' : props.selectedRange === range
}

function selectRange(range: TrafficRange) {
  if (range !== 'custom') {
    customOpen.value = false
    emit('select', { range })
    return
  }
  if (customOpen.value) {
    customOpen.value = false
    return
  }
  customOpen.value = true
  nowTick.value = Date.now()
  if (props.selectedRange !== 'custom') applyCustom()
}

const minStartDate = computed(() => {
  if (!props.retainedFrom) return undefined
  const retained = new Date(props.retainedFrom)
  if (Number.isNaN(retained.getTime())) return undefined
  return formatDateInput(firstSelectableDate(retained, dateMode.value), dateMode.value)
})

const todayDate = computed(() => formatDateInput(new Date(nowTick.value), dateMode.value))

const customValidation = computed<{ code: CustomIssueCode; message: string } | null>(() => {
  const mode = dateMode.value
  if (!customStart.value) {
    return { code: 'startRequired', message: t('wireguard.customStartRequired') }
  }
  const start = parseDateInput(customStart.value, mode)
  if (!start) {
    return { code: 'invalid', message: t('wireguard.customStartRequired') }
  }
  const nowMs = nowTick.value
  let end: Date | null = null
  if (!customToNow.value) {
    if (!customEnd.value) {
      return { code: 'endRequired', message: t('wireguard.customEndRequired') }
    }
    const endDay = parseDateInput(customEnd.value, mode)
    if (!endDay) {
      return { code: 'invalid', message: t('wireguard.customEndRequired') }
    }
    end = nextCalendarDay(endDay, mode)
  }
  if (start.getTime() >= nowMs) {
    return { code: 'startFuture', message: t('wireguard.customStartFuture') }
  }
  if (end && start.getTime() >= end.getTime()) {
    return { code: 'order', message: t('wireguard.customOrderInvalid') }
  }
  if (!isUtcBucketAligned(start)) {
    return { code: 'unaligned', message: t('wireguard.customBoundaryUnaligned') }
  }
  if (end && end.getTime() <= nowMs && !isUtcBucketAligned(end)) {
    return { code: 'unaligned', message: t('wireguard.customBoundaryUnaligned') }
  }
  if (end && end.getTime() > nowMs + 25 * 3_600_000) {
    return { code: 'endTooFar', message: t('wireguard.customEndTooFar') }
  }
  if (props.retainedFrom) {
    const retained = new Date(props.retainedFrom)
    if (!Number.isNaN(retained.getTime()) && start.getTime() < retained.getTime()) {
      return {
        code: 'retention',
        message: t('wireguard.customRetentionExceeded', {
          date: minStartDate.value || formatDateLabel(retained)
        })
      }
    }
  }
  return null
})

function applyCustom() {
  if (customValidation.value || !customStart.value) return
  const mode = dateMode.value
  const start = parseDateInput(customStart.value, mode)
  if (!start) return
  const endDay = customToNow.value ? null : parseDateInput(customEnd.value, mode)
  const end = endDay ? nextCalendarDay(endDay, mode) : null
  emit('select', {
    range: 'custom',
    start: formatRFC3339(start, mode),
    end: end ? formatRFC3339(end, mode) : undefined
  })
}

const dateTimezoneHint = computed(() => {
  if (customUtc.value) return t('wireguard.dateTimezoneUtc')
  const start = parseDateInput(customStart.value, 'local')
  const offsetDate = start ?? new Date(nowTick.value)
  return t('wireguard.dateTimezoneLocal', {
    zone: `${timeZoneName()} (${utcOffsetLabel(offsetDate)})`
  })
})

const intervalHint = computed(() =>
  props.report?.interval === 'minute' ? t('wireguard.perMinute') : t('wireguard.perHour')
)

const rangeLabel = computed(() =>
  props.report
    ? `${formatDateTimeLabel(props.report.start)} – ${formatDateTimeLabel(props.report.end)}`
    : '—'
)

const timezoneLabel = computed(() => {
  if (!props.report) return timeZoneName()
  const startOffset = utcOffsetLabel(new Date(props.report.start))
  const endOffset = utcOffsetLabel(new Date(props.report.end))
  const name = timeZoneName()
  return startOffset === endOffset ? `${name} (${startOffset})` : `${name} (${startOffset} → ${endOffset})`
})

const metrics = computed<Metric[]>(() => {
  const report = props.report
  const hasRecords = report?.has_records ?? false
  return [
    {
      key: 'sent',
      label: t('wireguard.sent'),
      value: hasRecords ? formatBytes(report!.tx_bytes) : '—',
      rate: props.rate.tx_bytes_per_second,
      color: palette.sent,
      icon: 'arrowUp'
    },
    {
      key: 'received',
      label: t('wireguard.received'),
      value: hasRecords ? formatBytes(report!.rx_bytes) : '—',
      rate: props.rate.rx_bytes_per_second,
      color: palette.received,
      icon: 'arrowDown'
    },
    {
      key: 'total',
      label: t('wireguard.total'),
      value: hasRecords ? formatBytes(report!.total_bytes) : '—',
      rate: null,
      color: palette.total,
      icon: 'chartBar'
    }
  ]
})

const seriesMetrics = computed(() => metrics.value.filter((metric) => metric.key !== 'total'))

const showChart = computed(() => Boolean(props.report?.has_records && props.report.history.length))

const chartAriaLabel = computed(() =>
  props.report
    ? `${t('wireguard.traffic')}: ${t('wireguard.sent')} ${formatBytes(props.report.tx_bytes)}, ${t('wireguard.received')} ${formatBytes(props.report.rx_bytes)}, ${t('wireguard.total')} ${formatBytes(props.report.total_bytes)}`
    : ''
)

const emptyTitle = computed(() => {
  if (props.error) return t('wireguard.trafficLoadError')
  if (props.report && !props.report.has_records) return t('wireguard.noRecords')
  return t('common.noData')
})

const emptyHint = computed(() =>
  props.report && !props.report.has_records ? t('wireguard.noRecordsHint') : ''
)

function formatBytes(value: string | number, decimals = 2) {
  const number = Number(value)
  if (!Number.isFinite(number) || number <= 0) return '0 B'
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB']
  let size = number
  let unit = 0
  while (size >= 1024 && unit < units.length - 1) {
    size /= 1024
    unit++
  }
  const text = unit === 0 ? String(Math.round(size)) : String(parseFloat(size.toFixed(decimals)))
  return `${text} ${units[unit]}`
}

function formatRate(value: number) {
  return rateAvailable.value ? `${formatBytes(value)}/s` : '—'
}

function fullDate(value?: string) {
  return value ? new Date(value).toLocaleString() : '—'
}

function clock(value?: string) {
  return value ? new Date(value).toLocaleTimeString() : '—'
}

// 同一根轴的刻度共用一个单位，避免出现 976.6 KiB 与 1.9 MiB 混排。
const axisUnit = computed(() => {
  const max = (props.report?.history ?? []).reduce(
    (value, bucket) => Math.max(value, Number(bucket.tx_bytes), Number(bucket.rx_bytes)),
    0
  )
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB']
  let divisor = 1
  let unit = 0
  while (max / divisor >= 1024 && unit < units.length - 1) {
    divisor *= 1024
    unit++
  }
  return { divisor, label: units[unit] }
})

const bucketDuration = computed(() =>
  props.report?.interval === 'minute' ? 60_000 : 3_600_000
)

// history 只含实际保存的记录；在缺失桶处插入断点，避免把缺失区间连成连续曲线。
function buildSeries(key: 'tx_bytes' | 'rx_bytes'): LinePoint[] {
  const points: LinePoint[] = []
  let previous: number | null = null
  for (const bucket of props.report?.history ?? []) {
    const x = Date.parse(bucket.start)
    if (previous !== null && x - previous > bucketDuration.value * 1.5) {
      points.push({ x: previous + bucketDuration.value, y: null })
    }
    points.push({ x, y: Number(bucket[key]) / axisUnit.value.divisor })
    previous = x
  }
  return points
}

// Chart.js 接受 null 断点，但官方类型只声明了数值点。
function asLineData(points: LinePoint[]) {
  return points as unknown as { x: number; y: number }[]
}

function areaFill(top: string) {
  return (context: ScriptableContext<'line'>) => {
    const { ctx, chartArea } = context.chart
    if (!chartArea) return 'transparent'
    const gradient = ctx.createLinearGradient(0, chartArea.top, 0, chartArea.bottom)
    gradient.addColorStop(0, top)
    gradient.addColorStop(1, 'rgba(0, 0, 0, 0)')
    return gradient
  }
}

const chartData = computed<ChartData<'line'>>(() => ({
  datasets: [
    {
      label: t('wireguard.sent'),
      data: asLineData(buildSeries('tx_bytes')),
      borderColor: palette.sent,
      backgroundColor: areaFill(palette.sentFill),
      fill: true,
      borderWidth: 2,
      tension: 0.35,
      spanGaps: false,
      pointRadius: 0,
      pointHitRadius: 12,
      pointHoverRadius: 4,
      pointHoverBorderWidth: 2,
      pointHoverBorderColor: palette.surface,
      pointHoverBackgroundColor: palette.sent
    },
    {
      label: t('wireguard.received'),
      data: asLineData(buildSeries('rx_bytes')),
      borderColor: palette.received,
      backgroundColor: areaFill(palette.receivedFill),
      fill: true,
      borderWidth: 2,
      tension: 0.35,
      spanGaps: false,
      pointRadius: 0,
      pointHitRadius: 12,
      pointHoverRadius: 4,
      pointHoverBorderWidth: 2,
      pointHoverBorderColor: palette.surface,
      pointHoverBackgroundColor: palette.received
    }
  ]
}))

function axisTime(value: number) {
  const date = new Date(value)
  const pad = (part: number) => String(part).padStart(2, '0')
  if (props.report?.interval === 'minute') {
    return `${pad(date.getHours())}:${pad(date.getMinutes())}`
  }
  const span = props.report
    ? Date.parse(props.report.end) - Date.parse(props.report.start)
    : 0
  const day = `${pad(date.getMonth() + 1)}-${pad(date.getDate())}`
  return span > 36 * 3_600_000 ? day : `${day} ${pad(date.getHours())}:${pad(date.getMinutes())}`
}

const chartOptions = computed<ChartOptions<'line'>>(() => ({
  responsive: true,
  maintainAspectRatio: false,
  animation: animateChart.value ? { duration: 600, easing: 'easeOutQuart' } : false,
  interaction: { intersect: false, mode: 'index' },
  plugins: {
    legend: { display: false },
    tooltip: {
      backgroundColor: palette.tooltipBg,
      titleColor: palette.tooltipTitle,
      bodyColor: palette.tooltipBody,
      borderColor: palette.tooltipBorder,
      borderWidth: 1,
      padding: 10,
      displayColors: true,
      usePointStyle: true,
      boxWidth: 8,
      boxHeight: 8,
      boxPadding: 4,
      callbacks: {
        title: (items: TooltipItem<'line'>[]) => {
          const x = items[0]?.parsed.x
          return typeof x === 'number' ? formatDateTimeLabel(new Date(x)) : ''
        },
        label: (context: TooltipItem<'line'>) => {
          const y = context.parsed.y
          return `${context.dataset.label}: ${typeof y === 'number' ? formatBytes(y * axisUnit.value.divisor) : '—'}`
        }
      }
    }
  },
  scales: {
    x: {
      type: 'linear',
      min: props.report ? Date.parse(props.report.start) : undefined,
      max: props.report ? Date.parse(props.report.end) : undefined,
      grid: { display: false },
      border: { display: false },
      ticks: {
        color: palette.ticks,
        font: { size: 10 },
        maxTicksLimit: 7,
        maxRotation: 0,
        autoSkip: true,
        autoSkipPadding: 12,
        callback: (value) => axisTime(Number(value))
      }
    },
    y: {
      beginAtZero: true,
      grid: { color: palette.grid, drawTicks: false },
      border: { display: false },
      ticks: {
        color: palette.ticks,
        font: { size: 10 },
        maxTicksLimit: 4,
        padding: 6,
        callback: (value) => {
          const scaled = Number(value)
          if (scaled === 0) return '0'
          return `${parseFloat(scaled.toFixed(1))} ${axisUnit.value.label}`
        }
      }
    }
  }
}))
</script>
