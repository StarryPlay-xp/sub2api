<template>
  <section
    :class="embedded ? '' : 'rounded-3xl bg-white p-6 shadow-sm ring-1 ring-gray-900/5 dark:bg-dark-800 dark:ring-dark-700'"
    :data-testid="embedded ? 'ranking-embedded' : `ranking-${mode}`"
    :aria-busy="ranking.loading.value"
  >
    <header v-if="!embedded" class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
      <div>
        <h3 class="text-base font-semibold text-gray-900 dark:text-white">{{ t(`traffic.ranking.${mode}.title`) }}</h3>
        <p class="mt-0.5 text-xs text-gray-500 dark:text-dark-400">{{ t(`traffic.ranking.${mode}.subtitle`) }}</p>
      </div>
      <SubjectTypeSwitch v-model="ranking.subject.value" />
    </header>

    <!-- 搜索、环比与导出 -->
    <div class="flex flex-wrap items-center gap-3" :class="embedded ? '' : 'mt-4'">
      <div class="w-full sm:w-72">
        <SearchInput v-model="searchText" :placeholder="searchPlaceholder" @search="ranking.query.value = $event" />
      </div>
      <label
        v-if="mode === 'traffic'"
        class="inline-flex cursor-pointer items-center gap-2 text-sm text-gray-600 dark:text-dark-300"
        data-testid="ranking-compare"
      >
        <input v-model="ranking.compare.value" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-dark-600 dark:bg-dark-800" />
        {{ t('traffic.ranking.compare') }}
      </label>
      <button
        v-if="!embedded"
        type="button"
        class="btn btn-secondary btn-sm sm:ml-auto"
        :disabled="exporting || !items.length"
        data-testid="ranking-export"
        @click="exportCsv"
      >
        <Icon name="download" size="xs" class="h-3.5 w-3.5" :class="{ 'motion-safe:animate-pulse': exporting }" />
        {{ exporting ? t('traffic.ranking.exporting') : t('traffic.ranking.export') }}
      </button>
    </div>

    <!-- 集中度、未归属与统计起点 -->
    <dl v-if="!embedded && data" class="mt-4 grid gap-3 md:grid-cols-3" data-testid="ranking-summary">
      <div class="rounded-2xl bg-gray-50 px-4 py-3 dark:bg-dark-900/40" data-testid="ranking-concentration">
        <dt class="text-xs font-medium text-gray-500 dark:text-dark-400">{{ t('traffic.ranking.concentration') }}</dt>
        <dd class="mt-1 text-sm font-semibold tabular-nums text-gray-900 dark:text-white">{{ concentrationText }}</dd>
        <dd class="mt-0.5 text-[11px] text-gray-400 dark:text-dark-500">
          {{ t(`traffic.ranking.subjectCount.${ranking.subject.value}`, { count: data.concentration.subjects }) }}
        </dd>
      </div>
      <div class="rounded-2xl bg-gray-50 px-4 py-3 dark:bg-dark-900/40" data-testid="ranking-unattributed">
        <dt class="flex items-center gap-1 text-xs font-medium text-gray-500 dark:text-dark-400">
          {{ t('traffic.ranking.unattributed') }}
          <HelpTooltip :content="t('traffic.ranking.unattributedHint')" />
        </dt>
        <dd class="mt-1 text-sm font-semibold tabular-nums text-gray-900 dark:text-white">
          {{ formatByteCount(data.totals.unattributed.total_bytes) }}
          <span class="ml-1 text-xs font-normal text-gray-500 dark:text-dark-400">{{ unattributedShare }}</span>
        </dd>
        <dd class="mt-0.5 text-[11px] text-gray-400 dark:text-dark-500">
          {{ t('traffic.ranking.allTotal', { total: formatByteCount(data.totals.all_total_bytes) }) }}
        </dd>
      </div>
      <div class="rounded-2xl bg-gray-50 px-4 py-3 dark:bg-dark-900/40" data-testid="ranking-coverage">
        <dt class="text-xs font-medium text-gray-500 dark:text-dark-400">{{ t('traffic.ranking.coverage') }}</dt>
        <dd class="mt-1 text-sm font-semibold text-gray-900 dark:text-white">{{ coverageText }}</dd>
        <dd class="mt-0.5 text-[11px] text-gray-400 dark:text-dark-500">{{ t('traffic.ranking.coverageHint') }}</dd>
      </div>
    </dl>

    <ul
      v-if="notices.length"
      class="mt-3 space-y-1 text-xs leading-relaxed text-amber-700 dark:text-amber-400"
      data-testid="ranking-notices"
    >
      <li v-for="notice in notices" :key="notice.key" class="flex items-start gap-1.5">
        <Icon name="exclamationTriangle" size="xs" class="mt-px h-3.5 w-3.5 shrink-0" />
        <span>{{ notice.text }}</span>
      </li>
    </ul>

    <p
      v-if="ranking.error.value"
      role="alert"
      class="mt-4 rounded-2xl bg-red-50 px-4 py-3 text-sm text-red-700 ring-1 ring-red-200/70 dark:bg-red-950/30 dark:text-red-300 dark:ring-red-900/40"
    >
      {{ ranking.error.value }}
    </p>

    <div v-else class="mt-4 overflow-x-auto" :class="embedded ? '' : '-mx-6'">
      <table class="w-full min-w-max text-sm">
        <thead>
          <tr class="border-y border-gray-100 bg-gray-50/80 text-xs text-gray-500 dark:border-dark-700 dark:bg-dark-900/40 dark:text-dark-400">
            <th scope="col" class="w-12 py-2.5 pl-6 pr-2 text-left font-medium">#</th>
            <th scope="col" class="px-3 py-2.5 text-left font-medium">{{ t(`traffic.subject.${ranking.subject.value}`) }}</th>
            <th
              v-for="column in columns"
              :key="column.key"
              scope="col"
              class="whitespace-nowrap px-3 py-2.5 text-right font-medium last:pr-6"
              :aria-sort="column.sort && ranking.sort.value === column.sort ? (ranking.order.value === 'desc' ? 'descending' : 'ascending') : undefined"
            >
              <button
                v-if="column.sort"
                type="button"
                class="inline-flex items-center gap-1 rounded transition-colors hover:text-gray-900 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/40 dark:hover:text-white"
                :class="ranking.sort.value === column.sort ? 'text-primary-600 dark:text-primary-400' : ''"
                :data-testid="`sort-${column.key}`"
                @click="ranking.setSort(column.sort)"
              >
                {{ column.label }}
                <Icon
                  v-if="ranking.sort.value === column.sort"
                  :name="ranking.order.value === 'desc' ? 'arrowDown' : 'arrowUp'"
                  size="xs"
                  class="h-3 w-3"
                />
              </button>
              <span v-else class="inline-flex items-center gap-1">
                {{ column.label }}
                <HelpTooltip v-if="column.hint" :content="column.hint" />
              </span>
            </th>
          </tr>
        </thead>
        <tbody class="divide-y divide-gray-100 dark:divide-dark-700/70">
          <template v-if="initialLoading">
            <tr v-for="index in 5" :key="`skeleton-${index}`">
              <td :colspan="columns.length + 2" class="px-6 py-3">
                <div class="h-8 animate-pulse rounded-lg bg-gray-100 dark:bg-dark-700/50"></div>
              </td>
            </tr>
          </template>
          <tr v-else-if="!rows.length">
            <td :colspan="columns.length + 2" class="px-6 py-10 text-center" data-testid="ranking-empty">
              <p class="text-sm font-medium text-gray-500 dark:text-dark-300">{{ emptyText }}</p>
              <p v-if="!ranking.query.value" class="mt-1 text-xs text-gray-400 dark:text-dark-500">{{ t('traffic.ranking.emptyHint') }}</p>
            </td>
          </tr>
          <tr
            v-for="row in rows"
            v-else
            :key="row.key"
            class="group cursor-pointer transition-colors hover:bg-gray-50 dark:hover:bg-dark-700/40"
            :data-testid="`ranking-row-${row.key}`"
            @click="emit('select', row.stats)"
          >
            <td class="py-3 pl-6 pr-2 align-top tabular-nums">
              <span
                class="inline-flex h-6 min-w-6 items-center justify-center rounded-full px-1.5 text-xs font-semibold"
                :class="row.rank <= 3 && ranking.order.value === 'desc' ? 'bg-primary-50 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300' : 'text-gray-400 dark:text-dark-500'"
              >
                {{ row.rank }}
              </span>
            </td>
            <td class="max-w-[18rem] px-3 py-3 align-top">
              <button
                type="button"
                class="block w-full rounded text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/40"
                :aria-label="t('traffic.ranking.openDetail', { name: row.title })"
                @click.stop="emit('select', row.stats)"
              >
                <SubjectIdentity :identity="row.stats" :hide-user="userId !== null" :active-keys="row.stats.active_api_keys" />
              </button>
            </td>

            <td v-for="column in columns" :key="column.key" class="whitespace-nowrap px-3 py-3 text-right align-top tabular-nums last:pr-6" :data-testid="`cell-${column.key}`">
              <!-- 流量列 -->
              <template v-if="column.key === 'total'">
                <span class="font-semibold text-gray-900 dark:text-white">{{ row.totalText }}</span>
                <span class="mt-1 flex items-center justify-end gap-2 text-xs text-gray-500 dark:text-dark-400">
                  <span class="h-1.5 w-16 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
                    <span class="block h-full rounded-full bg-primary-500" :style="{ width: row.shareWidth }"></span>
                  </span>
                  <span class="w-11 text-right">{{ row.shareText }}</span>
                </span>
              </template>
              <span v-else-if="column.key === 'in'" class="text-gray-700 dark:text-dark-200">{{ row.inText }}</span>
              <span v-else-if="column.key === 'out'" class="text-gray-700 dark:text-dark-200">{{ row.outText }}</span>
              <span v-else-if="column.key === 'requests'" class="text-gray-700 dark:text-dark-200">{{ row.requestsText }}</span>
              <span v-else-if="column.key === 'perRequest'" class="text-gray-700 dark:text-dark-200">{{ row.perRequestText }}</span>
              <template v-else-if="column.key === 'growth'">
                <span class="inline-flex items-center gap-1 font-medium" :class="growthClass(row.growth.direction)">
                  <Icon v-if="row.growth.direction === 'up'" name="arrowUp" size="xs" class="h-3 w-3" />
                  <Icon v-else-if="row.growth.direction === 'down'" name="arrowDown" size="xs" class="h-3 w-3" />
                  {{ row.growth.bytesText }}
                </span>
                <span class="mt-0.5 block text-xs text-gray-400 dark:text-dark-500">
                  {{ row.growth.direction === 'new' ? t('traffic.ranking.growthNew') : row.growth.percentText }}
                </span>
              </template>

              <!-- 带宽列 -->
              <template v-else-if="column.key === 'peakTotal' || column.key === 'peakIn' || column.key === 'peakOut'">
                <template v-if="row.peaks[column.key]">
                  <span :class="column.key === 'peakTotal' ? 'font-semibold text-gray-900 dark:text-white' : 'text-gray-700 dark:text-dark-200'">
                    {{ row.peaks[column.key]!.mbps }}
                  </span>
                  <span class="mt-0.5 block text-xs text-gray-400 dark:text-dark-500" :title="row.peaks[column.key]!.at">
                    {{ column.key === 'peakTotal' ? row.peaks[column.key]!.at : row.peaks[column.key]!.rate }}
                  </span>
                </template>
                <span v-else class="text-gray-400 dark:text-dark-500">{{ t('traffic.ranking.peakUnknown') }}</span>
              </template>
              <template v-else-if="column.key === 'activeAvg'">
                <span class="text-gray-700 dark:text-dark-200">{{ row.activeAvgText }}</span>
                <span class="mt-0.5 block text-xs text-gray-400 dark:text-dark-500">{{ row.activeDurationText }}</span>
              </template>
              <span v-else-if="column.key === 'average'" class="text-gray-700 dark:text-dark-200">{{ row.averageText }}</span>
              <template v-else-if="column.key === 'usage'">
                <span v-if="row.usage === null" class="text-gray-400 dark:text-dark-500">—</span>
                <span v-else class="inline-flex items-center gap-2">
                  <span class="h-1.5 w-14 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
                    <span class="block h-full rounded-full" :class="usageFillClass(row.usage)" :style="{ width: `${Math.min(row.usage, 1) * 100}%` }"></span>
                  </span>
                  <span class="w-11 text-right font-medium" :class="usageTextClass(row.usage)">{{ formatShare(row.usage) }}</span>
                </span>
              </template>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <Pagination
      v-if="data && data.total > ranking.pageSize"
      class="mt-2 rounded-b-3xl"
      :class="embedded ? '' : '-mx-6 -mb-6'"
      :total="data.total"
      :page="ranking.page.value"
      :page-size="ranking.pageSize"
      :show-page-size-selector="false"
      @update:page="ranking.page.value = $event"
    />
  </section>
</template>

<script setup lang="ts">
import { computed, ref, toRef } from 'vue'
import { useI18n } from 'vue-i18n'
import { saveAs } from 'file-saver'
import Icon from '@/components/icons/Icon.vue'
import HelpTooltip from '@/components/common/HelpTooltip.vue'
import Pagination from '@/components/common/Pagination.vue'
import SearchInput from '@/components/common/SearchInput.vue'
import type { TrafficRankingSort, TrafficSubjectStats } from '@/api/admin/trafficSubjects'
import type { TrafficPeak } from '@/api/admin/wireguard'
import { useAppStore } from '@/stores/app'
import { headroomLevel } from '@/utils/bandwidthSeries'
import { formatByteCount, formatByteRate, formatMbps, formatSampleTime } from '@/utils/trafficFormat'
import SubjectIdentity from './SubjectIdentity.vue'
import SubjectTypeSwitch from './SubjectTypeSwitch.vue'
import { fetchRankingForExport, rankingCsv, rankingFileName } from './rankingExport'
import {
  averageRate,
  formatCount,
  formatShare,
  growthView,
  lateAttributionStart,
  parseSignedBytes,
  peakUsage,
  subjectKey,
  subjectTitle,
  type GrowthView
} from './subjectFormat'
import type { PresetRange } from './usePresetRange'
import { useSubjectRanking } from './useSubjectRanking'

type Mode = 'traffic' | 'bandwidth'
type PeakColumn = 'peakTotal' | 'peakIn' | 'peakOut'

interface Column {
  key: string
  label: string
  sort?: TrafficRankingSort
  hint?: string
}

interface PeakView {
  mbps: string
  rate: string
  at: string
}

const props = withDefaults(
  defineProps<{
    mode: Mode
    range: PresetRange
    limitMbps?: number | null
    // 只看某个用户名下的 Key，用于用户详情。
    userId?: number | null
    embedded?: boolean
  }>(),
  { limitMbps: null, userId: null, embedded: false }
)

const emit = defineEmits<{ select: [subject: TrafficSubjectStats] }>()

const { t } = useI18n()
const app = useAppStore()

const ranking = useSubjectRanking({
  range: toRef(props, 'range'),
  sort: props.mode === 'traffic' ? 'total_bytes' : 'peak_total',
  userId: props.userId !== null ? toRef(props, 'userId') : undefined,
  poll: !props.embedded
})

const searchText = ref('')
const exporting = ref(false)

const data = computed(() => ranking.data.value)
const items = computed(() => data.value?.items ?? [])
const initialLoading = computed(() => ranking.loading.value && !data.value)

const searchPlaceholder = computed(() =>
  t(ranking.subject.value === 'user' ? 'traffic.ranking.searchUser' : 'traffic.ranking.searchKey')
)

const columns = computed<Column[]>(() => {
  if (props.mode === 'traffic') {
    const list: Column[] = [
      { key: 'total', label: t('traffic.ranking.columns.total'), sort: 'total_bytes' },
      { key: 'in', label: t('traffic.ranking.columns.in'), sort: 'in_bytes' },
      { key: 'out', label: t('traffic.ranking.columns.out'), sort: 'out_bytes' },
      { key: 'requests', label: t('traffic.ranking.columns.requests'), sort: 'requests' },
      { key: 'perRequest', label: t('traffic.ranking.columns.perRequest'), sort: 'bytes_per_request' }
    ]
    if (ranking.compare.value) list.push({ key: 'growth', label: t('traffic.ranking.columns.growth'), sort: 'growth' })
    return list
  }
  return [
    { key: 'peakTotal', label: t('traffic.ranking.columns.peakTotal'), sort: 'peak_total' },
    { key: 'peakIn', label: t('traffic.ranking.columns.peakIn'), sort: 'peak_in' },
    { key: 'peakOut', label: t('traffic.ranking.columns.peakOut'), sort: 'peak_out' },
    { key: 'activeAvg', label: t('traffic.ranking.columns.activeAvg'), sort: 'active_avg_rate' },
    // 期间平均速率的分母对所有主体相同，与按总流量排序一致。
    { key: 'average', label: t('traffic.ranking.columns.average'), sort: 'total_bytes' },
    { key: 'usage', label: t('traffic.ranking.columns.usage'), hint: t('traffic.ranking.usageHint') }
  ]
})

function peakView(peak: TrafficPeak | undefined): PeakView | null {
  if (!peak) return null
  return {
    mbps: formatMbps(peak.bytes_per_second),
    rate: formatByteRate(peak.bytes_per_second),
    at: formatSampleTime(peak.at)
  }
}

function durationText(seconds: number) {
  if (seconds < 60) return t('traffic.duration.seconds', { count: seconds })
  if (seconds < 3600) return t('traffic.duration.minutes', { count: Math.round(seconds / 60) })
  return t('traffic.duration.hours', { count: parseFloat((seconds / 3600).toFixed(1)) })
}

const maxShare = computed(() => Math.max(0, ...items.value.map((item) => item.share ?? 0)))

const rows = computed(() =>
  items.value.map((stats, index) => {
    const share = stats.share
    const perRequest = parseSignedBytes(stats.bytes_per_request)
    const average = averageRate(stats, data.value?.window)
    return {
      key: subjectKey(stats),
      rank: (ranking.page.value - 1) * ranking.pageSize + index + 1,
      stats,
      title: subjectTitle(stats),
      totalText: formatByteCount(stats.total_bytes),
      shareText: formatShare(share),
      // 占比条按本页最大值缩放，便于比较本页各行。
      shareWidth: share && maxShare.value > 0 ? `${Math.max((share / maxShare.value) * 100, 2)}%` : '0%',
      inText: formatByteCount(stats.in_bytes),
      outText: formatByteCount(stats.out_bytes),
      requestsText: formatCount(stats.requests),
      perRequestText: perRequest === null ? '—' : formatByteCount(perRequest),
      growth: growthView(stats),
      peaks: {
        peakTotal: peakView(stats.peaks?.total),
        peakIn: peakView(stats.peaks?.in),
        peakOut: peakView(stats.peaks?.out)
      } as Record<PeakColumn, PeakView | null>,
      activeAvgText:
        stats.active_avg_bytes_per_second === null ? '—' : formatMbps(stats.active_avg_bytes_per_second),
      activeDurationText:
        stats.active_seconds > 0 ? t('traffic.ranking.activeFor', { duration: durationText(stats.active_seconds) }) : '',
      averageText: average === null ? '—' : formatMbps(average),
      usage: peakUsage(stats.peaks, props.limitMbps)
    }
  })
)

const concentrationText = computed(() => {
  const value = data.value?.concentration
  if (!value || !value.subjects) return '—'
  const parts = [t('traffic.ranking.top', { count: 1, share: formatShare(value.top1) })]
  if (value.subjects > 1) parts.push(t('traffic.ranking.top', { count: 5, share: formatShare(value.top5) }))
  if (value.subjects > 5) parts.push(t('traffic.ranking.top', { count: 10, share: formatShare(value.top10) }))
  return parts.join(' · ')
})

const unattributedShare = computed(() => {
  const totals = data.value?.totals
  const part = parseSignedBytes(totals?.unattributed.total_bytes)
  const all = parseSignedBytes(totals?.all_total_bytes)
  if (part === null || all === null || all <= 0n) return ''
  return formatShare(Number(part) / Number(all))
})

const coverageText = computed(() => {
  const late = lateAttributionStart(data.value?.window)
  return late ? t('traffic.ranking.coverageFrom', { time: formatSampleTime(late) }) : t('traffic.ranking.coverageFull')
})

const notices = computed(() => {
  const value = data.value
  const list: { key: string; text: string }[] = []
  if (!value) return list
  const window = value.window
  if (window.subject_overflow) list.push({ key: 'overflow', text: t('traffic.ranking.notices.overflow') })
  if (props.mode === 'bandwidth' && window.concurrent_runtimes) {
    list.push({ key: 'concurrent', text: t('traffic.ranking.notices.concurrent') })
  }
  if (window.incomplete) list.push({ key: 'incomplete', text: t('traffic.ranking.notices.incomplete') })
  if (ranking.compare.value) {
    list.push({ key: 'partial', text: t('traffic.ranking.notices.partialHour') })
    if (value.previous_window && !value.previous_window.complete) {
      list.push({ key: 'previous', text: t('traffic.ranking.notices.previousIncomplete') })
    }
  }
  if (props.mode === 'bandwidth' && !props.limitMbps) {
    list.push({ key: 'limit', text: t('traffic.ranking.notices.noLimit') })
  }
  return list
})

const emptyText = computed(() => {
  if (ranking.query.value) return t('traffic.ranking.noMatch')
  return t(ranking.subject.value === 'user' ? 'traffic.ranking.emptyUser' : 'traffic.ranking.emptyKey')
})

function growthClass(direction: GrowthView['direction']) {
  if (direction === 'up' || direction === 'new') return 'text-amber-600 dark:text-amber-400'
  if (direction === 'down') return 'text-emerald-600 dark:text-emerald-400'
  return 'text-gray-500 dark:text-dark-400'
}

function usageFillClass(ratio: number) {
  const level = headroomLevel(ratio)
  return level === 'critical' ? 'bg-red-500' : level === 'warning' ? 'bg-amber-500' : 'bg-primary-500'
}

function usageTextClass(ratio: number) {
  const level = headroomLevel(ratio)
  return level === 'critical'
    ? 'text-red-600 dark:text-red-400'
    : level === 'warning'
      ? 'text-amber-600 dark:text-amber-400'
      : 'text-gray-700 dark:text-dark-200'
}

async function exportCsv() {
  if (exporting.value) return
  exporting.value = true
  try {
    const { items: all, truncated } = await fetchRankingForExport({
      range: props.range,
      subject: ranking.subject.value,
      sort: ranking.sort.value,
      order: ranking.order.value,
      q: ranking.query.value,
      user_id: props.userId ?? undefined,
      compare: ranking.compare.value
    })
    const csv = rankingCsv(all, t)
    saveAs(new Blob(['\ufeff', csv], { type: 'text/csv;charset=utf-8' }), rankingFileName(ranking.subject.value, props.range))
    if (truncated) app.showInfo(t('traffic.ranking.exportTruncated', { count: all.length }))
  } catch {
    app.showError(t('traffic.ranking.exportFailed'))
  } finally {
    exporting.value = false
  }
}

defineExpose({ refresh: ranking.refresh })
</script>
