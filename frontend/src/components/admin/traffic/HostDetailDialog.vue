<template>
  <BaseDialog :show="true" :title="t('traffic.host.detail.title')" width="extra-wide" @close="emit('close')">
    <div class="space-y-6" data-testid="host-detail-dialog">
      <!-- 套餐与账期摘要 -->
      <dl v-if="plan && period" class="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <div v-for="tile in tiles" :key="tile.key" class="rounded-2xl bg-gray-50 px-4 py-3 dark:bg-dark-900/40" :data-testid="`host-detail-${tile.key}`">
          <dt class="text-xs font-medium text-gray-500 dark:text-dark-400">{{ tile.label }}</dt>
          <dd class="mt-1 text-base font-semibold tabular-nums text-gray-900 dark:text-white">{{ tile.value }}</dd>
          <dd v-if="tile.sub" class="mt-0.5 text-xs text-gray-500 dark:text-dark-400">{{ tile.sub }}</dd>
        </div>
      </dl>

      <p
        v-if="error"
        role="alert"
        class="rounded-2xl bg-red-50 px-4 py-3 text-sm text-red-700 ring-1 ring-red-200/70 dark:bg-red-950/30 dark:text-red-300 dark:ring-red-900/40"
      >
        {{ error }}
      </p>

      <!-- 每日用量 -->
      <section>
        <div class="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
          <h4 class="text-sm font-medium text-gray-700 dark:text-dark-100">{{ plan ? t('traffic.host.detail.days') : t('traffic.host.detail.daysRecent') }}</h4>
          <span v-if="detail" class="flex flex-wrap items-center gap-x-3 text-[11px] text-gray-400 dark:text-dark-500">
            <span class="inline-flex items-center gap-1.5"><span class="h-2 w-2 rounded-sm" :style="{ backgroundColor: palette.in }"></span>{{ t('traffic.host.detail.columns.rx') }}</span>
            <span class="inline-flex items-center gap-1.5"><span class="h-2 w-2 rounded-sm" :style="{ backgroundColor: palette.out }"></span>{{ t('traffic.host.detail.columns.tx') }}</span>
            {{ t('traffic.host.detail.daysHint', { timezone: detail.days_timezone }) }}
          </span>
        </div>
        <div class="relative mt-3 h-56" data-testid="host-detail-chart">
          <div v-if="loading && !detail" class="h-full animate-pulse rounded-xl bg-gray-100 dark:bg-dark-700/50"></div>
          <Bar v-else-if="detail && detail.days.length" :data="chartData" :options="chartOptions" role="img" :aria-label="t('traffic.host.detail.days')" />
          <div v-else class="flex h-full items-center justify-center rounded-xl border border-dashed border-gray-200 text-sm text-gray-500 dark:border-dark-700 dark:text-dark-400">
            {{ t('traffic.host.usage.noRecords') }}
          </div>
        </div>
      </section>

      <div class="grid gap-6 lg:grid-cols-2">
        <!-- 历史账期 -->
        <section>
          <h4 class="text-sm font-medium text-gray-700 dark:text-dark-100">{{ t('traffic.host.detail.periods') }}</h4>
          <div v-if="detail && detail.periods.length" class="mt-3 overflow-x-auto">
            <table class="w-full text-left text-xs" data-testid="host-detail-periods">
              <thead class="text-gray-400 dark:text-dark-500">
                <tr>
                  <th class="py-1.5 pr-3 font-medium">{{ t('traffic.host.detail.columns.period') }}</th>
                  <th class="py-1.5 pr-3 text-right font-medium">{{ t('traffic.host.detail.columns.billed') }}</th>
                  <th class="py-1.5 pr-3 text-right font-medium">{{ t('traffic.host.detail.columns.usage') }}</th>
                  <th class="py-1.5 font-medium">{{ t('traffic.host.detail.columns.coverage') }}</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-100 tabular-nums text-gray-700 dark:divide-dark-700 dark:text-dark-200">
                <tr v-for="row in periodRows" :key="row.key">
                  <td class="py-2 pr-3">{{ row.range }}</td>
                  <td class="py-2 pr-3 text-right font-medium">{{ row.billed }}</td>
                  <td class="py-2 pr-3 text-right" :class="row.over ? 'text-red-600 dark:text-red-400' : ''">{{ row.percent }}</td>
                  <td class="py-2"><span class="badge" :class="row.badgeClass">{{ row.bound }}</span></td>
                </tr>
              </tbody>
            </table>
          </div>
          <p v-else class="mt-3 text-xs text-gray-400 dark:text-dark-500">{{ t('traffic.host.detail.periodsEmpty') }}</p>
        </section>

        <!-- 对账与补齐记录 -->
        <section>
          <h4 class="text-sm font-medium text-gray-700 dark:text-dark-100">{{ t('traffic.host.detail.calibrations') }}</h4>
          <div v-if="detail && detail.calibrations.length" class="mt-3 overflow-x-auto">
            <table class="w-full text-left text-xs" data-testid="host-detail-calibrations">
              <thead class="text-gray-400 dark:text-dark-500">
                <tr>
                  <th class="py-1.5 pr-3 font-medium">{{ t('traffic.host.detail.columns.at') }}</th>
                  <th class="py-1.5 pr-3 font-medium">{{ t('traffic.host.detail.columns.kind') }}</th>
                  <th class="py-1.5 pr-3 text-right font-medium">{{ t('traffic.host.detail.columns.provider') }}</th>
                  <th class="py-1.5 pr-3 text-right font-medium">{{ t('traffic.host.detail.columns.measured') }}</th>
                  <th class="py-1.5 text-right font-medium">{{ t('traffic.host.detail.columns.deviation') }}</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-100 tabular-nums text-gray-700 dark:divide-dark-700 dark:text-dark-200">
                <tr v-for="row in calibrationRows" :key="row.id">
                  <td class="py-2 pr-3">{{ row.at }}</td>
                  <td class="py-2 pr-3">{{ row.kind }}</td>
                  <td class="py-2 pr-3 text-right">{{ row.provider }}</td>
                  <td class="py-2 pr-3 text-right">{{ row.measured }}</td>
                  <td class="py-2 text-right">{{ row.deviation }}</td>
                </tr>
              </tbody>
            </table>
          </div>
          <p v-else class="mt-3 text-xs text-gray-400 dark:text-dark-500">{{ t('traffic.host.detail.calibrationsEmpty') }}</p>
        </section>
      </div>

      <div class="grid gap-6 lg:grid-cols-2">
        <!-- 网卡 -->
        <section>
          <h4 class="text-sm font-medium text-gray-700 dark:text-dark-100">{{ t('traffic.host.detail.interfaces') }}</h4>
          <table class="mt-3 w-full text-left text-xs" data-testid="host-detail-interfaces">
            <thead class="text-gray-400 dark:text-dark-500">
              <tr>
                <th class="py-1.5 pr-3 font-medium">{{ t('traffic.host.detail.columns.name') }}</th>
                <th class="py-1.5 pr-3 font-medium">{{ t('traffic.host.detail.columns.route') }}</th>
                <th class="py-1.5 pr-3 font-medium">{{ t('traffic.host.detail.columns.rate') }}</th>
                <th class="py-1.5 font-medium">{{ t('traffic.host.detail.columns.selected') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 tabular-nums text-gray-700 dark:divide-dark-700 dark:text-dark-200">
              <tr v-for="item in status.interfaces" :key="item.name">
                <td class="py-2 pr-3 font-medium">{{ item.name }}</td>
                <td class="py-2 pr-3">{{ routeText(item) }}</td>
                <td class="py-2 pr-3">{{ rateText(item) }}</td>
                <td class="py-2">
                  <Icon v-if="item.selected" name="checkCircle" size="sm" class="h-4 w-4 text-primary-500" />
                  <span v-else class="text-gray-400">—</span>
                </td>
              </tr>
            </tbody>
          </table>
        </section>

        <!-- 采集信息 -->
        <section>
          <h4 class="text-sm font-medium text-gray-700 dark:text-dark-100">{{ t('traffic.host.detail.collection') }}</h4>
          <dl class="mt-3 grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-2 text-xs" data-testid="host-detail-collection">
            <dt class="text-gray-400 dark:text-dark-500">{{ t('traffic.host.detail.source') }}</dt>
            <dd class="text-gray-700 dark:text-dark-200">{{ sourceText }}</dd>
            <dt class="text-gray-400 dark:text-dark-500">{{ t('traffic.host.detail.startedAt') }}</dt>
            <dd class="tabular-nums text-gray-700 dark:text-dark-200">{{ formatSampleTime(status.collection.started_at) }}</dd>
            <dt class="text-gray-400 dark:text-dark-500">{{ t('traffic.host.detail.hostBoot') }}</dt>
            <dd class="tabular-nums text-gray-700 dark:text-dark-200">{{ formatSampleTime(status.collection.host_boot_at) }}</dd>
            <dt class="text-gray-400 dark:text-dark-500">{{ t('traffic.host.detail.lastSample') }}</dt>
            <dd class="tabular-nums text-gray-700 dark:text-dark-200">{{ formatSampleTime(status.collection.last_sample_at) }}</dd>
          </dl>
          <p class="mt-3 text-xs leading-relaxed text-gray-400 dark:text-dark-500">{{ t('traffic.quality.hostNicBasis') }}</p>
        </section>
      </div>
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  BarElement,
  CategoryScale,
  Chart as ChartJS,
  LinearScale,
  Tooltip,
  type ChartData,
  type ChartOptions,
  type TooltipItem
} from 'chart.js'
import { Bar } from 'vue-chartjs'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import type { HostInterface, HostTrafficStatus, HostUnitBase } from '@/api/admin/hostTraffic'
import { formatMbps, formatSampleTime } from '@/utils/trafficFormat'
import { useBandwidthPalette } from '@/views/admin/bandwidth/useBandwidthPalette'
import {
  billingModeLabel,
  boundPrefix,
  formatDeviation,
  formatDuration,
  formatLocalShort,
  formatPeriodRange,
  formatQuotaBytes,
  formatRatioPercent,
  parseByteString,
  resetRuleText
} from './hostFormat'
import { useHostTrafficDetail } from './useHostTraffic'

ChartJS.register(CategoryScale, LinearScale, BarElement, Tooltip)

const props = defineProps<{ status: HostTrafficStatus }>()
const emit = defineEmits<{ close: [] }>()

const { t } = useI18n()
const palette = useBandwidthPalette()
const { detail, loading, error } = useHostTrafficDetail()

const plan = computed(() => props.status.plan)
const period = computed(() => props.status.period)
const base = computed<HostUnitBase>(() => plan.value?.unit_base ?? 1000)
const timeZone = computed(() => plan.value?.timezone ?? detail.value?.days_timezone ?? 'UTC')

const tiles = computed(() => {
  if (!plan.value || !period.value) return []
  const prefix = boundPrefix(period.value.bound, t)
  return [
    { key: 'period', label: t('traffic.host.detail.columns.period'), value: formatPeriodRange(period.value, timeZone.value), sub: resetRuleText(plan.value, t) },
    {
      key: 'used',
      label: t('traffic.host.usedWithMode', { mode: billingModeLabel(plan.value.billing_mode, t) }),
      value: `${prefix ? `${prefix} ` : ''}${formatQuotaBytes(period.value.billed_bytes, base.value)}`,
      sub: `${t('traffic.host.ofQuota', { quota: formatQuotaBytes(period.value.quota_bytes, base.value) })} · ${formatRatioPercent(period.value.used_ratio)}`
    },
    { key: 'rx', label: t('traffic.host.detail.columns.rx'), value: formatQuotaBytes(period.value.rx_bytes, base.value), sub: t('traffic.host.legend.measured') },
    { key: 'tx', label: t('traffic.host.detail.columns.tx'), value: formatQuotaBytes(period.value.tx_bytes, base.value), sub: t('traffic.host.legend.measured') }
  ]
})

const BOUND_BADGE = { exact: 'badge-success', estimate: 'badge-primary', lower: 'badge-warning' } as const

const periodRows = computed(() =>
  (detail.value?.periods ?? []).map((row) => {
    const billed = parseByteString(row.billed_bytes)
    const quota = parseByteString(row.quota_bytes)
    const ratio = billed !== null && quota !== null && quota > 0n ? Number((billed * 100000n) / quota) / 100000 : null
    const prefix = boundPrefix(row.bound, t)
    return {
      key: row.start,
      range: formatPeriodRange(row, timeZone.value),
      billed: `${prefix ? `${prefix} ` : ''}${formatQuotaBytes(row.billed_bytes, base.value)}`,
      percent: formatRatioPercent(ratio),
      over: ratio !== null && ratio > 1,
      bound: t(`traffic.host.detail.bound.${row.bound}`),
      badgeClass: BOUND_BADGE[row.bound]
    }
  })
)

const calibrationRows = computed(() =>
  (detail.value?.calibrations ?? []).map((row) => ({
    id: row.id,
    at: formatLocalShort(row.at),
    kind: t(`traffic.host.detail.kind.${row.kind}`),
    provider: formatQuotaBytes(row.provider_bytes, base.value),
    measured: formatQuotaBytes(row.measured_bytes, base.value),
    deviation: row.deviation_ratio === null ? '—' : formatDeviation(row.deviation_ratio)
  }))
)

const sourceText = computed(() => {
  const source = props.status.collection.source
  if (source === 'host_mount') return t('traffic.host.detail.sourceHostMount')
  if (source === 'native') return t('traffic.host.detail.sourceNative')
  return '—'
})

function routeText(item: HostInterface) {
  if (!item.default_route) return t('traffic.host.detail.routeUnknown')
  const families = [item.default_route.ipv4 ? 'IPv4' : '', item.default_route.ipv6 ? 'IPv6' : ''].filter(Boolean)
  return families.length ? families.join(' / ') : t('traffic.host.detail.no')
}

function rateText(item: HostInterface) {
  if (item.rx_bytes_per_second === null || item.tx_bytes_per_second === null) return '—'
  return t('traffic.host.interfaceRate', { rx: formatMbps(item.rx_bytes_per_second), tx: formatMbps(item.tx_bytes_per_second) })
}

// 图表单位跟随套餐进制，与套餐卡的数字一致。
const chartUnit = computed(() => {
  const max = (detail.value?.days ?? []).reduce((value, day) => {
    const rx = Number(day.rx_bytes) || 0
    const tx = Number(day.tx_bytes) || 0
    return Math.max(value, rx + tx)
  }, 0)
  const units = base.value === 1024 ? ['B', 'KiB', 'MiB', 'GiB', 'TiB'] : ['B', 'KB', 'MB', 'GB', 'TB']
  let divisor = 1
  let unit = 0
  while (max / divisor >= base.value && unit < units.length - 1) {
    divisor *= base.value
    unit++
  }
  return { divisor, label: units[unit] }
})

const chartData = computed<ChartData<'bar'>>(() => {
  const days = detail.value?.days ?? []
  return {
    labels: days.map((day) => day.date.slice(5)),
    datasets: (['rx', 'tx'] as const).map((key, index) => ({
      label: key === 'rx' ? t('traffic.host.detail.columns.rx') : t('traffic.host.detail.columns.tx'),
      data: days.map((day) => (Number(key === 'rx' ? day.rx_bytes : day.tx_bytes) || 0) / chartUnit.value.divisor),
      backgroundColor: key === 'rx' ? palette.value.in : palette.value.out,
      hoverBackgroundColor: key === 'rx' ? palette.value.inHover : palette.value.outHover,
      borderColor: palette.value.dark ? '#1e293b' : '#ffffff',
      borderWidth: index === 1 ? { top: 0, right: 0, bottom: 2, left: 0 } : 0,
      borderRadius: index === 1 ? { topLeft: 3, topRight: 3, bottomLeft: 0, bottomRight: 0 } : 0,
      borderSkipped: false,
      barPercentage: 0.78,
      categoryPercentage: 0.9,
      maxBarThickness: 28,
      stack: 'host'
    }))
  }
})

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
      callbacks: {
        title: (items: TooltipItem<'bar'>[]) => detail.value?.days[items[0]?.dataIndex ?? 0]?.date ?? '',
        label: (context: TooltipItem<'bar'>) => {
          const day = detail.value?.days[context.dataIndex]
          const value = day ? (context.datasetIndex === 0 ? day.rx_bytes : day.tx_bytes) : null
          return `${context.dataset.label}: ${formatQuotaBytes(value, base.value)}`
        },
        footer: (items: TooltipItem<'bar'>[]) => {
          const day = detail.value?.days[items[0]?.dataIndex ?? 0]
          if (!day) return []
          const lines = [`${t('traffic.host.detail.columns.billed')}: ${formatQuotaBytes(day.billed_bytes, base.value)}`]
          if (day.observed_seconds < day.expected_seconds) {
            lines.push(t('traffic.host.detail.dayPartial', { duration: formatDuration(day.expected_seconds - day.observed_seconds, t) }))
          }
          return lines
        }
      }
    }
  },
  scales: {
    x: {
      stacked: true,
      grid: { display: false },
      border: { display: false },
      ticks: { color: palette.value.ticks, font: { size: 11 }, maxRotation: 0, autoSkip: true, autoSkipPadding: 12 }
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
          return number === 0 ? '0' : `${parseFloat(number.toFixed(1))} ${chartUnit.value.label}`
        }
      }
    }
  }
}))
</script>
