<template>
  <BaseDialog :show="Boolean(current)" :title="dialogTitle" width="extra-wide" @close="close">
    <div v-if="current" class="space-y-6" data-testid="subject-detail">
      <!-- 主体信息与跳转 -->
      <div class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div class="flex min-w-0 items-start gap-3">
          <button
            v-if="history.length"
            type="button"
            class="mt-0.5 rounded-lg p-1.5 text-gray-400 hover:bg-gray-100 hover:text-gray-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/40 dark:hover:bg-dark-700 dark:hover:text-dark-100"
            :aria-label="t('traffic.detail.back')"
            data-testid="detail-back"
            @click="back"
          >
            <Icon name="arrowLeft" size="sm" />
          </button>
          <span
            class="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-primary-50 text-primary-600 dark:bg-primary-900/25 dark:text-primary-300"
          >
            <Icon :name="current.subject_type === 'user' ? 'user' : 'key'" size="md" />
          </span>
          <div class="min-w-0">
            <p class="text-[11px] font-medium uppercase tracking-wide text-gray-400 dark:text-dark-500">
              {{ t(`traffic.subject.${current.subject_type}`) }}
            </p>
            <SubjectIdentity :identity="current" title-class="text-base" />
          </div>
        </div>
        <div class="flex flex-wrap items-center gap-2">
          <button
            v-if="current.subject_type === 'api_key'"
            type="button"
            class="btn btn-secondary btn-sm"
            data-testid="detail-owner"
            @click="openOwner"
          >
            <Icon name="user" size="xs" class="h-3.5 w-3.5" />
            {{ t('traffic.detail.viewOwner') }}
          </button>
          <RouterLink :to="usageLink" class="btn btn-secondary btn-sm" data-testid="detail-usage-link" @click="close">
            <Icon name="document" size="xs" class="h-3.5 w-3.5" />
            {{ t('traffic.detail.viewUsage') }}
          </RouterLink>
        </div>
      </div>

      <p class="text-xs text-gray-500 dark:text-dark-400" data-testid="detail-window">
        {{ windowText }}
        <span v-if="lateStart" class="ml-2 text-amber-600 dark:text-amber-400">
          {{ t('traffic.ranking.coverageFrom', { time: formatSampleTime(lateStart) }) }}
        </span>
      </p>

      <p
        v-if="detail.error.value"
        role="alert"
        class="rounded-2xl bg-red-50 px-4 py-3 text-sm text-red-700 ring-1 ring-red-200/70 dark:bg-red-950/30 dark:text-red-300 dark:ring-red-900/40"
      >
        {{ detail.error.value }}
      </p>

      <div v-else-if="!summary" class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3" data-testid="detail-skeleton">
        <div v-for="index in 6" :key="index" class="h-20 animate-pulse rounded-2xl bg-gray-100 dark:bg-dark-700/50"></div>
      </div>

      <template v-else>
        <!-- 期间汇总 -->
        <dl class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          <div v-for="tile in tiles" :key="tile.key" class="rounded-2xl bg-gray-50 px-4 py-3 dark:bg-dark-900/40" :data-testid="`detail-${tile.key}`">
            <dt class="text-xs font-medium text-gray-500 dark:text-dark-400">{{ tile.label }}</dt>
            <dd class="mt-1 text-xl font-semibold tabular-nums text-gray-900 dark:text-white" :class="tile.valueClass">{{ tile.value }}</dd>
            <dd v-if="tile.sub" class="mt-0.5 text-xs text-gray-500 dark:text-dark-400">{{ tile.sub }}</dd>
          </div>
        </dl>

        <ul v-if="notices.length" class="space-y-1 text-xs text-amber-700 dark:text-amber-400" data-testid="detail-notices">
          <li v-for="notice in notices" :key="notice" class="flex items-start gap-1.5">
            <Icon name="exclamationTriangle" size="xs" class="mt-px h-3.5 w-3.5 shrink-0" />
            <span>{{ notice }}</span>
          </li>
        </ul>

        <!-- 小时趋势 -->
        <section class="rounded-2xl border border-gray-100 p-4 dark:border-dark-700">
          <h4 class="mb-2 text-sm font-medium text-gray-700 dark:text-dark-100">{{ t('traffic.detail.trend') }}</h4>
          <SubjectTrendChart :history="detail.data.value?.history ?? []" :window="detail.data.value?.window ?? null" />
        </section>

        <!-- 四方向构成 -->
        <section class="rounded-2xl border border-gray-100 p-4 dark:border-dark-700" data-testid="detail-directions">
          <div class="flex flex-wrap items-baseline justify-between gap-2">
            <h4 class="text-sm font-medium text-gray-700 dark:text-dark-100">{{ t('traffic.overview.breakdown') }}</h4>
            <span class="text-[11px] text-gray-400 dark:text-dark-500">{{ t('traffic.overview.breakdownHint') }}</span>
          </div>
          <ul class="mt-3 grid gap-x-8 gap-y-3 md:grid-cols-2">
            <li v-for="item in directions" :key="item.key" :data-testid="`detail-direction-${item.key}`">
              <div class="flex items-baseline justify-between gap-3 text-xs">
                <span class="inline-flex items-center gap-1.5 text-gray-600 dark:text-dark-300">
                  <span class="h-0.5 w-3 rounded-full" :style="{ backgroundColor: item.color }"></span>
                  {{ item.label }}
                </span>
                <span class="tabular-nums text-gray-500 dark:text-dark-400">
                  <span class="font-medium text-gray-700 dark:text-dark-200">{{ item.bytesText }}</span>
                  <span v-if="item.percentText" class="ml-1.5">{{ item.percentText }}</span>
                </span>
              </div>
              <div class="mt-1.5 h-1.5 rounded-full bg-gray-100 dark:bg-dark-700">
                <span class="block h-full rounded-full" :style="{ width: item.width, backgroundColor: item.color }"></span>
              </div>
            </li>
          </ul>
        </section>
      </template>

      <!-- 用户名下的 API Key -->
      <section v-if="current.subject_type === 'user'" data-testid="detail-keys">
        <h4 class="mb-3 text-sm font-medium text-gray-700 dark:text-dark-100">{{ t('traffic.detail.keys') }}</h4>
        <SubjectRankingCard
          :key="`keys-${current.user_id}`"
          :mode="mode"
          :range="range"
          :limit-mbps="limitMbps"
          :user-id="current.user_id"
          embedded
          @select="openKey"
        />
      </section>
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, toRef, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import type { ServiceTrafficDirection } from '@/api/admin/ops'
import type { TrafficSubjectIdentity } from '@/api/admin/trafficSubjects'
import { busierPeak } from '@/utils/bandwidthSeries'
import { formatByteCount, formatMbps, formatSampleTime } from '@/utils/trafficFormat'
import { formatDateInput, formatDateTimeLabel } from '@/utils/wireguardTraffic'
import { useBandwidthPalette } from '@/views/admin/bandwidth/useBandwidthPalette'
import SubjectIdentity from './SubjectIdentity.vue'
import SubjectRankingCard from './SubjectRankingCard.vue'
import SubjectTrendChart from './SubjectTrendChart.vue'
import {
  averageRate,
  formatCount,
  formatShare,
  growthView,
  lateAttributionStart,
  parseSignedBytes,
  peakUsage,
  subjectTitle
} from './subjectFormat'
import type { PresetRange } from './usePresetRange'
import { useSubjectDetail } from './useSubjectDetail'

const props = withDefaults(
  defineProps<{
    subject: TrafficSubjectIdentity | null
    range: PresetRange
    mode: 'traffic' | 'bandwidth'
    limitMbps?: number | null
  }>(),
  { limitMbps: null }
)

const emit = defineEmits<{ close: [] }>()

const { t } = useI18n()
const palette = useBandwidthPalette()

// 在详情内从用户进入 Key、从 Key 回到所属用户时保留来路，便于返回。
const current = ref<TrafficSubjectIdentity | null>(props.subject)
const history = ref<TrafficSubjectIdentity[]>([])

watch(
  () => props.subject,
  (value) => {
    current.value = value
    history.value = []
  }
)

const detail = useSubjectDetail(current, toRef(props, 'range'))
const summary = computed(() => detail.data.value?.summary ?? null)
const detailWindow = computed(() => detail.data.value?.window ?? null)

const dialogTitle = computed(() => (current.value ? subjectTitle(current.value) : ''))

function identityOf(value: TrafficSubjectIdentity): TrafficSubjectIdentity {
  return {
    subject_type: value.subject_type,
    user_id: value.user_id,
    user_email: value.user_email,
    username: value.username,
    user_deleted: value.user_deleted,
    api_key_id: value.api_key_id,
    api_key_name: value.api_key_name,
    api_key_deleted: value.api_key_deleted,
    group_id: value.group_id,
    group_name: value.group_name
  }
}

function navigate(next: TrafficSubjectIdentity) {
  if (current.value) history.value = [...history.value, current.value]
  current.value = identityOf(next)
}

function openKey(key: TrafficSubjectIdentity) {
  navigate(key)
}

function openOwner() {
  const value = current.value
  if (!value) return
  navigate({ ...value, subject_type: 'user', api_key_id: null, api_key_name: null, api_key_deleted: null, group_id: null, group_name: null })
}

function back() {
  const previous = history.value[history.value.length - 1]
  if (!previous) return
  history.value = history.value.slice(0, -1)
  current.value = previous
}

function close() {
  emit('close')
}

const windowText = computed(() => {
  const value = detailWindow.value
  if (!value) return ''
  return `${formatDateTimeLabel(value.start)} – ${formatDateTimeLabel(value.end)} · ${t('bandwidth.perHour')}`
})

const lateStart = computed(() => lateAttributionStart(detailWindow.value))

// 使用记录按本地日期筛选，覆盖所选期间的首尾两天。
const usageLink = computed(() => {
  const value = current.value
  const query: Record<string, string> = { user_id: String(value?.user_id ?? '') }
  if (detailWindow.value) {
    query.start_date = formatDateInput(new Date(detailWindow.value.start), 'local')
    query.end_date = formatDateInput(new Date(detailWindow.value.end), 'local')
  }
  return { path: '/admin/usage', query }
})

function durationText(seconds: number) {
  if (seconds < 60) return t('traffic.duration.seconds', { count: seconds })
  if (seconds < 3600) return t('traffic.duration.minutes', { count: Math.round(seconds / 60) })
  return t('traffic.duration.hours', { count: parseFloat((seconds / 3600).toFixed(1)) })
}

interface Tile {
  key: string
  label: string
  value: string
  sub?: string
  valueClass?: string
}

const tiles = computed<Tile[]>(() => {
  const value = summary.value
  if (!value) return []
  const growth = growthView(value)
  const perRequest = parseSignedBytes(value.bytes_per_request)
  const peaks = value.peaks
  const busier = busierPeak(peaks?.in, peaks?.out)
  const usage = peakUsage(peaks, props.limitMbps)
  const list: Tile[] = [
    {
      key: 'total',
      label: t('traffic.detail.total'),
      value: formatByteCount(value.total_bytes),
      sub: t('traffic.overview.inOut', { inbound: formatByteCount(value.in_bytes), outbound: formatByteCount(value.out_bytes) })
    },
    {
      key: 'share',
      label: t('traffic.detail.share'),
      value: formatShare(value.share),
      sub:
        growth.direction === 'unknown'
          ? undefined
          : growth.direction === 'new'
            ? t('traffic.detail.growthNew', { bytes: growth.bytesText })
            : t('traffic.detail.growth', { bytes: growth.bytesText, percent: growth.percentText })
    },
    {
      key: 'requests',
      label: t('traffic.detail.requests'),
      value: formatCount(value.requests),
      sub: perRequest === null ? undefined : t('traffic.detail.perRequest', { bytes: formatByteCount(perRequest) })
    },
    {
      key: 'peak',
      label: t('traffic.detail.peak'),
      value: peaks?.total ? formatMbps(peaks.total.bytes_per_second) : t('traffic.ranking.peakUnknown'),
      sub: peaks?.total
        ? t('traffic.detail.peakSub', {
            inbound: peaks.in ? formatMbps(peaks.in.bytes_per_second) : '—',
            outbound: peaks.out ? formatMbps(peaks.out.bytes_per_second) : '—',
            time: formatSampleTime(peaks.total.at)
          })
        : undefined
    },
    {
      key: 'active',
      label: t('traffic.detail.activeAvg'),
      value: value.active_avg_bytes_per_second === null ? '—' : formatMbps(value.active_avg_bytes_per_second),
      sub: value.active_seconds > 0 ? t('traffic.ranking.activeFor', { duration: durationText(value.active_seconds) }) : undefined
    }
  ]
  // 流量页不读取带宽上限：带宽页展示峰值占上限，流量页展示期间平均速率。
  if (props.mode === 'traffic') {
    const average = averageRate(value, detailWindow.value)
    list.push({
      key: 'average',
      label: t('traffic.ranking.columns.average'),
      value: average === null ? '—' : formatMbps(average),
      sub: t('traffic.detail.averageSub')
    })
  } else list.push({
    key: 'usage',
    label: t('traffic.detail.usage'),
    value: usage === null ? '—' : formatShare(usage),
    sub: !props.limitMbps
      ? t('traffic.detail.usageNoLimit')
      : busier
        ? t('traffic.detail.usageSub', {
            direction: t(`bandwidth.direction.${busier.direction}`),
            value: formatMbps(busier.peak.bytes_per_second),
            limit: `${props.limitMbps} Mbps`
          })
        : undefined
  })
  return list
})

const notices = computed(() => {
  const value = detail.data.value
  const list: string[] = []
  if (!value) return list
  if (value.window.concurrent_runtimes) list.push(t('traffic.ranking.notices.concurrent'))
  if (value.window.incomplete) list.push(t('traffic.ranking.notices.incomplete'))
  list.push(t('traffic.ranking.notices.partialHour'))
  if (value.previous_window && !value.previous_window.complete) list.push(t('traffic.ranking.notices.previousIncomplete'))
  return list
})

const DIRECTIONS: ServiceTrafficDirection[] = ['client_in', 'client_out', 'upstream_out', 'upstream_in']
const DIRECTION_LABELS: Record<ServiceTrafficDirection, string> = {
  client_in: 'traffic.overview.clientIn',
  client_out: 'traffic.overview.clientOut',
  upstream_out: 'traffic.overview.upstreamOut',
  upstream_in: 'traffic.overview.upstreamIn'
}

const directions = computed(() => {
  const value = summary.value
  const total = parseSignedBytes(value?.total_bytes)
  return DIRECTIONS.map((key) => {
    const bytes = parseSignedBytes(value?.bytes?.[key])
    const percent = bytes !== null && total !== null && total > 0n ? Number((bytes * 1000n) / total) / 10 : null
    return {
      key,
      label: t(DIRECTION_LABELS[key]),
      color: key === 'client_in' || key === 'upstream_in' ? palette.value.in : palette.value.out,
      bytesText: bytes === null ? '—' : formatByteCount(bytes),
      percentText: percent === null ? '' : `${parseFloat(percent.toFixed(1))}%`,
      width: percent ? `${Math.max(percent, 0.5)}%` : '0%'
    }
  })
})
</script>
