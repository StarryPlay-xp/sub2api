<template>
  <section
    class="rounded-3xl bg-white p-6 shadow-sm ring-1 ring-gray-900/5 dark:bg-dark-800 dark:ring-dark-700"
    data-testid="live-subjects"
  >
    <header class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
      <div>
        <h3 class="flex items-center gap-2 text-base font-semibold text-gray-900 dark:text-white">
          {{ t('traffic.live.title') }}
          <span class="inline-flex items-center gap-1.5 text-xs font-medium" :class="statusClass" data-testid="live-status">
            <span class="relative flex h-2 w-2">
              <span
                v-if="live.status.value === 'sampled'"
                class="absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-60 motion-safe:animate-ping"
              ></span>
              <span class="relative inline-flex h-2 w-2 rounded-full" :class="dotClass"></span>
            </span>
            {{ statusText }}
          </span>
        </h3>
        <p class="mt-0.5 text-xs text-gray-500 dark:text-dark-400">{{ t('traffic.live.subtitle') }}</p>
      </div>
      <SubjectTypeSwitch v-model="live.subject.value" />
    </header>

    <p
      v-if="live.error.value"
      role="alert"
      class="mt-4 rounded-2xl bg-red-50 px-4 py-3 text-sm text-red-700 ring-1 ring-red-200/70 dark:bg-red-950/30 dark:text-red-300 dark:ring-red-900/40"
    >
      {{ live.error.value }}
    </p>

    <div v-else-if="!data" class="mt-5 space-y-3" data-testid="live-skeleton">
      <div v-for="index in 4" :key="index" class="h-10 animate-pulse rounded-xl bg-gray-100 dark:bg-dark-700/50"></div>
    </div>

    <template v-else>
      <div
        v-if="!rows.length"
        class="mt-5 rounded-2xl border border-dashed border-gray-200 px-4 py-8 text-center text-sm text-gray-500 dark:border-dark-700 dark:text-dark-300"
        data-testid="live-empty"
      >
        {{ t(live.subject.value === 'user' ? 'traffic.live.emptyUser' : 'traffic.live.emptyKey') }}
      </div>

      <ol v-else class="mt-5 space-y-1" :class="{ 'opacity-60': live.status.value === 'stale' }">
        <li v-for="row in rows" :key="row.key">
          <button
            type="button"
            class="grid w-full grid-cols-[1.75rem_minmax(0,1fr)_auto] items-center gap-x-3 rounded-xl px-2 py-2 text-left transition-colors hover:bg-gray-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/40 dark:hover:bg-dark-700/40"
            :data-testid="`live-row-${row.key}`"
            @click="emit('select', row.item)"
          >
            <span class="text-center text-xs font-semibold tabular-nums text-gray-400 dark:text-dark-500">{{ row.rank }}</span>
            <span class="min-w-0">
              <SubjectIdentity :identity="row.item" />
              <span class="mt-1.5 flex h-1.5 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700" aria-hidden="true">
                <span class="h-full" :style="{ width: row.inWidth, backgroundColor: palette.in }"></span>
                <span class="h-full" :style="{ width: row.outWidth, backgroundColor: palette.out }"></span>
              </span>
            </span>
            <span class="text-right tabular-nums">
              <span class="block text-sm font-semibold text-gray-900 dark:text-white">{{ row.averageText }}</span>
              <span class="block text-[11px] text-gray-500 dark:text-dark-400">{{ row.detailText }}</span>
            </span>
          </button>
        </li>
      </ol>

      <!-- 本机合计与未归属 -->
      <dl class="mt-4 flex flex-wrap gap-x-6 gap-y-1 border-t border-gray-100 pt-3 text-xs text-gray-500 dark:border-dark-700 dark:text-dark-400">
        <div class="flex gap-1.5" data-testid="live-host">
          <dt>{{ t('traffic.live.host') }}</dt>
          <dd class="font-medium tabular-nums text-gray-700 dark:text-dark-200">{{ formatMbps(data.host.avg_60s.total) }}</dd>
        </div>
        <div class="flex gap-1.5" data-testid="live-unattributed">
          <dt>{{ t('traffic.live.unattributed') }}</dt>
          <dd class="font-medium tabular-nums text-gray-700 dark:text-dark-200">{{ formatMbps(data.unattributed.avg_60s.total) }}</dd>
        </div>
        <div v-if="sampledText" class="flex gap-1.5">
          <dt>{{ t('traffic.live.sampledAt') }}</dt>
          <dd class="tabular-nums">{{ sampledText }}</dd>
        </div>
      </dl>
    </template>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { TrafficLiveItem } from '@/api/admin/trafficSubjects'
import { formatMbps, formatSampleTime, isMissingTime } from '@/utils/trafficFormat'
import { useBandwidthPalette } from '@/views/admin/bandwidth/useBandwidthPalette'
import SubjectIdentity from './SubjectIdentity.vue'
import SubjectTypeSwitch from './SubjectTypeSwitch.vue'
import { formatShare, subjectKey } from './subjectFormat'
import { useTrafficLive } from './useTrafficLive'

const emit = defineEmits<{ select: [subject: TrafficLiveItem] }>()

const { t } = useI18n()
const palette = useBandwidthPalette()
const live = useTrafficLive()

const data = computed(() => live.data.value)

const rows = computed(() => {
  const value = data.value
  if (!value) return []
  // 条长按本列表最大值缩放；括号内为占本机近 60 秒合计的比例。
  const max = Math.max(0, ...value.items.map((item) => item.avg_60s.total))
  const host = value.host.avg_60s.total
  return value.items.map((item, index) => {
    const scale = max > 0 ? 100 / max : 0
    const parts = [t('traffic.live.now', { value: formatMbps(item.rate.total) })]
    if (item.in_flight > 0) parts.push(t('traffic.live.inFlight', { count: item.in_flight }))
    if (host > 0) parts.push(formatShare(item.avg_60s.total / host))
    return {
      key: subjectKey(item),
      rank: index + 1,
      item,
      averageText: formatMbps(item.avg_60s.total),
      detailText: parts.join(' · '),
      inWidth: `${item.avg_60s.in * scale}%`,
      outWidth: `${item.avg_60s.out * scale}%`
    }
  })
})

const sampledText = computed(() => {
  const value = data.value?.sampled_at
  return value && !isMissingTime(value) ? formatSampleTime(value) : ''
})

const statusText = computed(() => t(`traffic.live.status.${live.status.value}`))

const statusClass = computed(() => {
  switch (live.status.value) {
    case 'sampled':
      return 'text-emerald-600 dark:text-emerald-400'
    case 'stale':
    case 'invalid_interval':
      return 'text-amber-600 dark:text-amber-400'
    default:
      return 'text-gray-400 dark:text-dark-500'
  }
})

const dotClass = computed(() => {
  switch (live.status.value) {
    case 'sampled':
      return 'bg-emerald-500'
    case 'stale':
    case 'invalid_interval':
      return 'bg-amber-500'
    default:
      return 'bg-gray-300 dark:bg-dark-500'
  }
})

defineExpose({ refresh: live.refresh })
</script>
