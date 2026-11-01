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

      <WarningList :warnings="warnings" />

      <TrafficOverview :report="report" :loading="feed.loading.value" />

      <SubjectRankingCard ref="rankingCard" mode="traffic" :range="range" @select="selected = $event" />

      <RelayPathCard
        :report="report"
        show-egress
        :egress="egressFeed.data.value"
        :egress-error="egressFeed.error.value"
        @open-route="openRoute"
      />

      <NodeTrafficCard
        :nodes="nodeFeed.nodes.value"
        :loaded="nodeFeed.loaded.value"
        :loading="nodeFeed.loading.value"
        :error="nodeFeed.error.value"
        :egress-routes="egressFeed.data.value?.routes ?? null"
        @open="detailNodeId = $event"
      />

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
            <p>{{ t('traffic.quality.duplicateBasis') }}</p>
            <p>{{ t('traffic.quality.subjectBasis') }}</p>
          </div>
          <dl class="grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-2">
            <dt class="text-gray-400 dark:text-dark-500">{{ t('traffic.quality.observed') }}</dt>
            <dd data-testid="quality-observed">{{ observedText }}</dd>
            <dt class="text-gray-400 dark:text-dark-500">{{ t('bandwidth.quality.savedAt') }}</dt>
            <dd>{{ report?.saved_at ? formatSampleTime(report.saved_at) : '—' }}</dd>
            <dt class="text-gray-400 dark:text-dark-500">{{ t('bandwidth.quality.retention') }}</dt>
            <dd>{{ t('bandwidth.quality.retentionValue') }}</dd>
          </dl>
        </div>
      </details>
    </div>

    <SubjectDetailDialog v-if="selected" :subject="selected" :range="range" mode="traffic" @close="selected = null" />

    <NodeDetailDialog
      v-if="detailNode"
      :node="detailNode"
      :range="range"
      mode="traffic"
      @close="detailNodeId = null"
      @manage="manageNode"
    />

    <EgressRouteDialog v-if="selectedRoute" :route="selectedRoute" :range="range" @close="selectedRoute = null" />

    <WireGuardDialog v-if="manageNodeId !== null" :node-id="manageNodeId" @close="manageNodeId = null" @changed="nodeFeed.refresh()" />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import WireGuardDialog from '@/components/admin/proxy/WireGuardDialog.vue'
import EgressRouteDialog from '@/components/admin/traffic/EgressRouteDialog.vue'
import NodeDetailDialog from '@/components/admin/traffic/NodeDetailDialog.vue'
import SubjectDetailDialog from '@/components/admin/traffic/SubjectDetailDialog.vue'
import SubjectRankingCard from '@/components/admin/traffic/SubjectRankingCard.vue'
import TrafficRangeBar from '@/components/admin/traffic/TrafficRangeBar.vue'
import WarningList from '@/components/admin/traffic/WarningList.vue'
import { reportWarnings } from '@/components/admin/traffic/reportWarnings'
import { usePresetRange } from '@/components/admin/traffic/usePresetRange'
import { useEgressReport } from '@/components/admin/traffic/useEgress'
import type { EgressRouteIdentity } from '@/api/admin/trafficEgress'
import type { TrafficSubjectIdentity } from '@/api/admin/trafficSubjects'
import { useServiceTrafficFeed } from '@/views/admin/bandwidth/useServiceTrafficFeed'
import { useWireGuardNodes } from '@/views/admin/bandwidth/useWireGuardNodes'
import { formatSampleTime, isMissingTime } from '@/utils/trafficFormat'
import NodeTrafficCard from './components/NodeTrafficCard.vue'
import RelayPathCard from './components/RelayPathCard.vue'
import TrafficOverview from './components/TrafficOverview.vue'

const { t } = useI18n()
const { range, options: rangeOptions } = usePresetRange()

// 流量页只看所选期间的累计与构成，不轮询实时速率。
const feed = useServiceTrafficFeed(range, { live: false })
const nodeFeed = useWireGuardNodes(range)
const egressFeed = useEgressReport(range)

const report = computed(() => feed.report.value)
const selected = ref<TrafficSubjectIdentity | null>(null)
const rankingCard = ref<InstanceType<typeof SubjectRankingCard> | null>(null)
const detailNodeId = ref<number | null>(null)
const manageNodeId = ref<number | null>(null)
const detailNode = computed(() => nodeFeed.nodes.value.find((node) => node.proxy.id === detailNodeId.value) ?? null)

const selectedRoute = ref<EgressRouteIdentity | null>(null)

// WG 出口打开节点详情（含隧道对比）；直连、代理以及已不在节点列表中的 WG 出口打开出口详情。
function openRoute(route: EgressRouteIdentity) {
  if (route.route_type === 'wireguard' && nodeFeed.nodes.value.some((node) => node.proxy.id === route.route_id)) {
    detailNodeId.value = route.route_id
    return
  }
  selectedRoute.value = route
}

function manageNode(id: number) {
  detailNodeId.value = null
  manageNodeId.value = id
}

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
  rankingCard.value?.refresh()
  nodeFeed.refresh()
  egressFeed.refresh()
}
</script>
