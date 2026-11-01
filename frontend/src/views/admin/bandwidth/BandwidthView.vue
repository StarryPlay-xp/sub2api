<template>
  <AppLayout>
    <div class="space-y-6 pb-12">
      <TrafficRangeBar
        v-model="range"
        :options="rangeOptions"
        :window="report"
        :loading="feed.loading.value"
        :refresh-disabled="feed.loading.value && nodeFeed.loading.value"
        @refresh="refreshAll"
      />

      <p
        v-if="feed.error.value"
        role="alert"
        class="rounded-2xl bg-red-50 px-4 py-3 text-sm text-red-700 ring-1 ring-red-200/70 dark:bg-red-950/30 dark:text-red-300 dark:ring-red-900/40"
      >
        {{ feed.error.value }}
      </p>

      <!-- 数据可信度提示：集中放在最上方，避免散落在各卡片里 -->
      <WarningList :warnings="warnings" />

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

      <!-- 按用户 / API Key：此刻谁在占用带宽，以及所选期间谁的峰值最高 -->
      <LiveSubjectsCard ref="liveCard" @select="selected = $event" />

      <SubjectRankingCard
        ref="rankingCard"
        mode="bandwidth"
        :range="range"
        :limit-mbps="limits.hostLimit.value"
        @select="selected = $event"
      />

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
            <p>{{ t('traffic.quality.subjectBasis') }}</p>
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

    <SubjectDetailDialog
      v-if="selected"
      :subject="selected"
      :range="range"
      mode="bandwidth"
      :limit-mbps="limits.hostLimit.value"
      @close="selected = null"
    />

    <WireGuardDialog
      v-if="dialogNodeId !== null"
      :node-id="dialogNodeId"
      @close="dialogNodeId = null"
      @changed="nodeFeed.refresh()"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import WireGuardDialog from '@/components/admin/proxy/WireGuardDialog.vue'
import LiveSubjectsCard from '@/components/admin/traffic/LiveSubjectsCard.vue'
import SubjectDetailDialog from '@/components/admin/traffic/SubjectDetailDialog.vue'
import SubjectRankingCard from '@/components/admin/traffic/SubjectRankingCard.vue'
import TrafficRangeBar from '@/components/admin/traffic/TrafficRangeBar.vue'
import WarningList from '@/components/admin/traffic/WarningList.vue'
import { reportWarnings } from '@/components/admin/traffic/reportWarnings'
import { usePresetRange } from '@/components/admin/traffic/usePresetRange'
import type { TrafficSubjectIdentity } from '@/api/admin/trafficSubjects'
import { formatSampleTime, isMissingTime } from '@/utils/trafficFormat'
import HostBandwidthHero from './components/HostBandwidthHero.vue'
import PeakTrendChart from './components/PeakTrendChart.vue'
import WireGuardNodeCard from './components/WireGuardNodeCard.vue'
import { useBandwidthLimits } from './useBandwidthLimits'
import { useServiceTrafficFeed } from './useServiceTrafficFeed'
import { useWireGuardNodes } from './useWireGuardNodes'

const { t } = useI18n()
const { range, options: rangeOptions } = usePresetRange()

const feed = useServiceTrafficFeed(range)
const nodeFeed = useWireGuardNodes(range)
const limits = useBandwidthLimits()
const dialogNodeId = ref<number | null>(null)
const selected = ref<TrafficSubjectIdentity | null>(null)
const liveCard = ref<InstanceType<typeof LiveSubjectsCard> | null>(null)
const rankingCard = ref<InstanceType<typeof SubjectRankingCard> | null>(null)

const report = computed(() => feed.report.value)

const observedText = computed(() => {
  const quality = report.value?.quality
  if (!quality || isMissingTime(quality.first_sample_at) || isMissingTime(quality.last_sample_at)) {
    return t('bandwidth.quality.noSamples')
  }
  return `${formatSampleTime(quality.first_sample_at)} – ${formatSampleTime(quality.last_sample_at)}`
})

const warnings = computed(() => reportWarnings(report.value, feed.persistenceError.value, t))

function refreshAll() {
  feed.refresh()
  nodeFeed.refresh()
  void limits.refresh()
  liveCard.value?.refresh()
  rankingCard.value?.refresh()
}
</script>
