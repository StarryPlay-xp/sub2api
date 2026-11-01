<template>
  <div class="mt-6 border-t border-gray-100 pt-5 dark:border-dark-700" data-testid="egress-breakdown">
    <div class="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
      <div>
        <h4 class="text-sm font-medium text-gray-700 dark:text-dark-100">{{ t('traffic.egress.title') }}</h4>
        <p class="mt-0.5 text-[11px] text-gray-400 dark:text-dark-500">{{ t('traffic.egress.subtitle') }}</p>
      </div>
      <span v-if="data" class="text-xs tabular-nums text-gray-500 dark:text-dark-400" data-testid="egress-total">
        {{ t('traffic.egress.total', { total: formatByteCount(data.totals.upstream.total_bytes) }) }}
      </span>
    </div>

    <p v-if="error" class="mt-3 text-xs text-gray-500 dark:text-dark-400" data-testid="egress-error">{{ error }}</p>
    <div v-else-if="!data" class="mt-3 h-16 animate-pulse rounded-xl bg-gray-100 dark:bg-dark-700/50"></div>

    <template v-else>
      <ul v-if="notices.length" class="mt-3 space-y-1 text-xs text-amber-700 dark:text-amber-400" data-testid="egress-notices">
        <li v-for="notice in notices" :key="notice" class="flex items-start gap-1.5">
          <Icon name="exclamationTriangle" size="xs" class="mt-px h-3.5 w-3.5 shrink-0" />
          <span>{{ notice }}</span>
        </li>
      </ul>

      <p v-if="!rows.length && !unidentified" class="mt-3 text-xs text-gray-500 dark:text-dark-400" data-testid="egress-empty">
        {{ t('traffic.egress.empty') }}
      </p>

      <ul v-else class="mt-3 space-y-1">
        <li v-for="row in rows" :key="row.key">
          <button
            type="button"
            class="grid w-full grid-cols-[minmax(0,1.1fr)_minmax(0,2fr)_6.5rem] items-center gap-x-4 rounded-xl px-2 py-2 text-left transition-colors hover:bg-gray-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/40 dark:hover:bg-dark-700/40"
            :data-testid="`egress-${row.key}`"
            @click="emit('open', row.route)"
          >
            <span class="min-w-0">
              <span class="flex min-w-0 items-center gap-1.5">
                <span class="badge shrink-0 text-[10px]" :class="kindClass(row.route.route_type)">{{ routeKindLabel(row.route.route_type, t) }}</span>
                <span
                  class="truncate text-sm font-medium"
                  :class="row.route.route_deleted ? 'text-gray-500 line-through dark:text-dark-400' : 'text-gray-900 dark:text-white'"
                  :title="row.title"
                >
                  {{ row.title }}
                </span>
              </span>
              <span class="mt-0.5 block truncate text-[11px] text-gray-500 dark:text-dark-400">{{ row.meta }}</span>
            </span>
            <span class="min-w-0">
              <span class="flex h-2 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700" aria-hidden="true">
                <span class="h-full" :style="{ width: row.outWidth, backgroundColor: palette.out }"></span>
                <span class="h-full" :style="{ width: row.inWidth, backgroundColor: palette.in }"></span>
              </span>
              <span class="mt-1 block truncate text-[11px] tabular-nums text-gray-500 dark:text-dark-400">{{ row.split }}</span>
            </span>
            <span class="text-right tabular-nums">
              <span class="block text-sm font-semibold text-gray-900 dark:text-white">{{ row.total }}</span>
              <span class="block text-[11px] text-gray-500 dark:text-dark-400">{{ row.share }}</span>
            </span>
          </button>
        </li>
        <li v-if="unidentified" class="grid grid-cols-[minmax(0,1.1fr)_minmax(0,2fr)_6.5rem] items-center gap-x-4 px-2 py-2" data-testid="egress-unidentified">
          <span class="flex items-center gap-1 text-sm text-gray-500 dark:text-dark-400">
            {{ t('traffic.egress.unidentified') }}
            <HelpTooltip :content="t('traffic.egress.unidentifiedHint')" width-class="w-72" />
          </span>
          <span class="min-w-0">
            <span class="flex h-2 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700" aria-hidden="true">
              <span class="h-full bg-gray-300 dark:bg-dark-500" :style="{ width: unidentified.width }"></span>
            </span>
            <span class="mt-1 block truncate text-[11px] tabular-nums text-gray-500 dark:text-dark-400">{{ unidentified.split }}</span>
          </span>
          <span class="text-right tabular-nums">
            <span class="block text-sm font-semibold text-gray-700 dark:text-dark-200">{{ unidentified.total }}</span>
            <span class="block text-[11px] text-gray-500 dark:text-dark-400">{{ unidentified.share }}</span>
          </span>
        </li>
      </ul>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import HelpTooltip from '@/components/common/HelpTooltip.vue'
import Icon from '@/components/icons/Icon.vue'
import type { EgressRouteStats, EgressRouteType, TrafficEgressData } from '@/api/admin/trafficEgress'
import { formatByteCount, formatSampleTime, isMissingTime } from '@/utils/trafficFormat'
import { useBandwidthPalette } from '@/views/admin/bandwidth/useBandwidthPalette'
import { routeKey, routeKindLabel, routeTitle } from '@/components/admin/traffic/egressFormat'
import { formatCount, formatShare, parseSignedBytes } from '@/components/admin/traffic/subjectFormat'

const props = defineProps<{
  data: TrafficEgressData | null
  error: string
}>()

const emit = defineEmits<{ open: [route: EgressRouteStats] }>()

const { t } = useI18n()
const palette = useBandwidthPalette()

const allTotal = computed(() => parseSignedBytes(props.data?.totals.upstream.total_bytes) ?? 0n)

// 条长按最大出口（含未识别出口）缩放，上游请求与响应分色堆叠，配色与流量路径的箭头一致。
const maxTotal = computed(() => {
  const values = [
    ...(props.data?.routes ?? []).map((route) => parseSignedBytes(route.total_bytes) ?? 0n),
    parseSignedBytes(props.data?.totals.unidentified.total_bytes) ?? 0n
  ]
  return values.reduce((max, value) => (value > max ? value : max), 0n)
})

function width(value: bigint | null) {
  return value !== null && maxTotal.value > 0n ? `${Number((value * 10000n) / maxTotal.value) / 100}%` : '0%'
}

function shareText(value: bigint | null) {
  return value !== null && allTotal.value > 0n ? formatShare(Number(value) / Number(allTotal.value)) : ''
}

function splitText(out: string, inbound: string) {
  return t('traffic.egress.split', { out: formatByteCount(out), in: formatByteCount(inbound) })
}

const rows = computed(() =>
  (props.data?.routes ?? []).map((route) => {
    const out = parseSignedBytes(route.bytes.upstream_out)
    const inbound = parseSignedBytes(route.bytes.upstream_in)
    const total = parseSignedBytes(route.total_bytes)
    const meta = [t('traffic.egress.attempts', { count: formatCount(route.attempts) })]
    if (route.account_count > 0) meta.push(t('traffic.egress.accounts', { count: route.account_count }))
    if (route.route_deleted) meta.push(t('traffic.egress.deleted'))
    return {
      key: routeKey(route),
      route,
      title: routeTitle(route, t),
      meta: meta.join(' · '),
      split: splitText(route.bytes.upstream_out, route.bytes.upstream_in),
      outWidth: width(out),
      inWidth: width(inbound),
      total: formatByteCount(route.total_bytes),
      share: total !== null && total > 0n ? shareText(total) : t('traffic.egress.noBytes')
    }
  })
)

const unidentified = computed(() => {
  const value = props.data?.totals.unidentified
  const total = parseSignedBytes(value?.total_bytes)
  if (!value || total === null || total <= 0n) return null
  return {
    total: formatByteCount(total),
    share: shareText(total),
    split: splitText(value.bytes.upstream_out, value.bytes.upstream_in),
    width: width(total)
  }
})

const notices = computed(() => {
  const window = props.data?.window
  const list: string[] = []
  if (!window) return list
  const started = window.egress_started_at
  if (!started || isMissingTime(started)) list.push(t('traffic.egress.notStarted'))
  else if (Date.parse(started) - Date.parse(window.start) > 60_000) {
    list.push(t('traffic.egress.coverageFrom', { time: formatSampleTime(started) }))
  }
  if (window.overflow) list.push(t('traffic.egress.overflow'))
  if (window.concurrent_runtimes) list.push(t('traffic.ranking.notices.concurrent'))
  else if (window.incomplete) list.push(t('traffic.ranking.notices.incomplete'))
  return list
})

function kindClass(type: EgressRouteType) {
  if (type === 'wireguard') return 'badge-primary'
  if (type === 'proxy') return 'badge-gray'
  return 'badge-success'
}
</script>
