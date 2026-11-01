<template>
  <section
    class="rounded-3xl bg-white p-6 shadow-sm ring-1 ring-gray-900/5 dark:bg-dark-800 dark:ring-dark-700"
    data-testid="relay-path"
  >
    <header>
      <h3 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('traffic.relay.title') }}</h3>
      <p class="mt-0.5 text-xs text-gray-500 dark:text-dark-400">{{ t('traffic.relay.subtitle') }}</p>
    </header>

    <!-- 宽屏：横向链路 -->
    <div class="mt-7 hidden items-center gap-3 md:grid md:grid-cols-[auto_minmax(0,1fr)_auto_minmax(0,1fr)_auto]">
      <template v-for="(item, index) in layout" :key="item.key">
        <div v-if="item.kind === 'node'" class="flex w-28 flex-col items-center gap-2 text-center">
          <span
            class="flex h-14 w-14 items-center justify-center rounded-2xl"
            :class="
              item.emphasis
                ? 'bg-primary-50 text-primary-600 ring-2 ring-primary-500/30 dark:bg-primary-900/25 dark:text-primary-300'
                : 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-dark-300'
            "
          >
            <Icon :name="item.icon" size="md" />
          </span>
          <span class="text-sm font-medium text-gray-800 dark:text-dark-100">{{ item.label }}</span>
          <span v-if="item.note" class="text-[11px] leading-snug text-gray-400 dark:text-dark-500">{{ item.note }}</span>
        </div>
        <div v-else class="space-y-4" :data-testid="`relay-hop-${index}`">
          <div v-for="flow in item.flows" :key="flow.key" :data-testid="`relay-${flow.key}`">
            <div class="flex items-center justify-between gap-2 text-xs">
              <span class="font-medium text-gray-600 dark:text-dark-200">{{ flow.label }}</span>
              <span class="text-gray-400 tabular-nums dark:text-dark-500">{{ flow.peakText }}</span>
            </div>
            <div class="mt-1.5 flex items-center" :class="flow.reverse ? 'flex-row-reverse' : ''">
              <span class="h-0.5 flex-1 rounded-full" :style="{ backgroundColor: flow.color }"></span>
              <svg
                class="h-3 w-3 shrink-0"
                :class="flow.reverse ? 'rotate-180' : ''"
                viewBox="0 0 12 12"
                aria-hidden="true"
              >
                <path d="M1 1.5 L10.5 6 L1 10.5 Z" :fill="flow.color" />
              </svg>
            </div>
            <div class="mt-1.5 text-sm font-semibold text-gray-900 dark:text-white">{{ flow.bytes }}</div>
          </div>
        </div>
      </template>
    </div>

    <!-- 窄屏：纵向链路 -->
    <ol class="mt-6 space-y-3 md:hidden">
      <template v-for="item in layout" :key="item.key">
        <li v-if="item.kind === 'node'" class="flex items-center gap-3">
          <span
            class="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl"
            :class="
              item.emphasis
                ? 'bg-primary-50 text-primary-600 dark:bg-primary-900/25 dark:text-primary-300'
                : 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-dark-300'
            "
          >
            <Icon :name="item.icon" size="sm" />
          </span>
          <span class="text-sm font-medium text-gray-800 dark:text-dark-100">{{ item.label }}</span>
        </li>
        <li v-else class="ml-5 space-y-2 border-l-2 border-gray-100 py-1 pl-6 dark:border-dark-700">
          <div v-for="flow in item.flows" :key="flow.key" class="flex items-baseline justify-between gap-3 text-sm">
            <span class="inline-flex items-center gap-1.5 text-gray-600 dark:text-dark-300">
              <span class="h-0.5 w-3 rounded-full" :style="{ backgroundColor: flow.color }"></span>
              {{ flow.reverse ? '↑' : '↓' }} {{ flow.label }}
            </span>
            <span class="text-right">
              <span class="font-semibold text-gray-900 dark:text-white">{{ flow.bytes }}</span>
              <span class="block text-[11px] text-gray-400 dark:text-dark-500">{{ flow.peakText }}</span>
            </span>
          </div>
        </li>
      </template>
    </ol>

    <!-- 上游这一段按出口拆分：直连、代理与各 WG 节点 -->
    <EgressBreakdown v-if="showEgress" :data="egress ?? null" :error="egressError ?? ''" @open="emit('open-route', $event)" />

    <p class="mt-6 border-t border-gray-100 pt-4 text-xs leading-relaxed text-gray-500 dark:border-dark-700 dark:text-dark-400">
      {{ t('traffic.relay.note') }}
    </p>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import type { ServiceTrafficDirection, ServiceTrafficReport } from '@/api/admin/ops'
import type { EgressRouteStats, TrafficEgressData } from '@/api/admin/trafficEgress'
import { formatByteCount, formatMbps } from '@/utils/trafficFormat'
import { useBandwidthPalette } from '@/views/admin/bandwidth/useBandwidthPalette'
import EgressBreakdown from './EgressBreakdown.vue'

const props = defineProps<{
  report: ServiceTrafficReport | null
  // 不传时不展示出口构成（例如尚未接入出口统计的页面）。
  showEgress?: boolean
  egress?: TrafficEgressData | null
  egressError?: string
}>()

const emit = defineEmits<{ 'open-route': [route: EgressRouteStats] }>()

const { t } = useI18n()
const palette = useBandwidthPalette()

type IconName = 'users' | 'server' | 'cloud'

interface Flow {
  key: ServiceTrafficDirection
  label: string
  bytes: string
  peakText: string
  color: string
  reverse: boolean
}

type LayoutItem =
  | { kind: 'node'; key: string; label: string; icon: IconName; emphasis?: boolean; note?: string }
  | { kind: 'hop'; key: string; flows: Flow[] }

function bytesOf(key: ServiceTrafficDirection) {
  const report = props.report
  return report?.has_records ? formatByteCount(report.bytes?.[key]) : '—'
}

function peakOf(key: ServiceTrafficDirection) {
  const peak = props.report?.peaks?.[key]
  return peak
    ? t('traffic.relay.peak', { value: formatMbps(peak.bytes_per_second) })
    : t('traffic.relay.peakUnknown')
}

// 进入本站的箭头用入站色，离开本站的箭头用出站色，与走势图一致。
function flow(key: ServiceTrafficDirection, label: string, inbound: boolean, reverse: boolean): Flow {
  return {
    key,
    label,
    bytes: bytesOf(key),
    peakText: peakOf(key),
    color: inbound ? palette.value.in : palette.value.out,
    reverse
  }
}

const layout = computed<LayoutItem[]>(() => [
  { kind: 'node', key: 'client', label: t('traffic.relay.client'), icon: 'users' },
  {
    kind: 'hop',
    key: 'client-hop',
    flows: [
      flow('client_in', t('traffic.relay.clientIn'), true, false),
      flow('client_out', t('traffic.relay.clientOut'), false, true)
    ]
  },
  { kind: 'node', key: 'site', label: t('traffic.relay.site'), icon: 'server', emphasis: true },
  {
    kind: 'hop',
    key: 'upstream-hop',
    flows: [
      flow('upstream_out', t('traffic.relay.upstreamOut'), false, false),
      flow('upstream_in', t('traffic.relay.upstreamIn'), true, true)
    ]
  },
  {
    kind: 'node',
    key: 'upstream',
    label: t('traffic.relay.upstream'),
    icon: 'cloud',
    note: t('traffic.relay.upstreamNote')
  }
])
</script>
