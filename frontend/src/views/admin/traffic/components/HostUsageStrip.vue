<template>
  <!-- 所选范围内的整机网卡流量：与服务流量口径不同，单独一行展示，不参与上方合计 -->
  <div
    class="mt-5 flex flex-col gap-2 rounded-2xl bg-gray-50 px-4 py-3 dark:bg-dark-900/40 sm:flex-row sm:items-center sm:justify-between"
    data-testid="host-usage"
  >
    <div class="flex items-center gap-2 text-xs font-medium text-gray-600 dark:text-dark-300">
      <Icon name="server" size="sm" class="h-4 w-4 text-gray-400 dark:text-dark-500" />
      {{ t('traffic.host.usage.title') }}
      <HelpTooltip :content="t('traffic.host.usage.hint')" width-class="w-72" />
    </div>
    <div v-if="loading && !usage" class="h-5 w-48 animate-pulse rounded bg-gray-200/70 dark:bg-dark-700/60"></div>
    <p v-else-if="error" class="text-xs text-red-600 dark:text-red-400" data-testid="host-usage-error">{{ error }}</p>
    <p v-else-if="message" class="text-xs text-gray-500 dark:text-dark-400" data-testid="host-usage-message">{{ message }}</p>
    <div v-else-if="usage" class="flex flex-wrap items-baseline gap-x-3 gap-y-1 tabular-nums sm:justify-end">
      <span class="text-base font-semibold text-gray-900 dark:text-white" data-testid="host-usage-total">{{ formatByteCount(usage.total_bytes) }}</span>
      <span class="text-xs text-gray-500 dark:text-dark-400">
        {{ t('traffic.host.usage.inOut', { inbound: formatByteCount(usage.rx_bytes), outbound: formatByteCount(usage.tx_bytes) }) }}
      </span>
      <span v-if="coverageText" class="text-xs text-amber-600 dark:text-amber-400">{{ coverageText }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import HelpTooltip from '@/components/common/HelpTooltip.vue'
import type { HostTrafficUsage } from '@/api/admin/hostTraffic'
import { formatRatioPercent } from '@/components/admin/traffic/hostFormat'
import { formatByteCount } from '@/utils/trafficFormat'

const props = defineProps<{
  usage: HostTrafficUsage | null
  loading: boolean
  error: string
}>()

const { t } = useI18n()

// 未开启或不支持时不显示数字，避免把容器网卡或空数据当作整机流量。
const message = computed(() => {
  const usage = props.usage
  if (!usage) return ''
  if (usage.state === 'needs_host_mount') return t('traffic.host.usage.notEnabled')
  if (usage.state === 'unsupported') return t('traffic.host.usage.unsupportedEnv')
  if (!usage.has_records) return t('traffic.host.usage.noRecords')
  return ''
})

// 读数覆盖不足 99% 时提示，数值只是已观测部分。
const coverageText = computed(() => {
  const usage = props.usage
  if (!usage || usage.expected_seconds <= 0) return ''
  const ratio = usage.observed_seconds / usage.expected_seconds
  return ratio < 0.99 ? t('traffic.host.usage.coverage', { percent: formatRatioPercent(ratio) }) : ''
})
</script>
