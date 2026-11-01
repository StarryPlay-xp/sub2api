<template>
  <div class="overflow-x-auto rounded-2xl border border-gray-200/80 bg-white dark:border-dark-700 dark:bg-dark-800/60" data-testid="node-table">
    <table class="w-full min-w-max text-sm">
      <thead>
        <tr class="border-b border-gray-100 bg-gray-50/80 text-xs text-gray-500 dark:border-dark-700 dark:bg-dark-900/40 dark:text-dark-400">
          <th scope="col" class="py-2.5 pl-5 pr-3 text-left font-medium">{{ t('bandwidth.nodes.column') }}</th>
          <th
            v-for="column in columns"
            :key="column.key"
            scope="col"
            class="whitespace-nowrap px-3 py-2.5 font-medium last:pr-5"
            :class="column.key === 'trend' ? 'text-left' : 'text-right'"
            :aria-sort="sortKey === column.key ? (descending ? 'descending' : 'ascending') : undefined"
          >
            <button
              v-if="column.sortable"
              type="button"
              class="inline-flex items-center gap-1 rounded transition-colors hover:text-gray-900 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/40 dark:hover:text-white"
              :class="sortKey === column.key ? 'text-primary-600 dark:text-primary-400' : ''"
              :data-testid="`node-sort-${column.key}`"
              @click="setSort(column.key)"
            >
              {{ column.label }}
              <Icon v-if="sortKey === column.key" :name="descending ? 'arrowDown' : 'arrowUp'" size="xs" class="h-3 w-3" />
            </button>
            <span v-else>{{ column.label }}</span>
            <HelpTooltip v-if="column.hint" class="ml-1 align-middle" :content="column.hint" width-class="w-72" />
          </th>
        </tr>
      </thead>
      <tbody class="divide-y divide-gray-100 dark:divide-dark-700/70">
        <tr
          v-for="row in rows"
          :key="row.id"
          class="cursor-pointer transition-colors hover:bg-gray-50 dark:hover:bg-dark-700/40"
          :data-testid="`wg-node-${row.id}`"
          @click="emit('open', row.id)"
        >
          <td class="max-w-[16rem] py-3 pl-5 pr-3 align-top">
            <button
              type="button"
              class="block w-full rounded text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/40"
              :aria-label="t('bandwidth.nodes.openDetail', { name: row.name })"
              @click.stop="emit('open', row.id)"
            >
              <span class="block truncate font-medium text-gray-900 dark:text-white" :title="row.name">{{ row.name }}</span>
              <span class="mt-0.5 inline-flex items-center gap-1.5 text-xs font-medium" :class="row.status.textClass" data-testid="node-state">
                <span class="h-1.5 w-1.5 rounded-full" :class="row.status.dotClass"></span>
                {{ row.status.label }}
              </span>
              <span v-if="row.note" class="mt-0.5 block truncate text-[11px]" :class="row.error ? 'text-red-600 dark:text-red-400' : 'text-amber-600 dark:text-amber-400'" :title="row.note">
                {{ row.note }}
              </span>
            </button>
          </td>
          <td class="whitespace-nowrap px-3 py-3 text-right align-top tabular-nums" data-testid="node-peak">
            <template v-if="row.peakTotal">
              <span class="font-semibold text-gray-900 dark:text-white">{{ formatMbps(row.peakTotal.bytes_per_second) }}</span>
              <span class="mt-0.5 block text-xs text-gray-400 dark:text-dark-500">{{ formatSampleTime(row.peakTotal.at) }}</span>
            </template>
            <span v-else class="text-gray-400 dark:text-dark-500">{{ row.peakNote }}</span>
          </td>
          <td class="whitespace-nowrap px-3 py-3 text-right align-top tabular-nums text-gray-700 dark:text-dark-200" data-testid="node-peak-tx">
            {{ row.peakTx ? formatMbps(row.peakTx.bytes_per_second) : '—' }}
          </td>
          <td class="whitespace-nowrap px-3 py-3 text-right align-top tabular-nums text-gray-700 dark:text-dark-200" data-testid="node-peak-rx">
            {{ row.peakRx ? formatMbps(row.peakRx.bytes_per_second) : '—' }}
          </td>
          <td class="whitespace-nowrap px-3 py-3 text-right align-top tabular-nums" data-testid="node-current">
            <span :class="row.current ? 'text-gray-700 dark:text-dark-200' : 'text-gray-400 dark:text-dark-500'">{{ row.currentText }}</span>
          </td>
          <td class="w-36 px-3 py-3 align-middle">
            <BandwidthSparkline
              v-if="row.spark.some((value) => value !== null)"
              :series="[{ key: 'total', color: palette.out, values: row.spark, area: true }]"
              :height="28"
              :label="t('bandwidth.nodes.trend')"
            />
            <span v-else class="text-xs text-gray-400 dark:text-dark-500">—</span>
          </td>
          <td class="whitespace-nowrap px-3 py-3 text-right align-top pr-5" data-testid="node-usage">
            <span v-if="row.usage !== null" class="inline-flex items-center gap-2">
              <span class="h-1.5 w-14 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
                <span class="block h-full rounded-full" :class="usageFill(row.usage)" :style="{ width: `${Math.min(row.usage, 1) * 100}%` }"></span>
              </span>
              <span class="w-11 text-right font-medium tabular-nums" :class="usageText(row.usage)">{{ formatShare(row.usage) }}</span>
            </span>
            <span v-else-if="!row.limit" class="text-xs font-medium text-primary-600 dark:text-primary-400">{{ t('bandwidth.limit.set') }}</span>
            <span v-else class="text-gray-400 dark:text-dark-500">—</span>
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import HelpTooltip from '@/components/common/HelpTooltip.vue'
import Icon from '@/components/icons/Icon.vue'
import { buildPeakBins, headroomLevel, nodeSamples } from '@/utils/bandwidthSeries'
import { formatMbps, formatSampleTime } from '@/utils/trafficFormat'
import { grainMs } from '@/utils/wireguardTrafficChart'
import { formatShare } from '@/components/admin/traffic/subjectFormat'
import {
  nodeCurrentRate,
  nodePeak,
  nodePeakNote,
  nodeRateNote,
  nodeStatus,
  nodeUsage,
  nodeWarning
} from '@/components/admin/traffic/nodeFormat'
import BandwidthSparkline from './BandwidthSparkline.vue'
import { useBandwidthPalette } from '../useBandwidthPalette'
import type { WireGuardNodeState } from '../useWireGuardNodes'

type SortKey = 'peak' | 'peakTx' | 'peakRx' | 'current' | 'usage'

const props = defineProps<{
  nodes: WireGuardNodeState[]
  limitOf: (id: number) => number | null
}>()

const emit = defineEmits<{ open: [id: number] }>()

const { t } = useI18n()
const palette = useBandwidthPalette()

// 节点数量少，在前端排序；默认按同一秒合计峰值从高到低。
const sortKey = ref<SortKey>('peak')
const descending = ref(true)

const columns = computed(() => [
  { key: 'peak', label: t('bandwidth.nodes.peakColumn'), sortable: true, hint: '' },
  { key: 'peakTx', label: t('bandwidth.nodes.toNodePeak'), sortable: true, hint: '' },
  { key: 'peakRx', label: t('bandwidth.nodes.fromNodePeak'), sortable: true, hint: '' },
  { key: 'current', label: t('bandwidth.nodes.current'), sortable: true, hint: '' },
  { key: 'trend', label: t('bandwidth.nodes.trendColumn'), sortable: false, hint: '' },
  { key: 'usage', label: t('bandwidth.nodes.usageColumn'), sortable: true, hint: t('bandwidth.nodes.usageHint') }
])

function setSort(key: string) {
  const next = key as SortKey
  if (sortKey.value === next) descending.value = !descending.value
  else {
    sortKey.value = next
    descending.value = true
  }
}

function spark(node: WireGuardNodeState) {
  const report = node.report
  if (!report?.has_records) return []
  const start = Date.parse(report.start)
  const end = Date.parse(report.end)
  if (Number.isNaN(start) || Number.isNaN(end) || end <= start) return []
  const { bins } = buildPeakBins(nodeSamples(report.history), ['total'] as const, {
    grain: grainMs(report.interval),
    range: { start, end },
    maxBins: 48
  })
  return bins.map((bin) => (bin.recorded && bin.peaks.total ? bin.peaks.total.bytes_per_second : null))
}

const baseRows = computed(() =>
  props.nodes.map((node) => {
    const limit = props.limitOf(node.proxy.id)
    const current = nodeCurrentRate(node.details)
    return {
      id: node.proxy.id,
      name: node.proxy.name,
      status: nodeStatus(node.details, t),
      error: Boolean(node.error),
      note: node.error || nodeWarning(node.report, t),
      peakTotal: nodePeak(node.report, 'total'),
      peakTx: nodePeak(node.report, 'tx'),
      peakRx: nodePeak(node.report, 'rx'),
      peakNote: node.report ? nodePeakNote(node.report, t) || '—' : '—',
      current,
      currentText: current ? formatMbps(current.total) : nodeRateNote(node.details, t) || '—',
      spark: spark(node),
      limit,
      usage: nodeUsage(node.report, limit)
    }
  })
)

type Row = (typeof baseRows.value)[number]

const SORT_VALUE: Record<SortKey, (row: Row) => number | null> = {
  peak: (row) => row.peakTotal?.bytes_per_second ?? null,
  peakTx: (row) => row.peakTx?.bytes_per_second ?? null,
  peakRx: (row) => row.peakRx?.bytes_per_second ?? null,
  current: (row) => row.current?.total ?? null,
  usage: (row) => row.usage
}

// 未知值始终排在最后，同值按节点编号保持稳定。
const rows = computed(() =>
  [...baseRows.value].sort((a, b) => {
    const left = SORT_VALUE[sortKey.value](a)
    const right = SORT_VALUE[sortKey.value](b)
    if (left === null && right === null) return a.id - b.id
    if (left === null) return 1
    if (right === null) return -1
    if (left === right) return a.id - b.id
    return descending.value ? right - left : left - right
  })
)

function usageFill(ratio: number) {
  const level = headroomLevel(ratio)
  return level === 'critical' ? 'bg-red-500' : level === 'warning' ? 'bg-amber-500' : 'bg-primary-500'
}

function usageText(ratio: number) {
  const level = headroomLevel(ratio)
  return level === 'critical'
    ? 'text-red-600 dark:text-red-400'
    : level === 'warning'
      ? 'text-amber-600 dark:text-amber-400'
      : 'text-gray-700 dark:text-dark-200'
}
</script>
