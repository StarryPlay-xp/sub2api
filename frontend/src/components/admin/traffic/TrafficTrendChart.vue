<template>
  <div>
    <div class="flex flex-wrap items-center justify-between gap-x-4 gap-y-1">
      <div class="flex flex-wrap items-center gap-x-4 gap-y-1 text-[11px] text-gray-500 dark:text-dark-400">
        <span class="inline-flex items-center gap-1.5">
          <span class="h-2 w-2 rounded-sm" :style="{ backgroundColor: palette.in }"></span>{{ inLabel }}
        </span>
        <span class="inline-flex items-center gap-1.5">
          <span class="h-2 w-2 rounded-sm" :style="{ backgroundColor: palette.out }"></span>{{ outLabel }}
        </span>
        <span class="inline-flex items-center gap-1.5">
          <span class="h-0.5 w-3 rounded-full" :style="{ backgroundColor: palette.limit }"></span>{{ peakLabel }}
        </span>
      </div>
      <span class="text-[11px] text-gray-400 dark:text-dark-500">{{ hint }}</span>
    </div>
    <div ref="chartBox" class="relative mt-3 h-56" :data-testid="testId">
      <Bar v-if="hasBins" :data="chartData" :options="chartOptions" role="img" :aria-label="t('traffic.detail.trend')" />
      <div
        v-else
        class="flex h-full items-center justify-center rounded-xl border border-dashed border-gray-200 text-sm text-gray-500 dark:border-dark-700 dark:text-dark-300"
        :data-testid="`${testId}-empty`"
      >
        {{ t('traffic.detail.noHistory') }}
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useElementSize } from '@vueuse/core'
import {
  BarController,
  BarElement,
  Chart as ChartJS,
  LinearScale,
  LineController,
  LineElement,
  PointElement,
  Tooltip,
  type ChartData,
  type ChartOptions,
  type TooltipItem
} from 'chart.js'
import { Bar } from 'vue-chartjs'
import { buildPeakBins, type PeakSample } from '@/utils/bandwidthSeries'
import { formatByteCount, formatMbps, formatSampleTime } from '@/utils/trafficFormat'
import {
  buildTimeTicks,
  DAY_MS,
  durationParts,
  formatBinRange,
  formatTimeTick,
  type TimeSpan
} from '@/utils/wireguardTrafficChart'
import { useBandwidthPalette } from '@/views/admin/bandwidth/useBandwidthPalette'
import type { TrendKey } from './trendSamples'

ChartJS.register(BarController, LineController, LinearScale, BarElement, LineElement, PointElement, Tooltip)

// 通用的“流量柱 + 峰值曲线”：入站、出站字节堆叠，合计峰值画在右轴。
// 用户 / Key 与 WG 节点共用；调用方负责把各自的桶换算成 in/out/total。
const props = withDefaults(
  defineProps<{
    samples: PeakSample<TrendKey>[]
    span: TimeSpan | null
    // 原始桶粒度（毫秒），用于判断柱内有几个桶有记录。
    grain: number
    inLabel: string
    outLabel: string
    peakLabel: string
    // 柱宽说明的文案键，参数 size 为每柱时长。
    hintKey?: string
    testId?: string
  }>(),
  { hintKey: 'traffic.detail.trendHint', testId: 'subject-trend' }
)

const { t } = useI18n()
const palette = useBandwidthPalette()
const chartBox = ref<HTMLElement | null>(null)
const { width } = useElementSize(chartBox)

const samples = computed(() => props.samples)
const span = computed(() => props.span)

// 24 小时按小时；7 天约 6 小时一柱；更长按天。
const maxBins = computed(() => {
  if (!span.value) return 24
  const days = (span.value.end - span.value.start) / DAY_MS
  if (days <= 1.05) return 24
  if (days <= 7.05) return 28
  return Math.ceil(days)
})

const binned = computed(() =>
  span.value
    ? buildPeakBins(samples.value, ['in', 'out', 'total'] as const, { grain: props.grain, range: span.value, maxBins: maxBins.value })
    : { size: props.grain, bins: [] }
)

const hasBins = computed(() => samples.value.length > 0 && binned.value.bins.length > 0)

const hint = computed(() => {
  const { unit, count } = durationParts(binned.value.size)
  const keys = { minute: 'bandwidth.trend.minutes', hour: 'bandwidth.trend.hours', day: 'bandwidth.trend.days' } as const
  return t(props.hintKey, { size: t(keys[unit], { count }) })
})

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

type Point = { x: number; y: number | null }
// Chart.js 接受 null 点以留空，但官方类型只声明了数值点。
const asData = (points: Point[]) => points as unknown as { x: number; y: number }[]

const xBounds = computed(() => {
  const bins = binned.value.bins
  return bins.length ? { min: bins[0].start, max: bins[bins.length - 1].end } : null
})

const maxTicks = computed(() => Math.min(8, Math.max(3, Math.floor((width.value || 720) / 90))))
const xTicks = computed(() =>
  xBounds.value
    ? buildTimeTicks(xBounds.value.min, xBounds.value.max, maxTicks.value, binned.value.size)
    : { values: [], step: binned.value.size }
)

const chartData = computed<ChartData<'bar', { x: number; y: number }[]>>(() => {
  const bins = binned.value.bins
  const bars = (['in', 'out'] as const).map((key, index) => ({
    type: 'bar' as const,
    label: key === 'in' ? props.inLabel : props.outLabel,
    data: asData(bins.map((bin) => ({ x: (bin.start + bin.end) / 2, y: bin.recorded ? bin.bytes[key] / byteUnit.value.divisor : null }))),
    backgroundColor: key === 'in' ? palette.value.in : palette.value.out,
    hoverBackgroundColor: key === 'in' ? palette.value.inHover : palette.value.outHover,
    borderColor: palette.value.dark ? '#1e293b' : '#ffffff',
    borderWidth: index === 1 ? { top: 0, right: 0, bottom: 2, left: 0 } : 0,
    borderRadius: index === 1 ? { topLeft: 3, topRight: 3, bottomLeft: 0, bottomRight: 0 } : 0,
    borderSkipped: false,
    barPercentage: 0.82,
    categoryPercentage: 1,
    maxBarThickness: 26,
    stack: 'traffic',
    yAxisID: 'y',
    order: 2
  }))
  const line = {
    type: 'line' as const,
    label: props.peakLabel,
    data: asData(
      bins.map((bin) => ({
        x: (bin.start + bin.end) / 2,
        y: bin.peaks.total ? (bin.peaks.total.bytes_per_second * 8) / 1_000_000 : null
      }))
    ),
    borderColor: palette.value.limit,
    backgroundColor: palette.value.limit,
    borderWidth: 1.5,
    pointRadius: 0,
    pointHoverRadius: 3,
    tension: 0.25,
    spanGaps: false,
    yAxisID: 'peak',
    order: 1
  }
  return { datasets: [...bars, line] as unknown as ChartData<'bar', { x: number; y: number }[]>['datasets'] }
})

function binAt(index: number | undefined) {
  return index === undefined ? undefined : binned.value.bins[index]
}

const chartOptions = computed<ChartOptions<'bar'>>(() => ({
  responsive: true,
  maintainAspectRatio: false,
  animation: false,
  interaction: { intersect: false, mode: 'index' },
  plugins: {
    legend: { display: false },
    tooltip: {
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
      boxPadding: 6,
      filter: (item: TooltipItem<'bar'>) => item.parsed.y !== null,
      callbacks: {
        title: (items: TooltipItem<'bar'>[]) => {
          const bin = binAt(items[0]?.dataIndex)
          if (!bin || !span.value) return ''
          return formatBinRange(Math.max(bin.start, span.value.start), Math.min(bin.end, span.value.end), binned.value.size)
        },
        label: (context: TooltipItem<'bar'>) => {
          const bin = binAt(context.dataIndex)
          if (!bin) return ''
          if (context.datasetIndex === 2) {
            const peak = bin.peaks.total
            return peak
              ? t('traffic.detail.tooltipPeak', { label: props.peakLabel, value: formatMbps(peak.bytes_per_second), time: formatSampleTime(peak.at) })
              : ''
          }
          const key = context.datasetIndex === 0 ? 'in' : 'out'
          return `${context.dataset.label}: ${formatByteCount(Math.round(bin.bytes[key]))}`
        },
        footer: (items: TooltipItem<'bar'>[]) => {
          const bin = binAt(items[0]?.dataIndex)
          if (!bin) return []
          const lines: string[] = []
          if (bin.recorded > 0) lines.push(t('traffic.detail.tooltipTotal', { total: formatByteCount(Math.round(bin.bytes.total)) }))
          if (bin.recorded < bin.expected) {
            lines.push(t('bandwidth.trend.coverage', { recorded: bin.recorded, expected: bin.expected }))
          }
          return lines
        }
      }
    }
  },
  scales: {
    x: {
      type: 'linear',
      min: xBounds.value?.min,
      max: xBounds.value?.max,
      stacked: true,
      offset: false,
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
    },
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
    },
    peak: {
      position: 'right',
      beginAtZero: true,
      grace: '8%',
      grid: { display: false },
      border: { display: false },
      ticks: {
        color: palette.value.ticks,
        font: { size: 11 },
        maxTicksLimit: 4,
        padding: 8,
        callback: (value: string | number) => {
          const number = Number(value)
          return number === 0 ? '0' : `${parseFloat(number.toFixed(number < 10 ? 1 : 0))} Mbps`
        }
      }
    }
  }
}))
</script>
