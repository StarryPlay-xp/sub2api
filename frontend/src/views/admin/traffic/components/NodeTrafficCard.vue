<template>
  <section
    class="rounded-3xl bg-white p-6 shadow-sm ring-1 ring-gray-900/5 dark:bg-dark-800 dark:ring-dark-700"
    data-testid="node-traffic"
    :aria-busy="loading"
  >
    <header class="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
      <div>
        <h3 class="flex items-center gap-2 text-base font-semibold text-gray-900 dark:text-white">
          {{ t('traffic.nodes.title') }}
          <span v-if="nodes.length" class="badge badge-gray">{{ t('bandwidth.nodes.count', { count: nodes.length }) }}</span>
        </h3>
        <p class="mt-0.5 text-xs text-gray-500 dark:text-dark-400">{{ t('traffic.nodes.subtitle') }}</p>
      </div>
      <dl v-if="summary" class="text-right text-xs text-gray-500 dark:text-dark-400" data-testid="node-traffic-summary">
        <dt>{{ t('traffic.nodes.total') }}</dt>
        <dd class="mt-0.5 text-lg font-semibold tabular-nums text-gray-900 dark:text-white">{{ summary.total }}</dd>
        <dd class="tabular-nums">{{ t('bandwidth.nodeDetail.split', { toNode: summary.tx, fromNode: summary.rx }) }}</dd>
      </dl>
    </header>

    <p
      v-if="error"
      role="alert"
      class="mt-4 rounded-2xl bg-red-50 px-4 py-3 text-sm text-red-700 dark:bg-red-950/30 dark:text-red-300"
    >
      {{ error }}
    </p>
    <div v-else-if="!loaded" class="mt-5 h-24 animate-pulse rounded-2xl bg-gray-100 dark:bg-dark-700/50"></div>
    <div
      v-else-if="!nodes.length"
      class="mt-5 rounded-2xl border border-dashed border-gray-200 px-4 py-8 text-center text-sm text-gray-500 dark:border-dark-700 dark:text-dark-300"
      data-testid="node-traffic-empty"
    >
      {{ t('bandwidth.nodes.empty') }}
    </div>

    <ul v-else class="mt-5 space-y-1">
      <li v-for="row in rows" :key="row.id">
        <button
          type="button"
          class="grid w-full grid-cols-[minmax(0,1.2fr)_minmax(0,2fr)_6.5rem] items-center gap-x-4 rounded-xl px-2 py-2.5 text-left transition-colors hover:bg-gray-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/40 dark:hover:bg-dark-700/40"
          :data-testid="`node-traffic-${row.id}`"
          @click="emit('open', row.id)"
        >
          <span class="min-w-0">
            <span class="block truncate text-sm font-medium text-gray-900 dark:text-white" :title="row.name">{{ row.name }}</span>
            <span class="mt-0.5 inline-flex items-center gap-1.5 text-xs" :class="row.status.textClass">
              <span class="h-1.5 w-1.5 rounded-full" :class="row.status.dotClass"></span>
              {{ row.status.label }}
            </span>
            <span v-if="row.note" class="mt-0.5 block truncate text-[11px]" :class="row.error ? 'text-red-600 dark:text-red-400' : 'text-amber-600 dark:text-amber-400'">
              {{ row.note }}
            </span>
          </span>
          <!-- 发往节点、从节点接收按全部节点中的最大合计缩放，便于横向比较 -->
          <span class="min-w-0">
            <span class="flex h-2 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700" aria-hidden="true">
              <span class="h-full" :style="{ width: row.txWidth, backgroundColor: palette.out }"></span>
              <span class="h-full" :style="{ width: row.rxWidth, backgroundColor: palette.in }"></span>
            </span>
            <span class="mt-1 block truncate text-[11px] tabular-nums text-gray-500 dark:text-dark-400" :title="row.serviceText ? `${row.splitText} · ${row.serviceText}` : row.splitText">
              {{ row.splitText }}<template v-if="row.serviceText"> · {{ row.serviceText }}</template>
            </span>
          </span>
          <span class="text-right tabular-nums">
            <span class="block text-sm font-semibold text-gray-900 dark:text-white">{{ row.totalText }}</span>
            <span class="block text-[11px] text-gray-500 dark:text-dark-400">{{ row.shareText }}</span>
          </span>
        </button>
      </li>
    </ul>

    <p class="mt-4 border-t border-gray-100 pt-4 text-xs leading-relaxed text-gray-500 dark:border-dark-700 dark:text-dark-400">
      {{ t('traffic.nodes.note') }}
    </p>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { formatByteCount } from '@/utils/trafficFormat'
import { useBandwidthPalette } from '@/views/admin/bandwidth/useBandwidthPalette'
import type { WireGuardNodeState } from '@/views/admin/bandwidth/useWireGuardNodes'
import type { EgressRouteStats } from '@/api/admin/trafficEgress'
import { nodeStatus, nodeWarning } from '@/components/admin/traffic/nodeFormat'
import { formatShare } from '@/components/admin/traffic/subjectFormat'

const props = defineProps<{
  nodes: WireGuardNodeState[]
  loaded: boolean
  loading: boolean
  error: string
  // 出口统计中的 WG 出口，用于标注经各节点的服务载荷；后端未提供时为 null。
  egressRoutes?: EgressRouteStats[] | null
}>()

const emit = defineEmits<{ open: [id: number] }>()

const { t } = useI18n()
const palette = useBandwidthPalette()

function parse(value: string | undefined): bigint | null {
  return typeof value === 'string' && /^\d+$/.test(value) ? BigInt(value) : null
}

const measured = computed(() =>
  props.nodes.map((node) => {
    const report = node.report?.has_records ? node.report : null
    return { node, tx: parse(report?.tx_bytes), rx: parse(report?.rx_bytes), total: parse(report?.total_bytes) }
  })
)

const allTotal = computed(() => measured.value.reduce((sum, item) => sum + (item.total ?? 0n), 0n))

const summary = computed(() => {
  const items = measured.value.filter((item) => item.total !== null)
  if (!items.length) return null
  const sum = (key: 'tx' | 'rx') => items.reduce((total, item) => total + (item[key] ?? 0n), 0n)
  return { total: formatByteCount(allTotal.value), tx: formatByteCount(sum('tx')), rx: formatByteCount(sum('rx')) }
})

const serviceByNode = computed(() => {
  const map = new Map<number, string>()
  for (const route of props.egressRoutes ?? []) {
    if (route.route_type === 'wireguard' && route.route_id !== null) map.set(route.route_id, route.total_bytes)
  }
  return map
})

// 按节点合计流量从高到低；没有记录的节点排在最后。
const rows = computed(() => {
  const max = measured.value.reduce((value, item) => (item.total !== null && item.total > value ? item.total : value), 0n)
  const width = (value: bigint | null) => (value !== null && max > 0n ? `${Number((value * 10000n) / max) / 100}%` : '0%')
  return [...measured.value]
    .sort((a, b) => {
      if (a.total === null && b.total === null) return a.node.proxy.id - b.node.proxy.id
      if (a.total === null) return 1
      if (b.total === null) return -1
      return a.total === b.total ? a.node.proxy.id - b.node.proxy.id : a.total > b.total ? -1 : 1
    })
    .map(({ node, tx, rx, total }) => ({
      id: node.proxy.id,
      name: node.proxy.name,
      status: nodeStatus(node.details, t),
      error: Boolean(node.error),
      note: node.error || nodeWarning(node.report, t),
      totalText: total === null ? '—' : formatByteCount(total),
      shareText: total !== null && allTotal.value > 0n ? formatShare(Number(total) / Number(allTotal.value)) : '',
      splitText:
        tx === null || rx === null
          ? node.report && !node.report.has_records
            ? t('bandwidth.nodes.noRecords')
            : '—'
          : t('bandwidth.nodeDetail.split', { toNode: formatByteCount(tx), fromNode: formatByteCount(rx) }),
      serviceText: serviceByNode.value.has(node.proxy.id)
        ? t('traffic.nodes.service', { total: formatByteCount(serviceByNode.value.get(node.proxy.id)) })
        : '',
      txWidth: width(tx),
      rxWidth: width(rx)
    }))
})
</script>
