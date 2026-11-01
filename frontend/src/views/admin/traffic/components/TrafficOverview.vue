<template>
  <section
    class="rounded-3xl bg-white p-6 shadow-sm ring-1 ring-gray-900/5 dark:bg-dark-800 dark:ring-dark-700"
    data-testid="traffic-overview"
    :aria-busy="loading"
  >
    <header>
      <h3 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('traffic.overview.title') }}</h3>
      <p class="mt-0.5 text-xs text-gray-500 dark:text-dark-400">{{ t('traffic.overview.subtitle') }}</p>
    </header>

    <div v-if="initialLoading" class="mt-5 space-y-4" data-testid="traffic-skeleton">
      <div class="h-24 animate-pulse rounded-2xl bg-gray-100 dark:bg-dark-700/50"></div>
      <div class="h-32 animate-pulse rounded-2xl bg-gray-100 dark:bg-dark-700/50"></div>
    </div>

    <template v-else>
      <!-- 累计、日均与平均带宽：后两项只按实际观察时长折算 -->
      <dl class="mt-5 grid gap-6 border-b border-gray-100 pb-6 dark:border-dark-700 sm:grid-cols-3">
        <div data-testid="traffic-total">
          <dt class="text-xs font-medium text-gray-500 dark:text-dark-400">{{ t('traffic.overview.total') }}</dt>
          <dd class="mt-2 text-3xl font-semibold tracking-tight text-gray-900 dark:text-white">{{ totalText }}</dd>
          <dd class="mt-1.5 text-xs text-gray-500 tabular-nums dark:text-dark-400">
            {{ report?.has_records ? t('traffic.overview.inOut', { inbound: inboundText, outbound: outboundText }) : '—' }}
          </dd>
          <div
            v-if="inOutRatio !== null"
            class="mt-3 flex h-1.5 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700"
            role="img"
            :aria-label="t('traffic.overview.inOut', { inbound: inboundText, outbound: outboundText })"
          >
            <span class="h-full" :style="{ width: `${inOutRatio}%`, backgroundColor: palette.in }"></span>
            <span class="h-full grow" :style="{ backgroundColor: palette.out }"></span>
          </div>
        </div>
        <div data-testid="traffic-daily">
          <dt class="text-xs font-medium text-gray-500 dark:text-dark-400">{{ t('traffic.overview.daily') }}</dt>
          <dd class="mt-2 text-3xl font-semibold tracking-tight text-gray-900 dark:text-white">{{ dailyText }}</dd>
          <dd v-if="observedText" class="mt-1.5 text-xs text-gray-400 dark:text-dark-500">{{ observedText }}</dd>
        </div>
        <div data-testid="traffic-average">
          <dt class="text-xs font-medium text-gray-500 dark:text-dark-400">{{ t('traffic.overview.average') }}</dt>
          <dd class="mt-2 text-3xl font-semibold tracking-tight text-gray-900 dark:text-white">{{ averageText }}</dd>
          <dd class="mt-1.5 text-xs text-gray-400 dark:text-dark-500">{{ t('traffic.overview.averageHint') }}</dd>
        </div>
      </dl>

      <!-- 四方向构成：同一内容经过两段路径时会各记一次 -->
      <div class="mt-6" data-testid="traffic-breakdown">
        <div class="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
          <h4 class="text-sm font-medium text-gray-700 dark:text-dark-100">{{ t('traffic.overview.breakdown') }}</h4>
          <span class="text-[11px] text-gray-400 dark:text-dark-500">{{ t('traffic.overview.breakdownHint') }}</span>
        </div>
        <ul class="mt-3 grid gap-x-8 gap-y-3 md:grid-cols-2">
          <li v-for="item in directionItems" :key="item.key" :data-testid="`traffic-${item.key}`">
            <div class="flex items-baseline justify-between gap-3 text-xs">
              <span class="inline-flex items-center gap-1.5 text-gray-600 dark:text-dark-300">
                <span class="h-0.5 w-3 rounded-full" :style="{ backgroundColor: item.color }"></span>
                {{ item.label }}
              </span>
              <span class="whitespace-nowrap tabular-nums text-gray-500 dark:text-dark-400">
                <span class="font-medium text-gray-700 dark:text-dark-200">{{ item.bytesText }}</span>
                <span v-if="item.percentText" class="ml-1.5">{{ item.percentText }}</span>
              </span>
            </div>
            <div class="mt-1.5 h-1.5 rounded-full bg-gray-100 dark:bg-dark-700">
              <span
                class="block h-full rounded-full"
                :style="{ width: item.barWidth, backgroundColor: item.color }"
              ></span>
            </div>
          </li>
        </ul>
      </div>

      <!-- 流量趋势：24 小时按小时，长范围按天，分箱随宽度自适应 -->
      <div class="mt-6 border-t border-gray-100 pt-5 dark:border-dark-700">
        <div class="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
          <h4 class="text-sm font-medium text-gray-700 dark:text-dark-100">{{ t('traffic.overview.trend') }}</h4>
          <span class="text-[11px] text-gray-400 dark:text-dark-500">{{ trendHint }}</span>
        </div>
        <div ref="chartBox" class="relative mt-3 h-52" data-testid="traffic-chart">
          <Bar
            v-if="hasBins"
            :data="barData"
            :options="barOptions"
            role="img"
            :aria-label="t('traffic.overview.trend')"
          />
          <div
            v-else
            class="flex h-full flex-col items-center justify-center gap-1.5 rounded-xl border border-dashed border-gray-200 px-4 text-center dark:border-dark-700"
            data-testid="traffic-chart-empty"
          >
            <p class="text-sm font-medium text-gray-500 dark:text-dark-300">
              {{ report ? t('traffic.overview.noRecords') : t('traffic.overview.unavailable') }}
            </p>
            <p v-if="report && !report.has_records" class="text-xs text-gray-400 dark:text-dark-500">
              {{ t('traffic.overview.noRecordsHint') }}
            </p>
          </div>
        </div>
      </div>
    </template>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useElementSize } from '@vueuse/core'
import {
  BarElement,
  Chart as ChartJS,
  LinearScale,
  LineElement,
  PointElement,
  Tooltip,
  type ChartData,
  type ChartOptions,
  type TooltipItem
} from 'chart.js'
import { Bar } from 'vue-chartjs'
import type { ServiceTrafficDirection, ServiceTrafficReport } from '@/api/admin/ops'
import {
  buildPeakBins,
  hostSamples,
  trafficDirectionShares,
  trafficSummary
} from '@/utils/bandwidthSeries'
import { formatByteCount, formatMbps } from '@/utils/trafficFormat'
import {
  buildTimeTicks,
  DAY_MS,
  durationParts,
  formatBinRange,
  formatTimeTick,
  grainMs
} from '@/utils/wireguardTrafficChart'
import { useBandwidthPalette } from '@/views/admin/bandwidth/useBandwidthPalette'

ChartJS.register(LinearScale, LineElement, PointElement, BarElement, Tooltip)

const props = defineProps<{
  report: ServiceTrafficReport | null
  loading: boolean
}>()

const { t } = useI18n()
const palette = useBandwidthPalette()

const initialLoading = computed(() => props.loading && !props.report)
const summary = computed(() => trafficSummary(props.report))

function bytesText(value: bigint | null) {
  return value === null ? '—' : formatByteCount(value)
}

const totalText = computed(() => bytesText(summary.value.total))
const inboundText = computed(() => bytesText(summary.value.inbound))
const outboundText = computed(() => bytesText(summary.value.outbound))
const dailyText = computed(() => bytesText(summary.value.dailyBytes))
const averageText = computed(() =>
  summary.value.averageBytesPerSecond === null ? '—' : formatMbps(summary.value.averageBytesPerSecond)
)

const inOutRatio = computed(() => {
  const { inbound, total } = summary.value
  if (inbound === null || total === null || total <= 0n) return null
  return Number((inbound * 1000n) / total) / 10
})

function durationText(seconds: number) {
  if (seconds < 3600) {
    return t('traffic.overview.observedMinutes', { minutes: Math.max(1, Math.round(seconds / 60)) })
  }
  if (seconds < 2 * 86_400) {
    return t('traffic.overview.observedHours', { hours: parseFloat((seconds / 3600).toFixed(1)) })
  }
  let days = Math.floor(seconds / 86_400)
  let hours = Math.round((seconds % 86_400) / 3600)
  if (hours === 24) {
    days += 1
    hours = 0
  }
  return hours === 0
    ? t('traffic.overview.observedDaysOnly', { days })
    : t('traffic.overview.observedDays', { days, hours })
}

// 分母是实际观察时长，不是所选范围；记录不完整时数值偏小也不会被伪装成整段平均值。
const observedText = computed(() =>
  summary.value.observedSeconds === null
    ? ''
    : t('traffic.overview.observed', { duration: durationText(summary.value.observedSeconds) })
)

function formatPercent(value: number) {
  return String(parseFloat(value.toFixed(1)))
}

const directionLabels = computed<Record<ServiceTrafficDirection, string>>(() => ({
  client_in: t('traffic.overview.clientIn'),
  client_out: t('traffic.overview.clientOut'),
  upstream_out: t('traffic.overview.upstreamOut'),
  upstream_in: t('traffic.overview.upstreamIn')
}))

const directionItems = computed(() =>
  trafficDirectionShares(props.report).map((share) => ({
    key: share.key,
    label: directionLabels.value[share.key],
    color: share.key === 'client_in' || share.key === 'upstream_in' ? palette.value.in : palette.value.out,
    bytesText: bytesText(share.bytes),
    percentText: share.percent === null ? '' : `${formatPercent(share.percent)}%`,
    barWidth:
      share.bytes !== null && share.bytes > 0n && share.percent !== null
        ? `${Math.max(share.percent, 0.5)}%`
        : '0%'
  }))
)

const chartBox = ref<HTMLElement | null>(null)
const { width } = useElementSize(chartBox)

const grain = computed(() => grainMs(props.report?.interval ?? 'hour'))
const samples = computed(() => hostSamples(props.report?.history ?? []))

const range = computed(() => {
  if (!props.report) return null
  const start = Date.parse(props.report.start)
  const end = Date.parse(props.report.end)
  return Number.isNaN(start) || Number.isNaN(end) || end <= start ? null : { start, end }
})

// 24 小时按小时，7/30/90 天按天；不随宽度合并，保证每柱就是一个小时或一天。
const maxBins = computed(() => {
  const report = props.report
  if (!report || !range.value) return 24
  if (report.interval === 'minute') return 24
  return Math.max(7, Math.ceil((range.value.end - range.value.start) / DAY_MS))
})

const binned = computed(() =>
  range.value
    ? buildPeakBins(samples.value, ['in', 'out'] as const, {
        grain: grain.value,
        range: range.value,
        maxBins: maxBins.value
      })
    : { size: grain.value, bins: [] }
)

const hasBins = computed(() => Boolean(props.report?.has_records && binned.value.bins.length))

const trendHint = computed(() => {
  const { unit, count } = durationParts(binned.value.size)
  const keys = {
    minute: 'bandwidth.trend.minutes',
    hour: 'bandwidth.trend.hours',
    day: 'bandwidth.trend.days'
  } as const
  return t('traffic.overview.trendHint', { size: t(keys[unit], { count }) })
})

const reduceMotion =
  typeof window !== 'undefined' &&
  typeof window.matchMedia === 'function' &&
  window.matchMedia('(prefers-reduced-motion: reduce)').matches
// 只在首次出现时动画，之后每 15 秒刷新不再重放，避免柱子抖动。
const animate = ref(!reduceMotion)
let animationTimer: ReturnType<typeof setTimeout> | undefined
onMounted(() => {
  if (animate.value) animationTimer = setTimeout(() => (animate.value = false), 900)
})
onUnmounted(() => {
  if (animationTimer) clearTimeout(animationTimer)
})

type Point = { x: number; y: number | null }
// Chart.js 接受 null（无记录）点以留空柱子，但官方类型只声明了数值点。
const asData = (points: Point[]) => points as unknown as { x: number; y: number }[]

const byteUnit = computed(() => {
  const max = binned.value.bins.reduce(
    (value, bin) => Math.max(value, bin.bytes.in + bin.bytes.out),
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

const xBounds = computed(() => {
  const bins = binned.value.bins
  return bins.length ? { min: bins[0].start, max: bins[bins.length - 1].end } : null
})

const maxTicks = computed(() => Math.min(8, Math.max(3, Math.floor((width.value || 800) / 90))))

const xTicks = computed(() =>
  xBounds.value
    ? buildTimeTicks(xBounds.value.min, xBounds.value.max, maxTicks.value, binned.value.size)
    : { values: [], step: binned.value.size }
)

function xScale() {
  return {
    type: 'linear' as const,
    min: xBounds.value?.min,
    max: xBounds.value?.max,
    grid: { display: false },
    border: { display: false },
    afterBuildTicks: (axis: { ticks: { value: number }[] }) => {
      axis.ticks = xTicks.value.values.map((value) => ({ value }))
    },
    ticks: {
      color: palette.value.ticks,
      font: { size: 11 },
      maxRotation: 0,
      autoSkip: true,
      autoSkipPadding: 16,
      callback: (value: string | number) => formatTimeTick(Number(value), xTicks.value.step)
    }
  }
}

const barData = computed<ChartData<'bar', { x: number; y: number }[]>>(() => ({
  datasets: (['in', 'out'] as const).map((key, index) => ({
    label: key === 'in' ? t('traffic.overview.inbound') : t('traffic.overview.outbound'),
    data: asData(
      binned.value.bins.map((bin) => ({
        x: (bin.start + bin.end) / 2,
        y: bin.recorded ? bin.bytes[key] / byteUnit.value.divisor : null
      }))
    ),
    backgroundColor: key === 'in' ? palette.value.in : palette.value.out,
    hoverBackgroundColor: key === 'in' ? palette.value.inHover : palette.value.outHover,
    // 上层段底部留 2px 表面色缝隙，顶部圆角只给最上层。
    borderColor: palette.value.dark ? '#1e293b' : '#ffffff',
    borderWidth: index === 1 ? { top: 0, right: 0, bottom: 2, left: 0 } : 0,
    borderRadius: index === 1 ? { topLeft: 3, topRight: 3, bottomLeft: 0, bottomRight: 0 } : 0,
    borderSkipped: false,
    barPercentage: 0.82,
    categoryPercentage: 1,
    maxBarThickness: 28,
    stack: 'traffic'
  }))
}))

function binAt(index: number | undefined) {
  return index === undefined ? undefined : binned.value.bins[index]
}

function tooltipBase() {
  return {
    backgroundColor: palette.value.tooltipBg,
    titleColor: palette.value.tooltipTitle,
    bodyColor: palette.value.tooltipBody,
    footerColor: palette.value.tooltipBody,
    footerFont: { size: 11, weight: 'normal' as const },
    borderColor: palette.value.tooltipBorder,
    borderWidth: 1,
    padding: 10,
    boxWidth: 12,
    boxHeight: 2,
    boxPadding: 6
  }
}

const barOptions = computed<ChartOptions<'bar'>>(() => ({
  responsive: true,
  maintainAspectRatio: false,
  animation: animate.value ? { duration: 700, easing: 'easeOutQuart' } : false,
  interaction: { intersect: false, mode: 'index' },
  plugins: {
    legend: { display: false },
    tooltip: {
      ...tooltipBase(),
      filter: (item: TooltipItem<'bar'>) => item.parsed.y !== null,
      callbacks: {
        title: (items: TooltipItem<'bar'>[]) => {
          const bin = binAt(items[0]?.dataIndex)
          const span = range.value
          if (!bin || !span) return ''
          return formatBinRange(Math.max(bin.start, span.start), Math.min(bin.end, span.end), binned.value.size)
        },
        label: (context: TooltipItem<'bar'>) => {
          const bin = binAt(context.dataIndex)
          const key = context.datasetIndex === 0 ? 'in' : 'out'
          return `${context.dataset.label}: ${formatByteCount(bin ? Math.round(bin.bytes[key]) : 0)}`
        },
        footer: (items: TooltipItem<'bar'>[]) => {
          const bin = binAt(items[0]?.dataIndex)
          if (!bin) return []
          const lines: string[] = []
          if (bin.recorded > 0) {
            lines.push(
              t('traffic.overview.tooltipTotal', {
                total: formatByteCount(Math.round(bin.bytes.in + bin.bytes.out))
              })
            )
          }
          if (bin.recorded < bin.expected) {
            lines.push(t('bandwidth.trend.coverage', { recorded: bin.recorded, expected: bin.expected }))
          }
          return lines
        }
      }
    }
  },
  scales: {
    x: { ...xScale(), stacked: true, offset: false },
    y: {
      stacked: true,
      beginAtZero: true,
      grace: '8%',
      grid: { color: palette.value.grid, drawTicks: false },
      border: { display: false },
      ticks: {
        color: palette.value.ticks,
        font: { size: 11 },
        maxTicksLimit: 4,
        padding: 8,
        callback: (value: string | number) => {
          const number = Number(value)
          return number === 0 ? '0' : `${parseFloat(number.toFixed(1))} ${byteUnit.value.label}`
        }
      }
    }
  }
}))
</script>
