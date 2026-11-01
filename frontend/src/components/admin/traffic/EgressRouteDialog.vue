<template>
  <BaseDialog :show="true" :title="title" width="extra-wide" @close="emit('close')">
    <div class="space-y-6" data-testid="egress-detail">
      <div class="flex min-w-0 items-start gap-3">
        <span
          class="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-primary-50 text-primary-600 dark:bg-primary-900/25 dark:text-primary-300"
        >
          <Icon :name="route.route_type === 'direct' ? 'globe' : 'link'" size="md" />
        </span>
        <div class="min-w-0">
          <p class="text-[11px] font-medium uppercase tracking-wide text-gray-400 dark:text-dark-500">
            <template v-if="route.route_type === 'direct'">{{ t('traffic.egress.kindLabel') }}</template>
            <template v-else>{{ routeKindLabel(route.route_type, t) }}<template v-if="route.protocol"> · {{ route.protocol }}</template></template>
          </p>
          <p class="flex min-w-0 items-center gap-1.5">
            <span class="truncate text-base font-medium" :class="route.route_deleted ? 'text-gray-500 line-through dark:text-dark-400' : 'text-gray-900 dark:text-white'">
              {{ title }}
            </span>
            <span v-if="route.route_deleted" class="badge badge-gray shrink-0 text-[10px]">{{ t('traffic.egress.deleted') }}</span>
          </p>
        </div>
      </div>

      <p v-if="windowText" class="text-xs text-gray-500 dark:text-dark-400" data-testid="egress-detail-window">{{ windowText }}</p>

      <EgressServiceSection :data="detail.data.value" :error="detail.error.value" />

      <section v-if="detail.data.value" class="rounded-2xl border border-gray-100 p-4 dark:border-dark-700">
        <h4 class="mb-2 text-sm font-medium text-gray-700 dark:text-dark-100">{{ t('traffic.egress.trend') }}</h4>
        <TrafficTrendChart
          :samples="samples"
          :span="span"
          :grain="HOUR_MS"
          :in-label="t('traffic.egress.inLabel')"
          :out-label="t('traffic.egress.outLabel')"
          :peak-label="t('traffic.egress.peakLine')"
          hint-key="traffic.egress.trendHint"
          test-id="egress-trend"
        />
      </section>
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, toRef } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import type { EgressRouteIdentity } from '@/api/admin/trafficEgress'
import { formatDateTimeLabel } from '@/utils/wireguardTraffic'
import { HOUR_MS } from '@/utils/wireguardTrafficChart'
import EgressServiceSection from './EgressServiceSection.vue'
import TrafficTrendChart from './TrafficTrendChart.vue'
import { routeKindLabel, routeTitle } from './egressFormat'
import { egressTrendSamples } from './trendSamples'
import type { PresetRange } from './usePresetRange'
import { useEgressRouteDetail } from './useEgress'

const props = defineProps<{
  route: EgressRouteIdentity
  range: PresetRange
}>()

const emit = defineEmits<{ close: [] }>()

const { t } = useI18n()

const title = computed(() => routeTitle(props.route, t))
const target = computed(() => ({ type: props.route.route_type, id: props.route.route_id ?? 0 }))
const detail = useEgressRouteDetail(target, toRef(props, 'range'))

const windowText = computed(() => {
  const value = detail.data.value?.window
  return value ? `${formatDateTimeLabel(value.start)} – ${formatDateTimeLabel(value.end)} · ${t('bandwidth.perHour')}` : ''
})

const samples = computed(() => egressTrendSamples(detail.data.value?.history ?? []))
const span = computed(() => {
  const value = detail.data.value?.window
  if (!value) return null
  const start = Date.parse(value.start)
  const end = Date.parse(value.end)
  return Number.isNaN(start) || Number.isNaN(end) || end <= start ? null : { start, end }
})
</script>
