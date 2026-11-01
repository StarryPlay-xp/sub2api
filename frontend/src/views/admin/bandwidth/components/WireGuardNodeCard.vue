<template>
  <article
    class="flex flex-col rounded-2xl border border-gray-200/80 bg-white p-5 dark:border-dark-700 dark:bg-dark-800/60"
    :data-testid="`wg-node-${node.proxy.id}`"
  >
    <header class="flex items-start justify-between gap-3">
      <div class="min-w-0">
        <h4 class="truncate text-sm font-semibold text-gray-900 dark:text-white" :title="node.proxy.name">
          {{ node.proxy.name }}
        </h4>
        <span class="mt-1 inline-flex items-center gap-1.5 text-[11px] font-medium" :class="state.textClass" data-testid="node-state">
          <span class="h-1.5 w-1.5 rounded-full" :class="state.dotClass"></span>
          {{ state.label }}
        </span>
      </div>
      <button
        type="button"
        class="btn btn-secondary btn-sm shrink-0"
        @click="emit('open', node.proxy.id)"
      >
        {{ t('bandwidth.nodes.details') }}
      </button>
    </header>

    <div class="mt-4 flex items-center gap-1.5 text-xs text-gray-500 dark:text-dark-400">
      {{ t('bandwidth.nodes.peak') }}
      <HelpTooltip :content="t('bandwidth.nodes.peakHint')" width-class="w-72" />
    </div>
    <p class="mt-1 flex items-baseline gap-1.5" data-testid="node-peak">
      <span class="text-3xl font-semibold tracking-tight" :class="peak ? 'text-gray-900 dark:text-white' : 'text-gray-300 dark:text-dark-600'">
        {{ peakValue }}
      </span>
      <span v-if="peak" class="text-sm font-medium text-gray-500 dark:text-dark-300">Mbps</span>
      <span v-if="peak" class="ml-1 text-xs text-gray-400 tabular-nums dark:text-dark-500">{{ formatByteRate(peak.bytes_per_second) }}</span>
    </p>
    <p class="mt-1 min-h-[16px] text-[11px] text-gray-500 dark:text-dark-400">{{ peakDetail }}</p>

    <div class="mt-3 h-10">
      <BandwidthSparkline
        v-if="sparkValues.some((value) => value !== null)"
        :series="[{ key: 'total', color: palette.out, values: sparkValues, area: true }]"
        :height="40"
        :label="t('bandwidth.nodes.trend')"
      />
    </div>

    <div class="mt-4">
      <BandwidthMeter
        :peak-mbps="peak ? toMbps(peak.bytes_per_second) : null"
        :current-mbps="currentMbps"
        :limit-mbps="limit"
        :empty-text="t('bandwidth.limit.emptyNode')"
        @update:limit="emit('update:limit', node.proxy.id, $event)"
      />
    </div>

    <dl class="mt-4 grid grid-cols-3 gap-3 border-t border-gray-100 pt-4 text-xs dark:border-dark-700">
      <div>
        <dt class="text-gray-500 dark:text-dark-400">{{ t('bandwidth.nodes.sent') }}</dt>
        <dd class="mt-0.5 font-semibold text-gray-900 dark:text-white">{{ bytesText(node.report?.tx_bytes) }}</dd>
      </div>
      <div>
        <dt class="text-gray-500 dark:text-dark-400">{{ t('bandwidth.nodes.received') }}</dt>
        <dd class="mt-0.5 font-semibold text-gray-900 dark:text-white">{{ bytesText(node.report?.rx_bytes) }}</dd>
      </div>
      <div>
        <dt class="text-gray-500 dark:text-dark-400">{{ t('bandwidth.nodes.current') }}</dt>
        <dd class="mt-0.5 font-semibold text-gray-900 dark:text-white" data-testid="node-current">{{ currentText }}</dd>
      </div>
    </dl>

    <p
      v-if="node.error"
      role="alert"
      class="mt-3 rounded-lg bg-red-50 px-3 py-2 text-[11px] text-red-700 dark:bg-red-950/30 dark:text-red-300"
    >
      {{ node.error }}
    </p>
    <p
      v-else-if="warning"
      class="mt-3 flex items-start gap-1.5 rounded-lg bg-amber-50/80 px-3 py-2 text-[11px] leading-relaxed text-amber-700 dark:bg-amber-950/25 dark:text-amber-300"
    >
      <Icon name="exclamationTriangle" size="xs" class="mt-px h-3.5 w-3.5 shrink-0" />
      {{ warning }}
    </p>
  </article>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import HelpTooltip from '@/components/common/HelpTooltip.vue'
import Icon from '@/components/icons/Icon.vue'
import { buildPeakBins, nodeSamples, toMbps } from '@/utils/bandwidthSeries'
import { formatByteCount, formatByteRate, formatMbps, formatSampleTime } from '@/utils/trafficFormat'
import { grainMs } from '@/utils/wireguardTrafficChart'
import BandwidthMeter from './BandwidthMeter.vue'
import BandwidthSparkline from './BandwidthSparkline.vue'
import { useBandwidthPalette } from '../useBandwidthPalette'
import type { WireGuardNodeState } from '../useWireGuardNodes'

const props = defineProps<{ node: WireGuardNodeState; limit: number | null }>()
const emit = defineEmits<{ open: [id: number]; 'update:limit': [id: number, value: number | null] }>()

const { t } = useI18n()
const palette = useBandwidthPalette()

// 自建中转节点每个方向约承担本站发送 + 接收，因此取同一秒合计峰值。
const peak = computed(() => props.node.report?.peak_history?.peaks?.total ?? null)

const peakValue = computed(() => {
  if (peak.value) return formatMbps(peak.value.bytes_per_second).replace(' Mbps', '')
  if (!props.node.report) return '—'
  return props.node.report.has_records ? t('bandwidth.host.unknown') : '—'
})

const peakDetail = computed(() => {
  const report = props.node.report
  if (peak.value) return t('bandwidth.host.peakAt', { time: formatSampleTime(peak.value.at) })
  if (!report) return ''
  if (!report.has_records) return t('bandwidth.nodes.noRecords')
  return report.peak_history ? t('bandwidth.host.noValidSample') : t('bandwidth.nodes.legacy')
})

const rate = computed(() => props.node.details?.rate ?? null)
const sampled = computed(() => rate.value?.state === 'sampled')

const currentMbps = computed(() =>
  sampled.value && rate.value ? toMbps(rate.value.tx_bytes_per_second + rate.value.rx_bytes_per_second) : null
)

const currentText = computed(() => {
  if (sampled.value && rate.value) return formatMbps(rate.value.tx_bytes_per_second + rate.value.rx_bytes_per_second)
  if (rate.value?.state === 'invalid_interval') return t('bandwidth.nodes.rateDelayed')
  if (rate.value?.state === 'warming_up') return t('bandwidth.nodes.rateWarmingUp')
  return '—'
})

const state = computed(() => {
  const details = props.node.details
  if (details?.node.paused) {
    return { label: t('bandwidth.nodes.paused'), textClass: 'text-red-600 dark:text-red-400', dotClass: 'bg-red-500' }
  }
  if (details && !details.node.active) {
    return { label: t('bandwidth.nodes.inactive'), textClass: 'text-gray-500 dark:text-dark-400', dotClass: 'bg-gray-400' }
  }
  if (details && details.rate.state !== 'not_running') {
    return {
      label: t('bandwidth.nodes.running'),
      textClass: 'text-emerald-600 dark:text-emerald-400',
      dotClass: 'bg-emerald-500 motion-safe:animate-pulse'
    }
  }
  return { label: t('bandwidth.nodes.notRunning'), textClass: 'text-gray-500 dark:text-dark-400', dotClass: 'bg-gray-400' }
})

const sparkValues = computed(() => {
  const report = props.node.report
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
})

const warning = computed(() => {
  const report = props.node.report
  if (!report) return ''
  const invalid = report.peak_history?.quality?.invalid_samples ?? 0
  if (invalid > 0) return t('bandwidth.nodes.invalidSamples', { count: invalid })
  if (report.incomplete) return t('bandwidth.nodes.incomplete')
  return ''
})

function bytesText(value?: string) {
  return props.node.report?.has_records ? formatByteCount(value) : '—'
}
</script>
