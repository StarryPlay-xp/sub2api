<template>
  <section
    class="rounded-3xl bg-white p-6 shadow-sm ring-1 ring-gray-900/5 dark:bg-dark-800 dark:ring-dark-700"
    data-testid="peak-trend"
    :aria-busy="loading"
  >
    <header class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
      <div>
        <h3 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('bandwidth.trend.title') }}</h3>
        <p class="mt-0.5 text-xs text-gray-500 dark:text-dark-400">
          {{ t('bandwidth.trend.subtitle', { size: binSizeText }) }}
        </p>
      </div>
      <div
        class="inline-flex shrink-0 self-start rounded-xl border border-gray-200 bg-gray-50 p-1 dark:border-dark-700 dark:bg-dark-900/60"
        role="group"
        :aria-label="t('bandwidth.trend.viewLabel')"
      >
        <button
          v-for="option in viewOptions"
          :key="option.value"
          type="button"
          class="rounded-lg px-3 py-1.5 text-xs font-medium transition-colors duration-150 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/40"
          :class="
            view === option.value
              ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-700 dark:text-white'
              : 'text-gray-500 hover:text-gray-800 dark:text-dark-400 dark:hover:text-dark-200'
          "
          :aria-pressed="view === option.value"
          @click="view = option.value"
        >
          {{ option.label }}
        </button>
      </div>
    </header>

    <!-- 走势图：峰值折线 + 同一时间轴的流量柱 -->
    <template v-if="view === 'chart'">
      <div class="mt-5 flex flex-wrap items-center gap-x-4 gap-y-1.5 text-xs text-gray-600 dark:text-dark-300">
        <span class="inline-flex items-center gap-1.5">
          <span class="h-0.5 w-4 rounded-full" :style="{ backgroundColor: palette.in }"></span>
          {{ t('bandwidth.trend.inbound') }}
        </span>
        <span class="inline-flex items-center gap-1.5">
          <span class="h-0.5 w-4 rounded-full" :style="{ backgroundColor: palette.out }"></span>
          {{ t('bandwidth.trend.outbound') }}
        </span>
        <span v-if="limit" class="inline-flex items-center gap-1.5">
          <span class="w-4 border-t-2 border-dashed" :style="{ borderColor: palette.limit }"></span>
          {{ t('bandwidth.trend.limit', { value: limitText }) }}
        </span>
        <span v-if="gaps.length" class="inline-flex items-center gap-1.5">
          <span class="h-3 w-3 rounded-sm ring-1 ring-inset ring-gray-200 dark:ring-dark-600" :style="gapSwatchStyle"></span>
          {{ t('bandwidth.trend.noRecord') }}
        </span>
      </div>

      <div ref="chartBox" class="relative mt-3 h-72">
        <Line
          v-if="hasBins"
          :data="lineData"
          :options="lineOptions"
          :plugins="linePlugins"
          role="img"
          :aria-label="t('bandwidth.trend.title')"
        />
        <div
          v-if="overlayText"
          class="absolute inset-0 flex flex-col items-center justify-center gap-1.5 rounded-xl text-center"
          :class="hasBins ? 'pointer-events-none' : 'border border-dashed border-gray-200 bg-gray-50/60 dark:border-dark-700 dark:bg-dark-900/30'"
          data-testid="trend-overlay"
        >
          <div v-if="loading && !report" class="h-full w-full animate-pulse rounded-xl bg-gray-100 dark:bg-dark-700/60"></div>
          <template v-else>
            <p class="text-sm font-medium text-gray-500 dark:text-dark-300">{{ overlayText }}</p>
            <p v-if="overlayHint" class="max-w-sm text-xs text-gray-400 dark:text-dark-500">{{ overlayHint }}</p>
          </template>
        </div>
      </div>

      <div class="mt-6 flex items-center justify-between text-xs">
        <span class="font-medium text-gray-600 dark:text-dark-200">{{ t('bandwidth.trend.traffic') }}</span>
        <span class="text-gray-400 dark:text-dark-500">{{ t('bandwidth.trend.trafficHint', { size: binSizeText }) }}</span>
      </div>
      <div class="relative mt-2 h-32">
        <Bar v-if="hasBins" :data="barData" :options="barOptions" role="img" :aria-label="t('bandwidth.trend.traffic')" />
        <div v-else class="h-full rounded-xl bg-gray-50 dark:bg-dark-900/30" :class="{ 'animate-pulse': loading }"></div>
      </div>
      <p v-if="gaps.length" class="mt-3 text-[11px] text-gray-400 dark:text-dark-500">{{ t('bandwidth.trend.gapsHint') }}</p>
    </template>

    <!-- 峰值排行：走势图的表格视图，直接回答“哪段时间最忙” -->
    <div v-else class="mt-5" data-testid="peak-table">
      <p class="mb-3 text-xs text-gray-500 dark:text-dark-400">
        {{ t('bandwidth.trend.tableHint', { interval: intervalText }) }}
      </p>
      <div class="overflow-x-auto rounded-xl border border-gray-100 dark:border-dark-700">
        <table class="w-full min-w-[680px] text-left text-sm">
          <thead class="bg-gray-50/80 text-xs font-medium text-gray-500 dark:bg-dark-900/40 dark:text-dark-400">
            <tr>
              <th scope="col" class="w-16 whitespace-nowrap px-4 py-2.5 text-right">{{ t('bandwidth.trend.rank') }}</th>
              <th scope="col" class="px-4 py-2.5">{{ t('bandwidth.trend.window') }}</th>
              <th scope="col" class="px-4 py-2.5">{{ t('bandwidth.trend.direction') }}</th>
              <th scope="col" class="px-4 py-2.5 text-right">{{ t('bandwidth.trend.peak') }}</th>
              <th scope="col" class="px-4 py-2.5 text-right">{{ t('bandwidth.trend.peakAt') }}</th>
              <th v-if="limit" scope="col" class="px-4 py-2.5 text-right">{{ t('bandwidth.trend.usage') }}</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-100 tabular-nums dark:divide-dark-700/70">
            <tr v-for="(row, index) in topWindows" :key="row.start" class="text-gray-700 dark:text-dark-200">
              <td class="px-4 py-2.5 text-right text-gray-400 dark:text-dark-500">{{ index + 1 }}</td>
              <td class="px-4 py-2.5">{{ windowText(row.start, row.end) }}</td>
              <td class="px-4 py-2.5">
                <span class="inline-flex items-center gap-1.5">
                  <span class="h-2 w-2 rounded-full" :style="{ backgroundColor: row.direction === 'in' ? palette.in : palette.out }"></span>
                  {{ row.direction === 'in' ? t('bandwidth.direction.in') : t('bandwidth.direction.out') }}
                </span>
              </td>
              <td class="px-4 py-2.5 text-right">
                <span class="font-semibold text-gray-900 dark:text-white">{{ formatMbps(row.peak.bytes_per_second) }}</span>
                <span class="ml-1.5 text-xs text-gray-400 dark:text-dark-500">{{ formatByteRate(row.peak.bytes_per_second) }}</span>
              </td>
              <td class="px-4 py-2.5 text-right text-gray-500 dark:text-dark-400">{{ formatSampleTime(row.peak.at) }}</td>
              <td v-if="limit" class="px-4 py-2.5 text-right">{{ usageText(row.peak.bytes_per_second) }}</td>
            </tr>
            <tr v-if="!topWindows.length">
              <td :colspan="limit ? 6 : 5" class="px-4 py-8 text-center text-sm text-gray-400 dark:text-dark-500">
                {{ report?.has_records ? t('bandwidth.trend.tableEmpty') : t('bandwidth.trend.noRecords') }}
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
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
  type Plugin,
  type TooltipItem
} from 'chart.js'
import { Bar, Line } from 'vue-chartjs'
import type { ServiceTrafficReport } from '@/api/admin/ops'
import type { TrafficPeak } from '@/api/admin/wireguard'
import {
  buildPeakBins,
  hostSamples,
  toMbps,
  topPeakWindows,
  type HostDirection
} from '@/utils/bandwidthSeries'
import { formatByteCount, formatByteRate, formatMbps, formatSampleTime } from '@/utils/trafficFormat'
import {
  buildTimeTicks,
  durationParts,
  formatBinRange,
  formatTimeTick,
  gapSpans,
  grainMs
} from '@/utils/wireguardTrafficChart'
import { useBandwidthPalette } from '../useBandwidthPalette'

ChartJS.register(LinearScale, LineElement, PointElement, BarElement, Tooltip)

type View = 'chart' | 'table'

const props = defineProps<{
  report: ServiceTrafficReport | null
  limit: number | null
  loading: boolean
}>()

const { t } = useI18n()
const palette = useBandwidthPalette()

const view = ref<View>('chart')
const viewOptions = computed<{ value: View; label: string }[]>(() => [
  { value: 'chart', label: t('bandwidth.trend.viewChart') },
  { value: 'table', label: t('bandwidth.trend.viewTable') }
])

const reduceMotion =
  typeof window !== 'undefined' &&
  typeof window.matchMedia === 'function' &&
  window.matchMedia('(prefers-reduced-motion: reduce)').matches
// 只在首次出现时动画，之后每 15 秒刷新不再重放，避免折线抖动。
const animate = ref(!reduceMotion)
let animationTimer: ReturnType<typeof setTimeout> | undefined
onMounted(() => {
  if (animate.value) animationTimer = setTimeout(() => (animate.value = false), 900)
})
onUnmounted(() => {
  if (animationTimer) clearTimeout(animationTimer)
})

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

// 每个点约 5px，窄屏自动合并成更长的时间段。
const maxBins = computed(() => Math.min(180, Math.max(30, Math.floor((width.value || 800) / 5))))
const maxTicks = computed(() => Math.min(10, Math.max(3, Math.floor((width.value || 800) / 90))))

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
const gaps = computed(() => (hasBins.value ? gapSpans(binned.value.bins.map((bin) => ({ ...bin, tx: 0, rx: 0 }))) : []))
const hasPeakPoints = computed(() => binned.value.bins.some((bin) => bin.peaks.in || bin.peaks.out))

const overlayText = computed(() => {
  if (props.loading && !props.report) return ' '
  if (!props.report) return t('bandwidth.trend.unavailable')
  if (!props.report.has_records) return t('bandwidth.trend.noRecords')
  if (!hasPeakPoints.value) {
    return props.report.concurrent_runtimes ? t('bandwidth.trend.concurrent') : t('bandwidth.trend.noPeaks')
  }
  return ''
})

const overlayHint = computed(() =>
  props.report && !props.report.has_records ? t('bandwidth.trend.noRecordsHint') : ''
)

const durationKeys = {
  minute: 'bandwidth.trend.minutes',
  hour: 'bandwidth.trend.hours',
  day: 'bandwidth.trend.days'
} as const

const binSizeText = computed(() => {
  const { unit, count } = durationParts(binned.value.size)
  return t(durationKeys[unit], { count })
})

const intervalText = computed(() =>
  props.report?.interval === 'minute' ? t('bandwidth.trend.perMinute') : t('bandwidth.trend.perHour')
)

const limitText = computed(() => (props.limit ? formatLimit(props.limit) : ''))

function formatLimit(value: number) {
  const digits = value < 10 ? 2 : value < 100 ? 1 : 0
  return `${parseFloat(value.toFixed(digits))} Mbps`
}

function usageText(bytesPerSecond: number) {
  if (!props.limit) return ''
  const percent = (toMbps(bytesPerSecond) / props.limit) * 100
  return `${percent < 10 ? parseFloat(percent.toFixed(1)) : Math.round(percent)}%`
}

const topWindows = computed(() => topPeakWindows(samples.value, grain.value, 10))

function windowText(start: number, end: number) {
  return formatBinRange(start, end, grain.value)
}

const xBounds = computed(() => {
  const bins = binned.value.bins
  return bins.length ? { min: bins[0].start, max: bins[bins.length - 1].end } : null
})

const xTicks = computed(() =>
  xBounds.value
    ? buildTimeTicks(xBounds.value.min, xBounds.value.max, maxTicks.value, binned.value.size)
    : { values: [], step: binned.value.size }
)

type Point = { x: number; y: number | null }
// Chart.js 接受 null（无记录）点以断开折线，但官方类型只声明了数值点。
const asData = (points: Point[]) => points as unknown as { x: number; y: number }[]

function peakSeries(key: HostDirection): Point[] {
  return binned.value.bins.map((bin) => {
    const peak = bin.peaks[key]
    return { x: (bin.start + bin.end) / 2, y: bin.recorded && peak ? toMbps(peak.bytes_per_second) : null }
  })
}

const lineData = computed<ChartData<'line', { x: number; y: number }[]>>(() => ({
  datasets: (['in', 'out'] as const).map((key) => ({
    label: key === 'in' ? t('bandwidth.trend.inbound') : t('bandwidth.trend.outbound'),
    data: asData(peakSeries(key)),
    borderColor: key === 'in' ? palette.value.in : palette.value.out,
    backgroundColor: key === 'in' ? palette.value.in : palette.value.out,
    borderWidth: 2,
    borderCapStyle: 'round' as const,
    borderJoinStyle: 'round' as const,
    tension: 0.25,
    spanGaps: false,
    pointRadius: 0,
    pointHoverRadius: 4,
    pointHoverBorderWidth: 2,
    pointHoverBorderColor: palette.value.dark ? '#1e293b' : '#ffffff'
  }))
}))

const yMax = computed(() => {
  const values = binned.value.bins.flatMap((bin) =>
    (['in', 'out'] as const).map((key) => (bin.peaks[key] ? toMbps(bin.peaks[key]!.bytes_per_second) : 0))
  )
  const dataMax = values.length ? Math.max(...values) : 0
  return Math.max(dataMax * 1.08, (props.limit ?? 0) * 1.05) || 1
})

function binAt(index: number | undefined) {
  return index === undefined ? undefined : binned.value.bins[index]
}

function peakLine(label: string, peak: TrafficPeak | undefined) {
  if (!peak) return t('bandwidth.trend.tooltipUnknown', { label })
  return t('bandwidth.trend.tooltipPeak', {
    label,
    mbps: formatMbps(peak.bytes_per_second),
    rate: formatByteRate(peak.bytes_per_second),
    time: formatSampleTime(peak.at)
  })
}

// 无记录的时段画浅色阴影，与“峰值为 0”区分。
const gapShading: Plugin<'line'> = {
  id: 'bandwidthTrendGaps',
  beforeDatasetsDraw(chart) {
    const { ctx, chartArea, scales } = chart
    const x = scales.x
    if (!gaps.value.length || !chartArea || !x) return
    ctx.save()
    ctx.fillStyle = palette.value.gapFill
    for (const gap of gaps.value) {
      const left = Math.max(chartArea.left, x.getPixelForValue(gap.start))
      const right = Math.min(chartArea.right, x.getPixelForValue(gap.end))
      if (right > left) ctx.fillRect(left, chartArea.top, right - left, chartArea.bottom - chartArea.top)
    }
    ctx.restore()
  }
}

// 带宽上限参考线：虚线表示阈值，与实线网格区分。
const limitLine: Plugin<'line'> = {
  id: 'bandwidthLimitLine',
  afterDatasetsDraw(chart) {
    const limit = props.limit
    const { ctx, chartArea, scales } = chart
    if (!limit || !chartArea || !scales.y) return
    const y = scales.y.getPixelForValue(limit)
    if (y < chartArea.top || y > chartArea.bottom) return
    ctx.save()
    ctx.strokeStyle = palette.value.limit
    ctx.lineWidth = 1.5
    ctx.setLineDash([6, 4])
    ctx.beginPath()
    ctx.moveTo(chartArea.left, y)
    ctx.lineTo(chartArea.right, y)
    ctx.stroke()
    ctx.setLineDash([])
    ctx.fillStyle = palette.value.ticks
    ctx.font = '11px system-ui, -apple-system, sans-serif'
    ctx.textAlign = 'right'
    ctx.textBaseline = 'bottom'
    ctx.fillText(t('bandwidth.trend.limit', { value: limitText.value }), chartArea.right - 4, y - 4)
    ctx.restore()
  }
}

// 十字准线：跟随指针吸附到最近的时间点。
const crosshair: Plugin<'line'> = {
  id: 'bandwidthCrosshair',
  afterDatasetsDraw(chart) {
    const active = chart.tooltip?.getActiveElements?.() ?? []
    const { ctx, chartArea } = chart
    if (!active.length || !chartArea) return
    const x = active[0].element.x
    ctx.save()
    ctx.strokeStyle = palette.value.ticks
    ctx.globalAlpha = 0.5
    ctx.lineWidth = 1
    ctx.beginPath()
    ctx.moveTo(x, chartArea.top)
    ctx.lineTo(x, chartArea.bottom)
    ctx.stroke()
    ctx.restore()
  }
}

const linePlugins = [gapShading, limitLine, crosshair]

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

const lineOptions = computed<ChartOptions<'line'>>(() => ({
  responsive: true,
  maintainAspectRatio: false,
  animation: animate.value ? { duration: 700, easing: 'easeOutQuart' } : false,
  interaction: { intersect: false, mode: 'index' },
  plugins: {
    legend: { display: false },
    tooltip: {
      ...tooltipBase(),
      filter: (item: TooltipItem<'line'>) => item.parsed.y !== null,
      callbacks: {
        title: (items: TooltipItem<'line'>[]) => {
          const bin = binAt(items[0]?.dataIndex)
          const span = range.value
          if (!bin || !span) return ''
          return formatBinRange(Math.max(bin.start, span.start), Math.min(bin.end, span.end), binned.value.size)
        },
        label: (context: TooltipItem<'line'>) => {
          const bin = binAt(context.dataIndex)
          const key = context.datasetIndex === 0 ? 'in' : 'out'
          return peakLine(String(context.dataset.label), bin?.peaks[key])
        },
        footer: (items: TooltipItem<'line'>[]) => {
          const bin = binAt(items[0]?.dataIndex)
          if (!bin) return []
          const lines: string[] = []
          const busiest = Math.max(bin.peaks.in?.bytes_per_second ?? 0, bin.peaks.out?.bytes_per_second ?? 0)
          if (props.limit && busiest > 0) lines.push(t('bandwidth.trend.tooltipUsage', { percent: usageText(busiest) }))
          if (bin.recorded < bin.expected) {
            lines.push(t('bandwidth.trend.coverage', { recorded: bin.recorded, expected: bin.expected }))
          }
          return lines
        }
      }
    }
  },
  scales: {
    x: xScale(),
    y: {
      beginAtZero: true,
      // 用建议上限而不是硬上限，让刻度落在整数上；同时保证带宽上限参考线在图内。
      suggestedMax: yMax.value,
      grid: { color: palette.value.grid, drawTicks: false },
      border: { display: false },
      ticks: {
        color: palette.value.ticks,
        font: { size: 11 },
        maxTicksLimit: 5,
        padding: 8,
        callback: (value: string | number) => {
          const number = Number(value)
          if (number === 0) return '0'
          const digits = number < 1 ? 2 : number < 10 ? 1 : 0
          return `${parseFloat(number.toFixed(digits))} Mbps`
        }
      }
    }
  }
}))

// 流量柱：同一时间轴、同一分箱，纵轴共用一个字节单位。
const byteUnit = computed(() => {
  const max = binned.value.bins.reduce((value, bin) => Math.max(value, bin.bytes.in + bin.bytes.out), 0)
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB']
  let divisor = 1
  let unit = 0
  while (max / divisor >= 1024 && unit < units.length - 1) {
    divisor *= 1024
    unit++
  }
  return { divisor, label: units[unit] }
})

const barData = computed<ChartData<'bar', { x: number; y: number }[]>>(() => ({
  datasets: (['in', 'out'] as const).map((key, index) => ({
    label: key === 'in' ? t('bandwidth.trend.inboundBytes') : t('bandwidth.trend.outboundBytes'),
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
    maxBarThickness: 24,
    stack: 'traffic'
  }))
}))

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
        maxTicksLimit: 3,
        padding: 8,
        callback: (value: string | number) => {
          const number = Number(value)
          return number === 0 ? '0' : `${parseFloat(number.toFixed(1))} ${byteUnit.value.label}`
        }
      }
    }
  }
}))

const gapSwatchStyle = computed(() => ({ backgroundColor: palette.value.gapFill }))
</script>
