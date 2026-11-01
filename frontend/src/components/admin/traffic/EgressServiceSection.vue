<template>
  <section class="space-y-4" data-testid="egress-service">
    <div class="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
      <h4 class="text-sm font-medium text-gray-700 dark:text-dark-100">
        {{ t(tunnel ? 'traffic.egress.section.nodeTitle' : 'traffic.egress.section.title') }}
      </h4>
      <span class="text-[11px] text-gray-400 dark:text-dark-500">{{ t('traffic.egress.section.basis') }}</span>
    </div>

    <p v-if="error" class="rounded-2xl bg-gray-50 px-4 py-3 text-xs text-gray-500 dark:bg-dark-900/40 dark:text-dark-400" data-testid="egress-service-error">
      {{ error }}
    </p>
    <div v-else-if="!summary" class="grid gap-3 sm:grid-cols-3" data-testid="egress-service-skeleton">
      <div v-for="index in 3" :key="index" class="h-20 animate-pulse rounded-2xl bg-gray-100 dark:bg-dark-700/50"></div>
    </div>

    <template v-else>
      <dl class="grid gap-3 sm:grid-cols-2" :class="tunnel ? 'lg:grid-cols-3' : 'lg:grid-cols-4'">
        <div v-for="tile in tiles" :key="tile.key" class="rounded-2xl bg-gray-50 px-4 py-3 dark:bg-dark-900/40" :data-testid="`egress-tile-${tile.key}`">
          <dt class="flex items-center gap-1 text-xs font-medium text-gray-500 dark:text-dark-400">
            {{ tile.label }}
            <HelpTooltip v-if="tile.hint" :content="tile.hint" width-class="w-72" />
          </dt>
          <dd class="mt-1 text-xl font-semibold tabular-nums" :class="tile.muted ? 'text-gray-400 dark:text-dark-500' : 'text-gray-900 dark:text-white'">
            {{ tile.value }}
          </dd>
          <dd v-if="tile.sub" class="mt-0.5 text-xs text-gray-500 dark:text-dark-400">{{ tile.sub }}</dd>
        </div>
      </dl>

      <p v-if="previousIncomplete" class="flex items-start gap-1.5 text-xs text-amber-700 dark:text-amber-400">
        <Icon name="exclamationTriangle" size="xs" class="mt-px h-3.5 w-3.5 shrink-0" />
        {{ t('traffic.ranking.notices.previousIncomplete') }}
      </p>

      <!-- 上游账号排行：份额为占该出口流量的比例 -->
      <div class="overflow-x-auto rounded-2xl border border-gray-100 dark:border-dark-700" data-testid="egress-accounts">
        <table class="w-full min-w-max text-sm">
          <thead>
            <tr class="border-b border-gray-100 bg-gray-50/80 text-xs text-gray-500 dark:border-dark-700 dark:bg-dark-900/40 dark:text-dark-400">
              <th scope="col" class="py-2.5 pl-4 pr-3 text-left font-medium">{{ t('traffic.egress.accountsTable.account') }}</th>
              <th scope="col" class="px-3 py-2.5 text-right font-medium">{{ t('traffic.egress.accountsTable.total') }}</th>
              <th scope="col" class="px-3 py-2.5 text-right font-medium">{{ t('traffic.egress.accountsTable.out') }}</th>
              <th scope="col" class="px-3 py-2.5 text-right font-medium">{{ t('traffic.egress.accountsTable.in') }}</th>
              <th scope="col" class="px-3 py-2.5 text-right font-medium">{{ t('traffic.egress.accountsTable.attempts') }}</th>
              <th scope="col" class="py-2.5 pl-3 pr-4 text-right font-medium">{{ t('traffic.egress.accountsTable.peak') }}</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-100 dark:divide-dark-700/70">
            <tr v-if="!accountRows.length && !residual">
              <td colspan="6" class="px-4 py-6 text-center text-xs text-gray-500 dark:text-dark-400" data-testid="egress-accounts-empty">
                {{ t('traffic.egress.accountsTable.empty') }}
              </td>
            </tr>
            <tr v-for="row in accountRows" :key="row.id" :data-testid="`egress-account-${row.id}`">
              <td class="max-w-[16rem] py-2.5 pl-4 pr-3">
                <span class="flex min-w-0 items-center gap-1.5">
                  <span
                    class="truncate font-medium"
                    :class="row.deleted ? 'text-gray-500 line-through dark:text-dark-400' : 'text-gray-900 dark:text-white'"
                    :title="row.title"
                  >
                    {{ row.title }}
                  </span>
                  <span v-if="row.deleted" class="badge badge-gray shrink-0 text-[10px]">{{ t('traffic.egress.accountsTable.deleted') }}</span>
                </span>
                <span class="mt-0.5 block truncate text-xs text-gray-500 dark:text-dark-400">{{ row.meta }}</span>
              </td>
              <td class="whitespace-nowrap px-3 py-2.5 text-right tabular-nums">
                <span class="font-semibold text-gray-900 dark:text-white">{{ row.total }}</span>
                <span class="mt-1 flex items-center justify-end gap-2 text-xs text-gray-500 dark:text-dark-400">
                  <span class="h-1.5 w-14 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
                    <span class="block h-full rounded-full bg-primary-500" :style="{ width: row.shareWidth }"></span>
                  </span>
                  <span class="w-11 text-right">{{ row.share }}</span>
                </span>
              </td>
              <td class="whitespace-nowrap px-3 py-2.5 text-right tabular-nums text-gray-700 dark:text-dark-200">{{ row.out }}</td>
              <td class="whitespace-nowrap px-3 py-2.5 text-right tabular-nums text-gray-700 dark:text-dark-200">{{ row.in }}</td>
              <td class="whitespace-nowrap px-3 py-2.5 text-right tabular-nums text-gray-700 dark:text-dark-200">{{ row.attempts }}</td>
              <td class="whitespace-nowrap py-2.5 pl-3 pr-4 text-right tabular-nums text-gray-700 dark:text-dark-200">{{ row.peak }}</td>
            </tr>
            <tr v-if="residual" data-testid="egress-account-residual">
              <td class="py-2.5 pl-4 pr-3">
                <span class="flex items-center gap-1 text-gray-500 dark:text-dark-400">
                  {{ t('traffic.egress.accountsTable.residual') }}
                  <HelpTooltip :content="t('traffic.egress.accountsTable.residualHint')" width-class="w-72" />
                </span>
              </td>
              <td class="whitespace-nowrap px-3 py-2.5 text-right tabular-nums text-gray-700 dark:text-dark-200">
                {{ residual.total }}
                <span class="block text-xs text-gray-500 dark:text-dark-400">{{ residual.share }}</span>
              </td>
              <td class="whitespace-nowrap px-3 py-2.5 text-right tabular-nums text-gray-500 dark:text-dark-400">{{ residual.out }}</td>
              <td class="whitespace-nowrap px-3 py-2.5 text-right tabular-nums text-gray-500 dark:text-dark-400">{{ residual.in }}</td>
              <td class="whitespace-nowrap px-3 py-2.5 text-right tabular-nums text-gray-500 dark:text-dark-400">{{ residual.attempts }}</td>
              <td class="py-2.5 pl-3 pr-4 text-right text-gray-400 dark:text-dark-500">—</td>
            </tr>
          </tbody>
        </table>
      </div>
    </template>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import HelpTooltip from '@/components/common/HelpTooltip.vue'
import Icon from '@/components/icons/Icon.vue'
import type { TrafficEgressDetailData } from '@/api/admin/trafficEgress'
import { formatByteCount, formatMbps, formatSampleTime } from '@/utils/trafficFormat'
import { accountTitle, tunnelDifference } from './egressFormat'
import { formatCount, formatShare, growthView, parseSignedBytes } from './subjectFormat'

const props = withDefaults(
  defineProps<{
    data: TrafficEgressDetailData | null
    error: string
    // WG 节点：额外与同一小时窗口的隧道流量比较。
    tunnel?: boolean
  }>(),
  { tunnel: false }
)

const { t } = useI18n()

const summary = computed(() => props.data?.summary ?? null)
const previousIncomplete = computed(() => props.data?.previous_window?.complete === false)

interface Tile {
  key: string
  label: string
  value: string
  sub?: string
  hint?: string
  muted?: boolean
}

function signedBytes(value: bigint) {
  if (value === 0n) return formatByteCount(0)
  return `${value > 0n ? '+' : '−'}${formatByteCount(value < 0n ? -value : value)}`
}

const tiles = computed<Tile[]>(() => {
  const value = summary.value
  if (!value) return []
  const total = value.peaks?.total
  const growth = growthView(value)
  const list: Tile[] = [
    {
      key: 'service',
      label: t('traffic.egress.section.service'),
      value: formatByteCount(value.total_bytes),
      sub: t('traffic.egress.split', { out: formatByteCount(value.bytes.upstream_out), in: formatByteCount(value.bytes.upstream_in) })
    },
    {
      key: 'peak',
      label: t('traffic.egress.section.peak'),
      value: total ? formatMbps(total.bytes_per_second) : t('traffic.ranking.peakUnknown'),
      sub: total ? t('bandwidth.host.peakAt', { time: formatSampleTime(total.at) }) : undefined,
      muted: !total
    },
    {
      key: 'attempts',
      label: t('traffic.egress.section.attempts'),
      hint: t('traffic.egress.section.attemptsHint'),
      value: formatCount(value.attempts),
      sub: t('traffic.egress.accounts', { count: value.account_count })
    }
  ]
  if (props.tunnel) {
    const wireguard = value.wireguard
    const difference = tunnelDifference(value)
    list.push(
      {
        key: 'tunnel',
        label: t('traffic.egress.section.tunnel'),
        value: wireguard?.has_records ? formatByteCount(wireguard.total_bytes) : '—',
        sub: wireguard?.has_records
          ? t('bandwidth.nodeDetail.split', { toNode: formatByteCount(wireguard.tx_bytes), fromNode: formatByteCount(wireguard.rx_bytes) })
          : t('traffic.egress.section.tunnelNone'),
        muted: !wireguard?.has_records
      },
      {
        key: 'difference',
        label: t('traffic.egress.section.difference'),
        hint: t('traffic.egress.section.differenceHint'),
        value: difference === null ? '—' : signedBytes(difference),
        sub:
          difference === null
            ? undefined
            : difference < 0n
              ? t('traffic.egress.section.differenceNegative')
              : t('traffic.egress.section.differenceSub'),
        muted: difference === null
      }
    )
  }
  list.push({
    key: 'growth',
    label: t('traffic.egress.section.growth'),
    value: growth.direction === 'unknown' ? '—' : growth.bytesText,
    sub: growth.direction === 'new' ? t('traffic.ranking.growthNew') : growth.percentText || undefined,
    muted: growth.direction === 'unknown'
  })
  return list
})

const routeTotal = computed(() => parseSignedBytes(summary.value?.total_bytes) ?? 0n)

const maxAccountShare = computed(() => Math.max(0, ...(props.data?.accounts ?? []).map((account) => account.share ?? 0)))

const accountRows = computed(() =>
  (props.data?.accounts ?? []).map((account) => ({
    id: account.account_id,
    title: accountTitle(account),
    deleted: account.account_deleted,
    meta: [account.platform, account.account_type, `ID ${account.account_id}`].filter(Boolean).join(' · '),
    total: formatByteCount(account.total_bytes),
    share: formatShare(account.share),
    shareWidth: account.share && maxAccountShare.value > 0 ? `${Math.max((account.share / maxAccountShare.value) * 100, 2)}%` : '0%',
    out: formatByteCount(account.bytes.upstream_out),
    in: formatByteCount(account.bytes.upstream_in),
    attempts: formatCount(account.attempts),
    peak: account.peaks?.total ? formatMbps(account.peaks.total.bytes_per_second) : '—'
  }))
)

// 缺失账号或超出容量的残余：有字节或尝试时才显示，保证账号合计能对上出口合计。
const residual = computed(() => {
  const value = props.data?.unattributed_account
  const total = parseSignedBytes(value?.total_bytes)
  if (!value || total === null || (total <= 0n && !value.attempts)) return null
  return {
    total: formatByteCount(total),
    share: routeTotal.value > 0n ? formatShare(Number(total) / Number(routeTotal.value)) : '',
    out: formatByteCount(value.bytes.upstream_out),
    in: formatByteCount(value.bytes.upstream_in),
    attempts: formatCount(value.attempts)
  }
})
</script>
