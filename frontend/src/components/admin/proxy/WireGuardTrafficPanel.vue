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

      <!-- 节点带宽峰值：自建中转节点每个方向的负载约等于本站发送 + 接收，取同一秒合计的最大值 -->
      <div
        v-if="report"
        data-testid="wg-bandwidth"
        class="rounded-xl border border-primary-100 bg-primary-50/40 p-4 dark:border-primary-900/40 dark:bg-primary-900/10"
      >
        <div class="flex items-center gap-1.5 text-xs font-medium text-gray-600 dark:text-dark-200">
          {{ t('wireguard.nodeBandwidthPeak') }}
          <HelpTooltip :content="t('wireguard.nodeBandwidthPeakHint')" width-class="w-80" />
        </div>
        <div class="mt-1.5 flex flex-wrap items-baseline gap-x-3 gap-y-1">
          <span class="text-3xl font-semibold tracking-tight text-gray-900 tabular-nums dark:text-white">
            {{ peakValueText('total') }}
          </span>
          <span v-if="peakOf('total')" class="text-sm font-medium text-gray-500 tabular-nums dark:text-dark-300">
            ≈ {{ formatMbps(peakOf('total')!.bytes_per_second) }}
          </span>
        </div>
        <p class="mt-1 text-[11px] text-gray-500 tabular-nums dark:text-dark-400">
          {{ peakDetailText('total') || t('wireguard.noRecords') }}
        </p>
      </div>
      <div
        v-else-if="loading"
        class="h-[104px] animate-pulse rounded-xl border border-gray-100 bg-gray-100 dark:border-dark-700/70 dark:bg-dark-700/60"
      ></div>

      <!-- 周期发送 / 接收 / 合计 -->
      <div v-if="loading && !report" class="grid gap-3 md:grid-cols-3">
        <div
          v-for="index in 3"
          :key="index"
          class="h-[148px] animate-pulse rounded-xl border border-gray-100 bg-gray-100 dark:border-dark-700/70 dark:bg-dark-700/60"
        ></div>
      </div>
      <div v-else class="grid gap-3 md:grid-cols-3">
        <div
          v-for="metric in metrics"
          :key="metric.key"
          class="rounded-xl border border-gray-100 bg-gray-50/70 p-3.5 transition-colors duration-200 hover:border-gray-200 dark:border-dark-700/70 dark:bg-dark-900/30 dark:hover:border-dark-600"
        >
          <!-- 速率放在标题行，避免长速率文本把它挤到数值下方 -->
          <div class="flex min-h-[22px] items-center gap-1.5 text-xs font-medium text-gray-500 dark:text-dark-300">
            <span class="h-2 w-2 shrink-0 rounded-full" :style="{ backgroundColor: metric.color }"></span>
            <span class="truncate">{{ metric.label }}</span>
            <span
              v-if="metric.rate !== null"
              class="ml-auto inline-flex shrink-0 items-center gap-1 whitespace-nowrap rounded-full bg-white px-1.5 py-0.5 text-[11px] font-medium text-gray-500 ring-1 ring-gray-200/80 tabular-nums dark:bg-dark-800 dark:text-dark-300 dark:ring-dark-700"
              :title="t('wireguard.rate')"
            >
              <Icon :name="metric.icon" size="xs" class="h-3 w-3" />
              {{ formatRate(metric.rate) }}
            </span>
            <span
              v-else-if="metric.key === 'total'"
              class="ml-auto shrink-0 whitespace-nowrap text-[10px] font-normal text-gray-400 dark:text-dark-500"
            >
              {{ t('wireguard.periodTraffic') }}
            </span>
          </div>
          <div
            class="mt-2 whitespace-nowrap text-2xl font-semibold tracking-tight text-gray-900 tabular-nums dark:text-white"
          >
            {{ metric.value }}
          </div>
          <!-- 所选期间 1 秒平均峰值；升级前历史没有峰值，显示未知而不是 0 -->
          <div class="mt-3 border-t border-gray-200/70 pt-2.5 dark:border-dark-700/70" data-testid="wg-peak">
            <div class="flex items-baseline justify-between gap-2 text-[11px]">
              <span class="text-gray-500 dark:text-dark-400">{{ t('wireguard.peak') }}</span>
              <span class="font-semibold text-gray-800 tabular-nums dark:text-dark-100">
                {{ peakValueText(metric.peakKey) }}
              </span>
            </div>
            <div
              class="mt-0.5 flex min-h-[16px] items-baseline justify-between gap-2 text-[11px] text-gray-400 tabular-nums dark:text-dark-500"
            >
              <span class="truncate">{{ peakDetailText(metric.peakKey) }}</span>
              <span v-if="peakOf(metric.peakKey)" class="shrink-0">
                ≈ {{ formatMbps(peakOf(metric.peakKey)!.bytes_per_second) }}
              </span>
            </div>
          </div>
        </div>
      </div>

      <!-- 峰值观察范围：只覆盖有峰值样本的时段 -->
      <div
        v-if="report?.has_records"
        data-testid="wg-peak-quality"
        class="flex flex-wrap items-center gap-x-2 gap-y-1 px-1 text-[11px] text-gray-500 dark:text-dark-400"
      >
        <span class="inline-flex items-center gap-1 font-medium text-gray-600 dark:text-dark-200">
          {{ t('wireguard.peakObserved') }}
          <HelpTooltip :content="t('wireguard.peakBasis')" width-class="w-72" />
        </span>
        <template v-if="peakObservedText">
          <span class="tabular-nums">{{ peakObservedText }}</span>
          <span aria-hidden="true">·</span>
          <span class="tabular-nums">
            {{
              t('wireguard.peakSamples', {
                valid: report.peak_history?.quality?.valid_samples ?? 0,
                invalid: report.peak_history?.quality?.invalid_samples ?? 0
              })
            }}
          </span>
        </template>
        <span v-else>{{ t('wireguard.peakLegacyHint') }}</span>
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
          <div class="flex flex-wrap items-center gap-x-3 gap-y-1">
            <span
              v-for="metric in seriesMetrics"
              :key="metric.key"
              class="inline-flex items-center gap-1.5 text-[11px] font-medium text-gray-500 dark:text-dark-300"
            >
              <span class="h-2.5 w-2.5 rounded-sm" :style="{ backgroundColor: metric.color }"></span>
              {{ metric.label }}
            </span>
            <span
              v-if="showChart && gaps.length"
              class="inline-flex items-center gap-1.5 text-[11px] font-medium text-gray-500 dark:text-dark-300"
            >
              <span class="h-2.5 w-2.5 rounded-sm ring-1 ring-inset ring-gray-200 dark:ring-dark-600" :style="gapSwatchStyle"></span>
              {{ t('wireguard.noRecordLegend') }}
            </span>
          </div>
          <div v-if="showChart" class="flex items-center gap-2">
            <span class="text-[11px] text-gray-400 tabular-nums dark:text-dark-500">{{ binLabel }}</span>
            <div
              v-if="canFocus"
              class="inline-flex rounded-lg border border-gray-200 bg-gray-50 p-0.5 dark:border-dark-700 dark:bg-dark-900/60"
              role="group"
              :aria-label="t('wireguard.chartRange')"
            >
              <button
                v-for="option in chartViewOptions"
                :key="option.value"
                type="button"
                class="rounded-md px-2 py-0.5 text-[11px] font-medium transition-colors duration-150 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/40"
                :class="
                  chartView === option.value
                    ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-700 dark:text-white'
                    : 'text-gray-500 hover:text-gray-800 dark:text-dark-400 dark:hover:text-dark-200'
                "
                :aria-pressed="chartView === option.value"
                @click="chartView = option.value"
              >
                {{ option.label }}
              </button>
            </div>
          </div>
        </div>
        <div
          ref="chartBox"
          class="relative h-52 sm:h-56"
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
            <Bar v-if="showChart" key="chart" :data="chartData" :options="chartOptions" :plugins="chartPlugins" />
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
        <p v-if="showChart && gaps.length" class="mt-2 px-1 text-[11px] text-gray-400 dark:text-dark-500">
          {{ t('wireguard.gapsHint') }}
        </p>
      </div>

      <!-- 统计告警 -->
      <div
        v-if="report?.incomplete || telemetryGap || invalidPeakSamples > 0 || peakCoverageStart"
        class="space-y-1.5 rounded-xl bg-amber-50/80 px-3 py-2.5 text-[11px] leading-relaxed text-amber-700 ring-1 ring-amber-200/60 dark:bg-amber-950/25 dark:text-amber-300 dark:ring-amber-900/40"
      >
        <p v-if="report?.incomplete" class="flex items-start gap-1.5">
          <Icon name="exclamationTriangle" size="xs" class="mt-px h-3.5 w-3.5 shrink-0" />
          <span>{{ t('wireguard.incomplete') }}</span>
        </p>
        <p v-if="invalidPeakSamples > 0" class="flex items-start gap-1.5">
          <Icon name="exclamationTriangle" size="xs" class="mt-px h-3.5 w-3.5 shrink-0" />
          <span>{{ t('wireguard.invalidSamples', { count: invalidPeakSamples }) }}</span>
        </p>
        <p v-if="peakCoverageStart" class="flex items-start gap-1.5">
          <Icon name="exclamationTriangle" size="xs" class="mt-px h-3.5 w-3.5 shrink-0" />
          <span>{{ t('wireguard.peakPartial', { time: peakCoverageStart }) }}</span>
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
      <span v-if="sampleTimeKnown" class="tabular-nums">
        {{ t('wireguard.sampledAt') }} · {{ clock(rate.sampled_at) }}
      </span>
    </footer>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useElementSize } from '@vueuse/core'
import {
  BarElement,
  Chart as ChartJS,
  Legend,
  LinearScale,
  Tooltip,
  type ChartData,
  type ChartOptions,
  type Plugin,
  type TooltipItem
} from 'chart.js'
import { Bar } from 'vue-chartjs'
import Icon from '@/components/icons/Icon.vue'
import HelpTooltip from '@/components/common/HelpTooltip.vue'
import type {
  TrafficPeak,
  TrafficRange,
  TrafficReport,
  TrafficSelection,
  WGDetails,
  WGNode,
  WGPeakKey
} from '@/api/admin/wireguard'
import { formatByteRate, formatMbps, formatSampleTime, isMissingTime } from '@/utils/trafficFormat'
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
import {
  buildTimeTicks,
  buildTrafficBins,
  durationParts,
  formatBinRange,
  formatTimeTick,
  gapSpans,
  grainMs,
  recordedSpan,
  type TimeSpan
} from '@/utils/wireguardTrafficChart'

ChartJS.register(LinearScale, BarElement, Legend, Tooltip)

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
  peakKey: WGPeakKey
  color: string
  icon: MetricIcon
}

type ChartView = 'focus' | 'full'

interface BarPoint {
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
    sentHover: dark ? '#14b8a6' : '#0f766e',
    receivedHover: dark ? '#a59cf0' : '#4f3fc0',
    gapFill: dark ? 'rgba(148, 163, 184, 0.06)' : 'rgba(100, 116, 139, 0.05)',
    gapStroke: dark ? 'rgba(148, 163, 184, 0.22)' : 'rgba(100, 116, 139, 0.22)',
    total: dark ? '#94a3b8' : '#64748b',
    grid: dark ? 'rgba(148, 163, 184, 0.14)' : 'rgba(15, 23, 42, 0.06)',
    ticks: dark ? '#94a3b8' : '#64748b',
    tooltipBg: dark ? '#0f172a' : '#ffffff',
    tooltipTitle: dark ? '#f8fafc' : '#0f172a',
    tooltipBody: dark ? '#cbd5e1' : '#475569',
    tooltipBorder: dark ? '#334155' : '#e2e8f0'
  }
})()

const running = computed(() => props.rate.state !== 'not_running')
// 只有正常约 1 秒采样才是可信速率；采样延迟、等待采样以及 no_sample/counter_reset 等异常计数状态都不显示数值。
const rateAvailable = computed(() => props.rate.state === 'sampled')
const sampleTimeKnown = computed(
  () => ['sampled', 'invalid_interval'].includes(props.rate.state) && !isMissingTime(props.rate.sampled_at)
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
      peakKey: 'tx',
      color: palette.sent,
      icon: 'arrowUp'
    },
    {
      key: 'received',
      label: t('wireguard.received'),
      value: hasRecords ? formatBytes(report!.rx_bytes) : '—',
      rate: props.rate.rx_bytes_per_second,
      peakKey: 'rx',
      color: palette.received,
      icon: 'arrowDown'
    },
    {
      key: 'total',
      label: t('wireguard.total'),
      value: hasRecords ? formatBytes(report!.total_bytes) : '—',
      rate: null,
      peakKey: 'total',
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
  if (rateAvailable.value) return `${formatBytes(value)}/s`
  if (props.rate.state === 'invalid_interval') return t('wireguard.rateDelayed')
  if (props.rate.state === 'warming_up') return t('wireguard.rateWarmingUp')
  return '—'
}

// 24 小时等所选期间的峰值直接读取 peak_history，不从曲线平均速率反推。
function peakOf(key: WGPeakKey): TrafficPeak | null {
  return props.report?.peak_history?.peaks?.[key] ?? null
}

function peakValueText(key: WGPeakKey) {
  if (!props.report?.has_records) return '—'
  const peak = peakOf(key)
  return peak ? formatByteRate(peak.bytes_per_second) : t('wireguard.peakUnknown')
}

function peakDetailText(key: WGPeakKey) {
  const report = props.report
  if (!report?.has_records) return ''
  const peak = peakOf(key)
  if (peak) return formatSampleTime(peak.at)
  return report.peak_history ? t('wireguard.peakNoValidSample') : t('wireguard.peakLegacy')
}

const peakObservedText = computed(() => {
  const quality = props.report?.peak_history?.quality
  if (!quality || isMissingTime(quality.first_sample_at) || isMissingTime(quality.last_sample_at)) return ''
  return `${formatSampleTime(quality.first_sample_at)} – ${formatSampleTime(quality.last_sample_at)}`
})

const invalidPeakSamples = computed(() => props.report?.peak_history?.quality?.invalid_samples ?? 0)

// 升级前与升级后历史混合时，首个峰值样本晚于最早记录超过一个时间桶，提示峰值不覆盖整个周期。
const peakCoverageStart = computed(() => {
  const report = props.report
  const first = report?.peak_history?.quality?.first_sample_at
  if (!report?.has_records || !report.first_record_at || !first || isMissingTime(first)) return ''
  const grain = report.interval === 'minute' ? 60_000 : 3_600_000
  return Date.parse(first) - Date.parse(report.first_record_at) >= grain ? formatSampleTime(first) : ''
})

function fullDate(value?: string) {
  return value ? new Date(value).toLocaleString() : '—'
}

function clock(value?: string) {
  return value ? new Date(value).toLocaleTimeString() : '—'
}

const chartView = ref<ChartView>('focus')
const chartBox = ref<HTMLElement | null>(null)
const { width: chartWidth } = useElementSize(chartBox)

const chartViewOptions = computed<{ value: ChartView; label: string }[]>(() => [
  { value: 'focus', label: t('wireguard.chartFocus') },
  { value: 'full', label: t('wireguard.chartFull') }
])

const grain = computed(() => grainMs(props.report?.interval ?? 'hour'))

const reportSpan = computed<TimeSpan | null>(() => {
  if (!props.report) return null
  const start = Date.parse(props.report.start)
  const end = Date.parse(props.report.end)
  return Number.isNaN(start) || Number.isNaN(end) || end <= start ? null : { start, end }
})

const dataSpan = computed(() => recordedSpan(props.report?.history ?? [], grain.value))

// 有记录的时段明显短于所选周期时才提供聚焦，否则两种视图几乎一样。
const canFocus = computed(() => {
  const range = reportSpan.value
  const data = dataSpan.value
  return Boolean(range && data && data.end - data.start < (range.end - range.start) * 0.85)
})

const viewSpan = computed<TimeSpan | null>(() => {
  const range = reportSpan.value
  const data = dataSpan.value
  if (!range) return null
  if (chartView.value === 'focus' && canFocus.value && data) {
    return { start: Math.max(range.start, data.start), end: Math.min(range.end, data.end) }
  }
  return range
})

// 按图表宽度控制柱数（每柱约 7px）和刻度数，窄屏自动合并为更长的时间段。
const maxBins = computed(() => Math.min(120, Math.max(24, Math.floor((chartWidth.value || 640) / 7))))
const maxTicks = computed(() => Math.min(10, Math.max(3, Math.floor((chartWidth.value || 640) / 80))))

const binned = computed(() => {
  const view = viewSpan.value
  const range = reportSpan.value
  if (!view || !range) return { size: grain.value, bins: [] }
  return buildTrafficBins(props.report?.history ?? [], {
    grain: grain.value,
    view,
    range,
    maxBins: maxBins.value
  })
})

const gaps = computed(() => gapSpans(binned.value.bins))

const xBounds = computed(() => {
  const bins = binned.value.bins
  return bins.length ? { min: bins[0].start, max: bins[bins.length - 1].end } : null
})

const xTicks = computed(() =>
  xBounds.value
    ? buildTimeTicks(xBounds.value.min, xBounds.value.max, maxTicks.value, binned.value.size)
    : { values: [], step: binned.value.size }
)

const durationKeys = {
  minute: 'wireguard.binMinutes',
  hour: 'wireguard.binHours',
  day: 'wireguard.binDays'
} as const

const binLabel = computed(() => {
  const { unit, count } = durationParts(binned.value.size)
  return t('wireguard.perBar', { size: t(durationKeys[unit], { count }) })
})

const gapSwatchStyle = {
  backgroundColor: palette.gapFill,
  backgroundImage: `repeating-linear-gradient(135deg, ${palette.gapStroke} 0 1px, transparent 1px 3px)`
}

// 同一根轴的刻度共用一个单位，避免出现 976.6 KiB 与 1.9 MiB 混排；柱为堆叠，按合计取最大值。
const axisUnit = computed(() => {
  const max = binned.value.bins.reduce((value, bin) => Math.max(value, bin.tx + bin.rx), 0)
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB']
  let divisor = 1
  let unit = 0
  while (max / divisor >= 1024 && unit < units.length - 1) {
    divisor *= 1024
    unit++
  }
  return { divisor, label: units[unit] }
})

// Chart.js 接受 null（无记录）点，但官方类型只声明了数值点。
function asBarData(points: BarPoint[]) {
  return points as unknown as { x: number; y: number }[]
}

function barDataset(key: 'tx' | 'rx', label: string, color: string, hover: string) {
  return {
    label,
    data: asBarData(
      binned.value.bins.map((bin) => ({
        x: (bin.start + bin.end) / 2,
        y: bin.recorded ? bin[key] / axisUnit.value.divisor : null
      }))
    ),
    backgroundColor: color,
    hoverBackgroundColor: hover,
    borderRadius: 2,
    barPercentage: 0.86,
    categoryPercentage: 1,
    stack: 'traffic'
  }
}

const chartData = computed<ChartData<'bar', { x: number; y: number }[]>>(() => ({
  datasets: [
    barDataset('tx', t('wireguard.sent'), palette.sent, palette.sentHover),
    barDataset('rx', t('wireguard.received'), palette.received, palette.receivedHover)
  ]
}))

function binAt(index: number | undefined) {
  return index === undefined ? undefined : binned.value.bins[index]
}

const hatchPatterns = new WeakMap<CanvasRenderingContext2D, CanvasPattern | null>()

function hatchPattern(ctx: CanvasRenderingContext2D) {
  if (!hatchPatterns.has(ctx)) {
    const tile = document.createElement('canvas')
    tile.width = 6
    tile.height = 6
    const tileCtx = tile.getContext('2d')
    if (tileCtx) {
      tileCtx.strokeStyle = palette.gapStroke
      tileCtx.lineWidth = 1
      tileCtx.beginPath()
      tileCtx.moveTo(0, 6)
      tileCtx.lineTo(6, 0)
      tileCtx.moveTo(-1, 1)
      tileCtx.lineTo(1, -1)
      tileCtx.moveTo(5, 7)
      tileCtx.lineTo(7, 5)
      tileCtx.stroke()
    }
    hatchPatterns.set(ctx, tileCtx ? ctx.createPattern(tile, 'repeat') : null)
  }
  return hatchPatterns.get(ctx) ?? null
}

// 无记录的时段画斜线阴影，和流量为 0 的柱区分开。
const gapShading: Plugin<'bar'> = {
  id: 'wireguardTrafficGaps',
  beforeDatasetsDraw(chart) {
    const { ctx, chartArea, scales } = chart
    const x = scales.x
    if (!gaps.value.length || !chartArea || !x) return
    const height = chartArea.bottom - chartArea.top
    const pattern = hatchPattern(ctx)
    ctx.save()
    for (const gap of gaps.value) {
      const left = Math.max(chartArea.left, x.getPixelForValue(gap.start))
      const right = Math.min(chartArea.right, x.getPixelForValue(gap.end))
      if (right <= left) continue
      ctx.fillStyle = palette.gapFill
      ctx.fillRect(left, chartArea.top, right - left, height)
      if (pattern) {
        ctx.fillStyle = pattern
        ctx.fillRect(left, chartArea.top, right - left, height)
      }
    }
    ctx.restore()
  }
}

const chartPlugins = [gapShading]

const chartOptions = computed<ChartOptions<'bar'>>(() => ({
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
      footerColor: palette.tooltipBody,
      footerFont: { size: 11, weight: 'normal' },
      footerMarginTop: 6,
      borderColor: palette.tooltipBorder,
      borderWidth: 1,
      padding: 10,
      displayColors: true,
      usePointStyle: true,
      boxWidth: 8,
      boxHeight: 8,
      boxPadding: 4,
      filter: (item: TooltipItem<'bar'>) => item.parsed.y !== null,
      callbacks: {
        title: (items: TooltipItem<'bar'>[]) => {
          const bin = binAt(items[0]?.dataIndex)
          const range = reportSpan.value
          if (!bin || !range) return ''
          return formatBinRange(
            Math.max(bin.start, range.start),
            Math.min(bin.end, range.end),
            binned.value.size
          )
        },
        label: (context: TooltipItem<'bar'>) => {
          const bin = binAt(context.dataIndex)
          const value = bin ? (context.datasetIndex === 0 ? bin.tx : bin.rx) : 0
          return `${context.dataset.label}: ${formatBytes(value)}`
        },
        footer: (items: TooltipItem<'bar'>[]) => {
          const bin = binAt(items[0]?.dataIndex)
          if (!bin) return []
          const lines = [`${t('wireguard.total')}: ${formatBytes(bin.tx + bin.rx)}`]
          if (bin.recorded < bin.expected) {
            lines.push(
              t('wireguard.binCoverage', { recorded: bin.recorded, expected: bin.expected })
            )
          }
          return lines
        }
      }
    }
  },
  scales: {
    x: {
      type: 'linear',
      stacked: true,
      offset: false,
      min: xBounds.value?.min,
      max: xBounds.value?.max,
      grid: { display: false },
      border: { display: false },
      afterBuildTicks: (axis) => {
        axis.ticks = xTicks.value.values.map((value) => ({ value }))
      },
      ticks: {
        color: palette.ticks,
        font: { size: 10 },
        maxRotation: 0,
        autoSkip: true,
        autoSkipPadding: 12,
        callback: (value) => formatTimeTick(Number(value), xTicks.value.step)
      }
    },
    y: {
      stacked: true,
      beginAtZero: true,
      grace: '8%',
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
