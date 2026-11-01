<template>
  <AppLayout>
    <div class="space-y-6 pb-12">
      <!-- 统一控制栏：范围作用于本页全部内容 -->
      <div class="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
        <div
          class="inline-flex self-start rounded-xl border border-gray-200 bg-white p-1 shadow-sm dark:border-dark-700 dark:bg-dark-800"
          role="group"
          :aria-label="t('bandwidth.rangeLabel')"
        >
          <button
            v-for="option in rangeOptions"
            :key="option.value"
            type="button"
            class="rounded-lg px-3.5 py-1.5 text-sm font-medium transition-colors duration-150 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/40"
            :class="
              range === option.value
                ? 'bg-primary-600 text-white shadow-sm dark:bg-primary-500'
                : 'text-gray-600 hover:bg-gray-50 hover:text-gray-900 dark:text-dark-300 dark:hover:bg-dark-700 dark:hover:text-white'
            "
            :aria-pressed="range === option.value"
            @click="range = option.value"
          >
            {{ option.label }}
          </button>
        </div>

        <div class="flex flex-wrap items-center gap-x-4 gap-y-2 text-xs text-gray-500 dark:text-dark-400">
          <span v-if="report" class="inline-flex flex-wrap items-center gap-x-2" data-testid="range-text">
            <Icon name="clock" size="xs" class="h-3.5 w-3.5" />
            <span class="font-medium text-gray-700 dark:text-dark-200">{{ rangeText }}</span>
            <span :title="t('bandwidth.timeZone')">{{ timezoneText }}</span>
            <span>{{ report.interval === 'minute' ? t('bandwidth.perMinute') : t('bandwidth.perHour') }}</span>
            <span v-if="report.end_clamped" class="text-amber-600 dark:text-amber-400">{{ t('bandwidth.endClamped') }}</span>
          </span>
          <button
            type="button"
            class="btn btn-secondary btn-sm"
            :disabled="feed.loading.value && nodeFeed.loading.value"
            @click="refreshAll"
          >
            <Icon name="refresh" size="xs" class="h-3.5 w-3.5" :class="{ 'motion-safe:animate-spin': feed.loading.value }" />
            {{ t('bandwidth.refresh') }}
          </button>
        </div>
      </div>

      <p
        v-if="feed.error.value"
        role="alert"
        class="rounded-2xl bg-red-50 px-4 py-3 text-sm text-red-700 ring-1 ring-red-200/70 dark:bg-red-950/30 dark:text-red-300 dark:ring-red-900/40"
      >
        {{ feed.error.value }}
      </p>

      <!-- 数据可信度提示：集中放在最上方，避免散落在各卡片里 -->
      <div
        v-if="warnings.length"
        data-testid="host-warnings"
        class="space-y-1.5 rounded-2xl bg-amber-50/80 px-4 py-3 text-xs leading-relaxed text-amber-800 ring-1 ring-amber-200/60 dark:bg-amber-950/25 dark:text-amber-300 dark:ring-amber-900/40"
      >
        <p v-for="warning in warnings" :key="warning.key" class="flex items-start gap-2">
          <Icon name="exclamationTriangle" size="xs" class="mt-px h-3.5 w-3.5 shrink-0" />
          <span>{{ warning.text }}</span>
        </p>
      </div>

      <HostBandwidthHero
        :report="report"
        :current="feed.current.value"
        :current-status="feed.currentStatus.value"
        :live-samples="feed.liveSamples.value"
        :limit="limits.hostLimit.value"
        :loading="feed.loading.value"
        @update:limit="limits.setHostLimit"
      />

      <PeakTrendChart :report="report" :limit="limits.hostLimit.value" :loading="feed.loading.value" />

      <RelayPathCard :report="report" />

      <!-- WireGuard 节点：节点带宽按发送 + 接收同一秒合计计算 -->
      <section data-testid="wg-nodes">
        <header class="mb-4 flex flex-col gap-2 sm:flex-row sm:items-end sm:justify-between">
          <div>
            <h3 class="flex items-center gap-2 text-base font-semibold text-gray-900 dark:text-white">
              {{ t('bandwidth.nodes.title') }}
              <span v-if="nodeFeed.nodes.value.length" class="badge badge-gray">
                {{ t('bandwidth.nodes.count', { count: nodeFeed.nodes.value.length }) }}
              </span>
            </h3>
            <p class="mt-0.5 text-xs text-gray-500 dark:text-dark-400">{{ t('bandwidth.nodes.subtitle') }}</p>
          </div>
          <RouterLink
            to="/admin/proxies"
            class="text-sm font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400"
          >
            {{ t('bandwidth.nodes.manage') }}
          </RouterLink>
        </header>

        <p
          v-if="nodeFeed.error.value"
          role="alert"
          class="rounded-2xl bg-red-50 px-4 py-3 text-sm text-red-700 dark:bg-red-950/30 dark:text-red-300"
        >
          {{ nodeFeed.error.value }}
        </p>
        <div v-else-if="!nodeFeed.loaded.value" class="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          <div v-for="index in 2" :key="index" class="h-[360px] animate-pulse rounded-2xl bg-gray-100 dark:bg-dark-700/50"></div>
        </div>
        <div
          v-else-if="!nodeFeed.nodes.value.length"
          class="flex flex-col items-center justify-center gap-2 rounded-2xl border border-dashed border-gray-300 px-6 py-10 text-center dark:border-dark-600"
          data-testid="wg-empty"
        >
          <p class="text-sm font-medium text-gray-600 dark:text-dark-200">{{ t('bandwidth.nodes.empty') }}</p>
          <p class="max-w-md text-xs text-gray-400 dark:text-dark-500">{{ t('bandwidth.nodes.emptyHint') }}</p>
        </div>
        <div v-else class="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          <WireGuardNodeCard
            v-for="node in nodeFeed.nodes.value"
            :key="node.proxy.id"
            :node="node"
            :limit="limits.nodeLimit(node.proxy.id)"
            @open="dialogNodeId = $event"
            @update:limit="limits.setNodeLimit"
          />
        </div>
      </section>

      <!-- 统计口径与数据质量 -->
      <details
        class="group rounded-2xl border border-gray-200/80 bg-white px-5 py-4 dark:border-dark-700 dark:bg-dark-800/60"
        data-testid="quality"
      >
        <summary
          class="flex cursor-pointer list-none items-center justify-between gap-3 text-sm font-medium text-gray-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/40 dark:text-dark-100"
        >
          {{ t('bandwidth.quality.title') }}
          <Icon name="chevronDown" size="sm" class="text-gray-400 transition-transform duration-200 group-open:rotate-180" />
        </summary>
        <div class="mt-4 grid gap-6 text-xs leading-relaxed text-gray-600 dark:text-dark-300 lg:grid-cols-2">
          <div class="space-y-2">
            <p>{{ t('bandwidth.quality.hostBasis') }}</p>
            <p>{{ t('bandwidth.quality.nodeBasis') }}</p>
            <p>{{ t('bandwidth.quality.peakBasis') }}</p>
          </div>
          <dl class="grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-2">
            <dt class="text-gray-400 dark:text-dark-500">{{ t('bandwidth.quality.observed') }}</dt>
            <dd data-testid="quality-observed">{{ observedText }}</dd>
            <dt class="text-gray-400 dark:text-dark-500">{{ t('bandwidth.quality.samples') }}</dt>
            <dd>
              {{
                report
                  ? t('bandwidth.quality.samplesValue', {
                      valid: report.quality?.valid_samples ?? 0,
                      invalid: report.quality?.invalid_samples ?? 0
                    })
                  : '—'
              }}
            </dd>
            <dt class="text-gray-400 dark:text-dark-500">{{ t('bandwidth.quality.savedAt') }}</dt>
            <dd>{{ report?.saved_at ? formatSampleTime(report.saved_at) : '—' }}</dd>
            <dt class="text-gray-400 dark:text-dark-500">{{ t('bandwidth.quality.lastPersisted') }}</dt>
            <dd>{{ feed.lastPersistedAt.value ? formatSampleTime(feed.lastPersistedAt.value) : '—' }}</dd>
            <dt class="text-gray-400 dark:text-dark-500">{{ t('bandwidth.quality.retention') }}</dt>
            <dd>{{ t('bandwidth.quality.retentionValue') }}</dd>
            <dt class="text-gray-400 dark:text-dark-500">{{ t('bandwidth.quality.limitStorage') }}</dt>
            <dd>{{ t('bandwidth.quality.limitStorageValue') }}</dd>
          </dl>
        </div>
      </details>
    </div>

    <WireGuardDialog
      v-if="dialogNodeId !== null"
      :node-id="dialogNodeId"
      @close="dialogNodeId = null"
      @changed="nodeFeed.refresh()"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import WireGuardDialog from '@/components/admin/proxy/WireGuardDialog.vue'
import { formatDateTimeLabel, timeZoneName, utcOffsetLabel } from '@/utils/wireguardTraffic'
import { formatSampleTime, isMissingTime } from '@/utils/trafficFormat'
import HostBandwidthHero from './components/HostBandwidthHero.vue'
import PeakTrendChart from './components/PeakTrendChart.vue'
import RelayPathCard from './components/RelayPathCard.vue'
import WireGuardNodeCard from './components/WireGuardNodeCard.vue'
import { useBandwidthLimits } from './useBandwidthLimits'
import { useServiceTrafficFeed, type PresetRange } from './useServiceTrafficFeed'
import { useWireGuardNodes } from './useWireGuardNodes'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()

const PRESETS: PresetRange[] = ['24h', '7d', '30d', '90d']

function rangeFromQuery(): PresetRange {
  const value = route.query.range
  return typeof value === 'string' && (PRESETS as string[]).includes(value) ? (value as PresetRange) : '24h'
}

const range = ref<PresetRange>(rangeFromQuery())

// 范围写入地址栏，刷新或分享链接时保持。
watch(range, (value) => {
  const query = { ...route.query }
  if (value === '24h') delete query.range
  else query.range = value
  void router.replace({ query })
})

const rangeOptions = computed(() =>
  PRESETS.map((value) => ({ value, label: t(`bandwidth.range.${value}`) }))
)

const feed = useServiceTrafficFeed(range)
const nodeFeed = useWireGuardNodes(range)
const limits = useBandwidthLimits()
const dialogNodeId = ref<number | null>(null)

const report = computed(() => feed.report.value)

const rangeText = computed(() =>
  report.value ? `${formatDateTimeLabel(report.value.start)} – ${formatDateTimeLabel(report.value.end)}` : ''
)

const timezoneText = computed(() => {
  const name = timeZoneName()
  if (!report.value) return name
  const startOffset = utcOffsetLabel(new Date(report.value.start))
  const endOffset = utcOffsetLabel(new Date(report.value.end))
  return startOffset === endOffset ? `${name} (${startOffset})` : `${name} (${startOffset} → ${endOffset})`
})

const observedText = computed(() => {
  const quality = report.value?.quality
  if (!quality || isMissingTime(quality.first_sample_at) || isMissingTime(quality.last_sample_at)) {
    return t('bandwidth.quality.noSamples')
  }
  return `${formatSampleTime(quality.first_sample_at)} – ${formatSampleTime(quality.last_sample_at)}`
})

// 首个样本明显晚于所选期间开始（超过一个时间桶）时，提示峰值不覆盖整个期间。
const coverageStart = computed(() => {
  const value = report.value
  const first = value?.quality?.first_sample_at
  if (!value?.has_records || !first || isMissingTime(first)) return ''
  const grain = value.interval === 'minute' ? 60_000 : 3_600_000
  return Date.parse(first) - Date.parse(value.start) > grain ? formatSampleTime(first) : ''
})

const warnings = computed(() => {
  const list: { key: string; text: string }[] = []
  const value = report.value
  if (value?.concurrent_runtimes) list.push({ key: 'concurrent', text: t('bandwidth.warnings.concurrent') })
  if (value?.incomplete) list.push({ key: 'incomplete', text: t('bandwidth.warnings.incomplete') })
  const invalid = value?.quality?.invalid_samples ?? 0
  if (invalid > 0) list.push({ key: 'invalid', text: t('bandwidth.warnings.invalidSamples', { count: invalid }) })
  if (coverageStart.value) {
    list.push({ key: 'coverage', text: t('bandwidth.warnings.partialCoverage', { time: coverageStart.value }) })
  }
  if (value && !value.has_records) list.push({ key: 'empty', text: t('bandwidth.warnings.noRecords') })
  if (feed.persistenceError.value) list.push({ key: 'persistence', text: t('bandwidth.warnings.persistenceError') })
  return list
})

function refreshAll() {
  feed.refresh()
  nodeFeed.refresh()
}
</script>
