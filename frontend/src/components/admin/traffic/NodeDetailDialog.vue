<template>
  <BaseDialog :show="true" :title="node.proxy.name" width="extra-wide" @close="emit('close')">
    <div class="space-y-6" data-testid="node-detail">
      <!-- 节点信息与管理入口 -->
      <div class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div class="flex min-w-0 items-start gap-3">
          <span
            class="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-primary-50 text-primary-600 dark:bg-primary-900/25 dark:text-primary-300"
          >
            <Icon name="server" size="md" />
          </span>
          <div class="min-w-0">
            <p class="text-[11px] font-medium uppercase tracking-wide text-gray-400 dark:text-dark-500">
              {{ t('bandwidth.nodeDetail.kind') }}
            </p>
            <p class="truncate text-base font-medium text-gray-900 dark:text-white">{{ node.proxy.name }}</p>
            <span class="mt-0.5 inline-flex items-center gap-1.5 text-xs font-medium" :class="status.textClass" data-testid="node-detail-status">
              <span class="h-1.5 w-1.5 rounded-full" :class="status.dotClass"></span>
              {{ status.label }}
              <span v-if="accountCount !== null" class="font-normal text-gray-500 dark:text-dark-400">
                · {{ t('bandwidth.nodeDetail.accounts', { count: accountCount }) }}
              </span>
            </span>
          </div>
        </div>
        <button type="button" class="btn btn-secondary btn-sm" data-testid="node-detail-manage" @click="emit('manage', node.proxy.id)">
          <Icon name="cog" size="xs" class="h-3.5 w-3.5" />
          {{ t('bandwidth.nodeDetail.manage') }}
        </button>
      </div>

      <p v-if="windowText" class="text-xs text-gray-500 dark:text-dark-400" data-testid="node-detail-window">{{ windowText }}</p>

      <p
        v-if="node.error"
        role="alert"
        class="rounded-2xl bg-red-50 px-4 py-3 text-sm text-red-700 ring-1 ring-red-200/70 dark:bg-red-950/30 dark:text-red-300 dark:ring-red-900/40"
      >
        {{ node.error }}
      </p>

      <!-- 期间汇总 -->
      <dl class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        <div v-for="tile in tiles" :key="tile.key" class="rounded-2xl bg-gray-50 px-4 py-3 dark:bg-dark-900/40" :data-testid="`node-detail-${tile.key}`">
          <dt class="flex items-center gap-1 text-xs font-medium text-gray-500 dark:text-dark-400">
            {{ tile.label }}
            <HelpTooltip v-if="tile.hint" :content="tile.hint" width-class="w-72" />
          </dt>
          <dd class="mt-1 text-xl font-semibold tabular-nums" :class="tile.muted ? 'text-gray-400 dark:text-dark-500' : 'text-gray-900 dark:text-white'">
            {{ tile.value }}
          </dd>
          <dd v-if="tile.sub" class="mt-0.5 text-xs text-gray-500 dark:text-dark-400">{{ tile.sub }}</dd>
        </div>
      </dl>

      <!-- 节点带宽上限：带宽页可直接设置 -->
      <section v-if="mode === 'bandwidth'" class="rounded-2xl border border-gray-100 p-4 dark:border-dark-700" data-testid="node-detail-limit">
        <h4 class="mb-3 text-sm font-medium text-gray-700 dark:text-dark-100">{{ t('bandwidth.limit.title') }}</h4>
        <BandwidthMeter
          :peak-mbps="peakTotal ? toMbps(peakTotal.bytes_per_second) : null"
          :current-mbps="current ? toMbps(current.total) : null"
          :limit-mbps="limitMbps"
          :empty-text="t('bandwidth.limit.emptyNode')"
          @update:limit="emit('update:limit', node.proxy.id, $event)"
        />
      </section>

      <ul v-if="notices.length" class="space-y-1 text-xs text-amber-700 dark:text-amber-400" data-testid="node-detail-notices">
        <li v-for="notice in notices" :key="notice" class="flex items-start gap-1.5">
          <Icon name="exclamationTriangle" size="xs" class="mt-px h-3.5 w-3.5 shrink-0" />
          <span>{{ notice }}</span>
        </li>
      </ul>

      <!-- 流量与峰值趋势：接收（入站）与发送（出站）堆叠，曲线为同一秒合计峰值 -->
      <section class="rounded-2xl border border-gray-100 p-4 dark:border-dark-700">
        <h4 class="mb-2 text-sm font-medium text-gray-700 dark:text-dark-100">{{ t('bandwidth.nodeDetail.trend') }}</h4>
        <TrafficTrendChart
          :samples="samples"
          :span="span"
          :grain="grain"
          :in-label="t('bandwidth.nodes.fromNode')"
          :out-label="t('bandwidth.nodes.toNode')"
          :peak-label="t('bandwidth.nodeDetail.peakLine')"
          hint-key="bandwidth.nodeDetail.trendHint"
          test-id="node-trend"
        />
      </section>

      <!-- 经此节点的服务流量：与同一小时窗口的隧道流量比较，并列出上游账号 -->
      <EgressServiceSection :data="egress.data.value" :error="egress.error.value" tunnel />

      <p class="rounded-2xl bg-gray-50 px-4 py-3 text-xs leading-relaxed text-gray-500 dark:bg-dark-900/40 dark:text-dark-400" data-testid="node-detail-basis">
        {{ t('bandwidth.nodeDetail.basis') }}
      </p>
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, toRef } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import HelpTooltip from '@/components/common/HelpTooltip.vue'
import Icon from '@/components/icons/Icon.vue'
import { toMbps } from '@/utils/bandwidthSeries'
import { formatByteCount, formatMbps, formatSampleTime } from '@/utils/trafficFormat'
import { formatDateTimeLabel } from '@/utils/wireguardTraffic'
import { grainMs } from '@/utils/wireguardTrafficChart'
import BandwidthMeter from '@/views/admin/bandwidth/components/BandwidthMeter.vue'
import type { WireGuardNodeState } from '@/views/admin/bandwidth/useWireGuardNodes'
import EgressServiceSection from './EgressServiceSection.vue'
import TrafficTrendChart from './TrafficTrendChart.vue'
import {
  nodeCurrentRate,
  nodeObservedSeconds,
  nodePeak,
  nodePeakNote,
  nodeRateNote,
  nodeStatus,
  nodeWarning
} from './nodeFormat'
import { nodeTrendSamples } from './trendSamples'
import type { PresetRange } from './usePresetRange'
import { useEgressRouteDetail } from './useEgress'

const props = withDefaults(
  defineProps<{
    node: WireGuardNodeState
    range: PresetRange
    mode: 'bandwidth' | 'traffic'
    limitMbps?: number | null
  }>(),
  { limitMbps: null }
)

const emit = defineEmits<{
  close: []
  manage: [id: number]
  'update:limit': [id: number, value: number | null]
}>()

const { t } = useI18n()

const report = computed(() => props.node.report)
const egressTarget = computed(() => ({ type: 'wireguard' as const, id: props.node.proxy.id }))
const egress = useEgressRouteDetail(egressTarget, toRef(props, 'range'))
const status = computed(() => nodeStatus(props.node.details, t))
const current = computed(() => nodeCurrentRate(props.node.details))
const peakTotal = computed(() => nodePeak(report.value, 'total'))
const accountCount = computed(() => (typeof props.node.proxy.account_count === 'number' ? props.node.proxy.account_count : null))

const windowText = computed(() => {
  const value = report.value
  if (!value) return ''
  const interval = value.interval === 'minute' ? t('bandwidth.perMinute') : t('bandwidth.perHour')
  return `${formatDateTimeLabel(value.start)} – ${formatDateTimeLabel(value.end)} · ${interval}`
})

function bytes(value: string | undefined) {
  return report.value?.has_records ? formatByteCount(value) : '—'
}

interface Tile {
  key: string
  label: string
  value: string
  sub?: string
  hint?: string
  muted?: boolean
}

const tiles = computed<Tile[]>(() => {
  const value = report.value
  const tx = nodePeak(value, 'tx')
  const rx = nodePeak(value, 'rx')
  const total = peakTotal.value
  const observed = nodeObservedSeconds(value)
  const totalBytes = value?.has_records ? Number(value.total_bytes) : null
  const list: Tile[] = [
    {
      key: 'total',
      label: t('bandwidth.nodeDetail.total'),
      value: bytes(value?.total_bytes),
      sub: value?.has_records
        ? t('bandwidth.nodeDetail.split', { toNode: formatByteCount(value.tx_bytes), fromNode: formatByteCount(value.rx_bytes) })
        : undefined
    },
    {
      key: 'peak',
      label: t('bandwidth.nodes.peak'),
      hint: t('bandwidth.nodes.peakHint'),
      value: total ? formatMbps(total.bytes_per_second) : value?.has_records ? t('bandwidth.host.unknown') : '—',
      sub: total ? t('bandwidth.host.peakAt', { time: formatSampleTime(total.at) }) : nodePeakNote(value, t),
      muted: !total
    },
    {
      key: 'directions',
      label: t('bandwidth.nodeDetail.directionPeaks'),
      value: tx || rx ? `${tx ? formatMbps(tx.bytes_per_second) : '—'} / ${rx ? formatMbps(rx.bytes_per_second) : '—'}` : '—',
      sub: t('bandwidth.nodeDetail.directionPeaksSub'),
      muted: !tx && !rx
    },
    {
      key: 'current',
      label: t('bandwidth.nodes.current'),
      value: current.value ? formatMbps(current.value.total) : nodeRateNote(props.node.details, t) || '—',
      sub: current.value
        ? t('bandwidth.nodeDetail.split', { toNode: formatMbps(current.value.tx), fromNode: formatMbps(current.value.rx) })
        : undefined,
      muted: !current.value
    },
    {
      key: 'average',
      label: t('bandwidth.nodeDetail.average'),
      value: totalBytes !== null && observed ? formatMbps(totalBytes / observed) : '—',
      sub: t('bandwidth.nodeDetail.averageSub'),
      muted: totalBytes === null || !observed
    }
  ]
  // 峰值占上限在下方带宽上限区块展示；流量页不读取上限，这里统一展示日均流量。
  list.push({
    key: 'daily',
    label: t('bandwidth.nodeDetail.daily'),
    value: totalBytes !== null && observed ? formatByteCount(Math.round((totalBytes * 86_400) / observed)) : '—',
    sub: t('bandwidth.nodeDetail.dailySub'),
    muted: totalBytes === null || !observed
  })
  return list
})

const notices = computed(() => {
  const value = report.value
  const list: string[] = []
  const warning = nodeWarning(value, t)
  if (warning) list.push(warning)
  if (value?.has_records && !value.peak_history) list.push(t('bandwidth.nodes.legacy'))
  return list
})

const grain = computed(() => grainMs(report.value?.interval ?? 'hour'))
const samples = computed(() => nodeTrendSamples(report.value?.history ?? []))
const span = computed(() => {
  const value = report.value
  if (!value) return null
  const start = Date.parse(value.start)
  const end = Date.parse(value.end)
  return Number.isNaN(start) || Number.isNaN(end) || end <= start ? null : { start, end }
})
</script>
