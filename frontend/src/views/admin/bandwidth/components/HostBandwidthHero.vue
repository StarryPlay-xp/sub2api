<template>
  <section
    class="overflow-hidden rounded-3xl bg-white shadow-sm ring-1 ring-gray-900/5 dark:bg-dark-800 dark:ring-dark-700"
    data-testid="host-hero"
    :aria-busy="loading"
  >
    <div class="grid lg:grid-cols-[minmax(0,1.65fr)_minmax(0,1fr)]">
      <!-- 所选期间本机单方向最忙的 1 秒 -->
      <div class="p-6 sm:p-7">
        <div class="flex items-center gap-1.5 text-sm font-medium text-gray-600 dark:text-dark-200">
          {{ t('bandwidth.host.peak') }}
          <HelpTooltip :content="t('bandwidth.host.peakHint')" width-class="w-80" />
        </div>

        <!-- 左侧大数字，右侧用标签列出方向、时间和另一方向峰值；窄屏时右侧内容落到数字下方 -->
        <div class="mt-3 flex flex-col gap-5 md:flex-row md:items-end md:justify-between md:gap-10">
          <div class="flex min-h-[64px] flex-wrap items-end gap-x-4 gap-y-2">
            <template v-if="busier">
              <p class="flex items-baseline gap-2 text-gray-900 dark:text-white" data-testid="host-peak">
                <span class="text-5xl font-semibold leading-none tracking-tight sm:text-6xl">{{ heroMbps.value }}</span>
                <span class="text-xl font-medium text-gray-500 dark:text-dark-300">Mbps</span>
              </p>
              <p class="pb-1 text-sm text-gray-500 tabular-nums dark:text-dark-300">
                {{ formatByteRate(busier.peak.bytes_per_second) }}
              </p>
            </template>
            <div v-else-if="loading && !report" class="h-14 w-56 animate-pulse rounded-xl bg-gray-100 dark:bg-dark-700/60"></div>
            <p v-else class="flex flex-col gap-1" data-testid="host-peak">
              <span class="text-5xl font-semibold leading-none tracking-tight text-gray-300 dark:text-dark-600">
                {{ report?.has_records ? t('bandwidth.host.unknown') : '—' }}
              </span>
            </p>
          </div>

          <dl
            v-if="busier"
            class="grid shrink-0 grid-cols-[auto_minmax(0,1fr)] items-baseline gap-x-6 gap-y-2.5 text-sm md:border-l md:border-gray-100 md:pl-8 md:dark:border-dark-700"
            data-testid="host-peak-detail"
          >
            <dt class="text-xs text-gray-400 dark:text-dark-500">{{ t('bandwidth.host.directionLabel') }}</dt>
            <dd class="inline-flex items-center gap-1.5 font-medium text-gray-800 dark:text-dark-100">
              <span class="h-2.5 w-2.5 rounded-full" :style="{ backgroundColor: colorOf(busier.direction) }"></span>
              {{ directionLabel(busier.direction) }}
            </dd>
            <dt class="text-xs text-gray-400 dark:text-dark-500">{{ t('bandwidth.host.timeLabel') }}</dt>
            <dd class="text-gray-700 tabular-nums dark:text-dark-200">{{ formatSampleTime(busier.peak.at) }}</dd>
            <dt class="text-xs text-gray-400 dark:text-dark-500">
              {{ t('bandwidth.host.otherLabel', { direction: directionLabel(busier.otherDirection) }) }}
            </dt>
            <dd class="flex flex-wrap items-baseline gap-x-2 text-gray-700 tabular-nums dark:text-dark-200">
              <span class="font-medium">{{ formatMbps(busier.other.bytes_per_second) }}</span>
              <span class="text-xs text-gray-400 dark:text-dark-500">{{ formatSampleTime(busier.other.at) }}</span>
            </dd>
          </dl>
          <p
            v-else-if="report"
            class="text-sm text-gray-500 md:max-w-xs md:border-l md:border-gray-100 md:pl-8 md:dark:border-dark-700 dark:text-dark-400"
          >
            {{ unknownReason }}
          </p>
        </div>

        <div class="mt-5">
          <BandwidthMeter
            :peak-mbps="busier ? toMbps(busier.peak.bytes_per_second) : null"
            :current-mbps="currentBusierMbps"
            :limit-mbps="limit"
            :empty-text="t('bandwidth.limit.emptyHost')"
            @update:limit="emit('update:limit', $event)"
          />
        </div>
      </div>

      <!-- 当前进程最近的约 1 秒速率；不随所选范围变化 -->
      <div
        class="border-t border-gray-100 bg-gray-50/60 p-6 dark:border-dark-700 dark:bg-dark-900/30 sm:p-7 lg:border-l lg:border-t-0"
        data-testid="host-live"
      >
        <div class="flex flex-wrap items-center justify-between gap-2">
          <span class="inline-flex items-center gap-1.5 text-sm font-medium text-gray-600 dark:text-dark-200">
            {{ t('bandwidth.host.live') }}
            <HelpTooltip :content="t('bandwidth.host.liveHint')" width-class="w-72" />
          </span>
          <span
            class="inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-[11px] font-medium"
            :class="statusBadgeClass"
            data-testid="live-status"
          >
            <span class="h-1.5 w-1.5 rounded-full" :class="statusDotClass"></span>
            {{ statusText }}
          </span>
        </div>

        <dl class="mt-5 grid grid-cols-2 gap-4">
          <div v-for="item in liveRows" :key="item.key" :data-testid="`live-${item.key}`">
            <dt class="flex items-center gap-1.5 text-xs text-gray-500 dark:text-dark-400">
              <span class="h-0.5 w-3 rounded-full" :style="{ backgroundColor: item.color }"></span>
              {{ item.label }}
            </dt>
            <dd class="mt-1.5">
              <span class="text-2xl font-semibold tracking-tight text-gray-900 dark:text-white">{{ item.mbps }}</span>
              <span class="mt-0.5 block text-xs text-gray-500 tabular-nums dark:text-dark-400">{{ item.rate }}</span>
            </dd>
          </div>
        </dl>

        <div class="mt-5">
          <BandwidthSparkline
            v-if="liveSamples.length > 1"
            :series="sparkSeries"
            :height="64"
            :label="t('bandwidth.host.liveChart')"
          />
          <div
            v-else
            class="flex h-16 items-center justify-center rounded-lg border border-dashed border-gray-200 text-[11px] text-gray-400 dark:border-dark-700 dark:text-dark-500"
          >
            {{ t('bandwidth.host.liveCollecting') }}
          </div>
          <p class="mt-2 flex flex-wrap justify-between gap-2 text-[11px] text-gray-400 tabular-nums dark:text-dark-500">
            <span>{{ t('bandwidth.host.liveChart') }}</span>
            <span v-if="sampledAt">{{ t('bandwidth.host.sampledAt', { time: sampledAt }) }}</span>
          </p>
        </div>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import HelpTooltip from '@/components/common/HelpTooltip.vue'
import type { ServiceTrafficCurrent, ServiceTrafficReport } from '@/api/admin/ops'
import { busierPeak, toMbps, type HostDirection } from '@/utils/bandwidthSeries'
import { formatByteRate, formatMbps, formatSampleTime, isMissingTime } from '@/utils/trafficFormat'
import BandwidthMeter from './BandwidthMeter.vue'
import BandwidthSparkline from './BandwidthSparkline.vue'
import { useBandwidthPalette } from '../useBandwidthPalette'
import type { CurrentStatus, LiveSample } from '../useServiceTrafficFeed'

const props = defineProps<{
  report: ServiceTrafficReport | null
  current: ServiceTrafficCurrent | null
  currentStatus: CurrentStatus
  liveSamples: LiveSample[]
  limit: number | null
  loading: boolean
}>()

const emit = defineEmits<{ 'update:limit': [value: number | null] }>()

const { t } = useI18n()
const palette = useBandwidthPalette()

const busier = computed(() => busierPeak(props.report?.peaks?.in, props.report?.peaks?.out))

function mbpsParts(bytesPerSecond: number) {
  const mbps = toMbps(bytesPerSecond)
  const digits = mbps === 0 ? 0 : mbps < 1 ? 3 : mbps < 100 ? 2 : 1
  return { value: String(parseFloat(mbps.toFixed(digits))) }
}

const heroMbps = computed(() => mbpsParts(busier.value?.peak.bytes_per_second ?? 0))

const unknownReason = computed(() => {
  const report = props.report
  if (!report?.has_records) return t('bandwidth.host.noRecords')
  if (report.concurrent_runtimes) return t('bandwidth.host.concurrent')
  return t('bandwidth.host.noValidSample')
})

function directionLabel(direction: HostDirection) {
  return direction === 'in' ? t('bandwidth.direction.in') : t('bandwidth.direction.out')
}

function colorOf(direction: HostDirection) {
  return direction === 'in' ? palette.value.in : palette.value.out
}

const sampled = computed(() => props.currentStatus === 'sampled')

// 只有正常 1 秒采样才显示数值；采样延迟、等待采样、陈旧数据都不作为可信速率。
function liveRate(key: HostDirection) {
  const value = props.current?.bytes_per_second?.[key]
  return sampled.value && typeof value === 'number' ? value : null
}

const currentBusierMbps = computed(() => {
  const inbound = liveRate('in')
  const outbound = liveRate('out')
  return inbound === null || outbound === null ? null : toMbps(Math.max(inbound, outbound))
})

const liveRows = computed(() =>
  (['in', 'out'] as const).map((key) => {
    const value = liveRate(key)
    return {
      key,
      label: directionLabel(key),
      color: colorOf(key),
      mbps: value === null ? '—' : formatMbps(value),
      rate: value === null ? '' : formatByteRate(value)
    }
  })
)

const sparkSeries = computed(() => [
  { key: 'in', color: palette.value.in, values: props.liveSamples.map((sample) => sample.in), area: true },
  { key: 'out', color: palette.value.out, values: props.liveSamples.map((sample) => sample.out), area: true }
])

const sampledAt = computed(() => {
  const value = props.current?.sampled_at
  return value && !isMissingTime(value) ? formatSampleTime(value) : ''
})

const statusText = computed(() => {
  const seconds = props.current?.window_seconds
  const text = typeof seconds === 'number' && Number.isFinite(seconds) ? String(parseFloat(seconds.toFixed(2))) : '—'
  switch (props.currentStatus) {
    case 'sampled':
      return t('bandwidth.host.liveSampled', { seconds: text })
    case 'warming_up':
      return t('bandwidth.host.liveWarmingUp')
    case 'invalid_interval':
      return t('bandwidth.host.liveDelayed', { seconds: text })
    case 'stale':
      return t('bandwidth.host.liveStale')
    default:
      return t('bandwidth.host.liveUnavailable')
  }
})

const statusBadgeClass = computed(() =>
  props.currentStatus === 'sampled'
    ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/25 dark:text-emerald-300'
    : props.currentStatus === 'warming_up' || props.currentStatus === 'unavailable'
      ? 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-dark-300'
      : 'bg-amber-50 text-amber-700 dark:bg-amber-950/30 dark:text-amber-300'
)

const statusDotClass = computed(() =>
  props.currentStatus === 'sampled'
    ? 'bg-emerald-500 motion-safe:animate-pulse'
    : props.currentStatus === 'warming_up' || props.currentStatus === 'unavailable'
      ? 'bg-gray-400 dark:bg-dark-400'
      : 'bg-amber-500'
)
</script>
