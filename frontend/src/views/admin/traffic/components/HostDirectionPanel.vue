<template>
  <!-- 双向流量：入站、出站各一条，按较大方向缩放；有实时速率时条带缓慢流动 -->
  <section class="flex h-full flex-col rounded-2xl bg-gray-50/80 p-4 ring-1 ring-gray-900/[0.04] dark:bg-dark-900/40 dark:ring-white/[0.04]" data-testid="host-direction">
    <header class="flex items-center justify-between gap-3">
      <h4 class="text-xs font-semibold text-gray-700 dark:text-dark-100">{{ t('traffic.host.direction.title') }}</h4>
      <span class="text-[11px] text-gray-400 dark:text-dark-500">{{ ruleText }}</span>
    </header>

    <ul class="mt-3 flex flex-1 flex-col justify-center gap-4">
      <li v-for="row in rows" :key="row.key" :data-testid="`host-direction-${row.key}`" :class="row.muted ? 'opacity-55' : ''">
        <div class="flex items-center gap-3">
          <span
            class="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl"
            :style="{ backgroundColor: row.soft, color: row.color }"
            aria-hidden="true"
          >
            <svg
              class="h-4 w-4"
              :class="row.live ? `host-arrow host-arrow--${row.key}` : ''"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="2.25"
              stroke-linecap="round"
              stroke-linejoin="round"
            >
              <path v-if="row.key === 'rx'" d="M12 4v15m0 0 6-6m-6 6-6-6" />
              <path v-else d="M12 20V5m0 0-6 6m6-6 6 6" />
            </svg>
          </span>
          <div class="min-w-0 flex-1">
            <div class="flex items-baseline justify-between gap-2">
              <span class="flex items-center gap-1.5 text-xs font-medium text-gray-600 dark:text-dark-300">
                {{ row.label }}
                <span
                  v-if="row.badge"
                  class="rounded-full px-1.5 py-px text-[10px] font-semibold"
                  :class="row.muted ? 'bg-gray-200/70 text-gray-500 dark:bg-dark-700 dark:text-dark-400' : 'bg-white text-gray-600 ring-1 ring-gray-200 dark:bg-dark-800 dark:text-dark-200 dark:ring-dark-600'"
                >
                  {{ row.badge }}
                </span>
              </span>
              <span class="text-base font-semibold text-gray-900 dark:text-white">{{ row.bytesText }}</span>
            </div>
            <div class="mt-1.5 h-2 overflow-hidden rounded-full" :style="{ backgroundColor: row.track }">
              <div
                class="host-flow h-full rounded-full transition-[width] duration-700 ease-out"
                :class="row.live ? 'host-flow--live' : ''"
                :style="{ width: ready ? `${row.width}%` : '0%', backgroundColor: row.color }"
              ></div>
            </div>
            <div class="mt-1 flex items-center justify-between gap-2 text-[11px] tabular-nums text-gray-500 dark:text-dark-400">
              <span>{{ row.shareText }}</span>
              <span class="inline-flex items-center gap-1">
                <span v-if="row.live" class="h-1.5 w-1.5 rounded-full" :style="{ backgroundColor: row.color }"></span>
                {{ row.rateText }}
              </span>
            </div>
          </div>
        </div>
      </li>
    </ul>

    <p v-if="footnote" class="mt-3 border-t border-gray-200/70 pt-2.5 text-[11px] leading-relaxed text-gray-400 dark:border-dark-700 dark:text-dark-500" data-testid="host-direction-note">
      {{ footnote }}
    </p>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { HostBillingMode, HostUnitBase } from '@/api/admin/hostTraffic'
import { formatQuotaBytes, formatRatioPercent, parseByteString } from '@/components/admin/traffic/hostFormat'
import { formatByteCount, formatMbps } from '@/utils/trafficFormat'
import { useBandwidthPalette } from '@/views/admin/bandwidth/useBandwidthPalette'

const props = defineProps<{
  rx: string | null
  tx: string | null
  rxRate: number | null
  txRate: number | null
  // 没有套餐时为 null：不标注计费方向，字节按页面通用的 1024 进制显示。
  billingMode: HostBillingMode | null
  unitBase: HostUnitBase | null
  footnote?: string
}>()

const { t } = useI18n()
const palette = useBandwidthPalette()

// 挂载后再展开条带，产生从 0 增长的过渡。
const ready = ref(false)
onMounted(() => {
  if (typeof requestAnimationFrame === 'function') requestAnimationFrame(() => (ready.value = true))
  else ready.value = true
})

function format(value: bigint | null) {
  if (value === null) return '—'
  return props.unitBase ? formatQuotaBytes(value, props.unitBase) : formatByteCount(value)
}

const ruleText = computed(() =>
  props.billingMode ? t(`traffic.host.direction.rule.${props.billingMode}`) : t('traffic.host.direction.rule.none')
)

const rows = computed(() => {
  const rx = parseByteString(props.rx)
  const tx = parseByteString(props.tx)
  const max = rx !== null && tx !== null ? (rx > tx ? rx : tx) : rx ?? tx ?? 0n
  const total = (rx ?? 0n) + (tx ?? 0n)
  const larger = rx !== null && tx !== null ? (rx >= tx ? 'rx' : 'tx') : null
  const dark = palette.value.dark
  return (['rx', 'tx'] as const).map((key) => {
    const bytes = key === 'rx' ? rx : tx
    const rate = key === 'rx' ? props.rxRate : props.txRate
    const color = key === 'rx' ? palette.value.in : palette.value.out
    let badge = ''
    let muted = false
    switch (props.billingMode) {
      case 'outbound':
        badge = key === 'tx' ? t('traffic.host.direction.billed') : t('traffic.host.direction.notBilled')
        muted = key === 'rx'
        break
      case 'inbound':
        badge = key === 'rx' ? t('traffic.host.direction.billed') : t('traffic.host.direction.notBilled')
        muted = key === 'tx'
        break
      case 'max':
        badge = larger === key ? t('traffic.host.direction.larger') : ''
        muted = larger !== null && larger !== key
        break
    }
    return {
      key,
      label: key === 'rx' ? t('traffic.host.direction.inbound') : t('traffic.host.direction.outbound'),
      color,
      soft: key === 'rx' ? (dark ? 'rgba(144,133,233,0.16)' : 'rgba(98,80,214,0.10)') : dark ? 'rgba(13,148,136,0.18)' : 'rgba(13,148,136,0.10)',
      track: key === 'rx' ? (dark ? 'rgba(144,133,233,0.14)' : 'rgba(98,80,214,0.10)') : dark ? 'rgba(13,148,136,0.16)' : 'rgba(13,148,136,0.10)',
      bytesText: format(bytes),
      width: bytes !== null && max > 0n ? Math.max(Number((bytes * 1000n) / max) / 10, bytes > 0n ? 2 : 0) : 0,
      shareText: bytes !== null && total > 0n ? t('traffic.host.direction.share', { percent: formatRatioPercent(Number((bytes * 10000n) / total) / 10000) }) : '—',
      rateText: rate === null ? '—' : formatMbps(rate),
      live: rate !== null && rate > 0,
      badge,
      muted
    }
  })
})
</script>

<style scoped>
/* 有实时速率时在条带上叠一层缓慢移动的斜纹，表示流量正在流动。 */
.host-flow--live {
  background-image: repeating-linear-gradient(
    -45deg,
    rgba(255, 255, 255, 0.22) 0 6px,
    transparent 6px 12px
  );
  background-size: 17px 17px;
}

@media (prefers-reduced-motion: no-preference) {
  .host-flow--live {
    animation: host-flow 1.6s linear infinite;
  }

  /* 方向箭头沿流向轻微往复。 */
  .host-arrow--rx {
    animation: host-arrow-down 1.8s ease-in-out infinite;
  }

  .host-arrow--tx {
    animation: host-arrow-up 1.8s ease-in-out infinite;
  }
}

@keyframes host-arrow-down {
  0%,
  100% {
    transform: translateY(-1.5px);
  }
  50% {
    transform: translateY(1.5px);
  }
}

@keyframes host-arrow-up {
  0%,
  100% {
    transform: translateY(1.5px);
  }
  50% {
    transform: translateY(-1.5px);
  }
}

@keyframes host-flow {
  from {
    background-position: 0 0;
  }
  to {
    background-position: 17px 0;
  }
}
</style>
