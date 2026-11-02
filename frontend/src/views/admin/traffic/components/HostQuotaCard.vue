<template>
  <section
    class="relative overflow-hidden rounded-3xl bg-white shadow-sm ring-1 ring-gray-900/5 dark:bg-dark-800 dark:ring-dark-700"
    data-testid="host-quota"
    :aria-busy="loading"
  >
    <!-- 顶部柔和底色，按用量状态变化 -->
    <div class="pointer-events-none absolute inset-x-0 top-0 h-44 bg-gradient-to-b transition-colors duration-700" :class="glowClass" aria-hidden="true"></div>

    <div class="relative p-6">
      <header class="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div class="flex min-w-0 items-start gap-3">
          <span class="flex h-11 w-11 shrink-0 items-center justify-center rounded-2xl bg-gradient-to-br from-primary-400 to-primary-600 text-white shadow-md shadow-primary-500/25">
            <Icon name="server" size="md" />
          </span>
          <div class="min-w-0">
            <h3 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('traffic.host.title') }}</h3>
            <p class="mt-0.5 text-xs text-gray-500 dark:text-dark-400">{{ t('traffic.host.subtitle') }}</p>
            <div v-if="chips.length" class="mt-2 flex flex-wrap gap-1.5" data-testid="host-chips">
              <span
                v-for="chip in chips"
                :key="chip.key"
                class="inline-flex items-center gap-1 rounded-full bg-white/80 px-2 py-0.5 text-[11px] font-medium text-gray-600 ring-1 ring-gray-200 backdrop-blur dark:bg-dark-900/50 dark:text-dark-300 dark:ring-dark-600"
              >
                <Icon :name="chip.icon" size="xs" class="h-3 w-3 text-gray-400 dark:text-dark-500" />
                {{ chip.text }}
              </span>
            </div>
          </div>
        </div>
        <div v-if="mode === 'quota'" class="flex shrink-0 flex-wrap gap-2">
          <button type="button" class="btn btn-secondary btn-sm" data-testid="host-detail" @click="emit('detail')">
            <Icon name="chartBar" size="xs" class="h-3.5 w-3.5" />
            {{ t('traffic.host.actions.detail') }}
          </button>
          <button type="button" class="btn btn-secondary btn-sm" data-testid="host-calibrate" @click="emit('calibrate')">
            <Icon name="calculator" size="xs" class="h-3.5 w-3.5" />
            {{ t('traffic.host.actions.calibrate') }}
          </button>
          <button type="button" class="btn btn-secondary btn-sm" data-testid="host-edit" @click="emit('edit')">
            <Icon name="edit" size="xs" class="h-3.5 w-3.5" />
            {{ t('traffic.host.actions.edit') }}
          </button>
        </div>
      </header>

      <div v-if="!loaded" class="mt-6 grid gap-6 lg:grid-cols-[minmax(0,1.65fr)_minmax(0,1fr)]" data-testid="host-skeleton">
        <div class="space-y-4">
          <div class="h-12 w-64 animate-pulse rounded-xl bg-gray-100 dark:bg-dark-700/50"></div>
          <div class="h-3.5 animate-pulse rounded-full bg-gray-100 dark:bg-dark-700/50"></div>
          <div class="h-20 animate-pulse rounded-2xl bg-gray-100 dark:bg-dark-700/50"></div>
        </div>
        <div class="h-48 animate-pulse rounded-2xl bg-gray-100 dark:bg-dark-700/50"></div>
      </div>

      <p
        v-else-if="!status"
        :role="unsupported ? undefined : 'alert'"
        class="mt-5 rounded-2xl px-4 py-3 text-sm"
        :class="
          unsupported
            ? 'border border-dashed border-gray-200 text-gray-500 dark:border-dark-700 dark:text-dark-400'
            : 'bg-red-50 text-red-700 dark:bg-red-950/30 dark:text-red-300'
        "
        data-testid="host-error"
      >
        {{ error }}
      </p>

      <HostMountGuide v-else-if="mode === 'needs_host_mount'" />

      <p
        v-else-if="mode === 'unsupported'"
        class="mt-5 rounded-2xl border border-dashed border-gray-200 px-4 py-6 text-center text-sm text-gray-500 dark:border-dark-700 dark:text-dark-400"
        data-testid="host-unsupported"
      >
        {{ t('traffic.host.unsupported') }}
      </p>

      <div
        v-else-if="mode === 'starting'"
        class="mt-5 flex items-center gap-3 rounded-2xl border border-dashed border-gray-200 px-4 py-6 text-sm text-gray-500 dark:border-dark-700 dark:text-dark-400"
        data-testid="host-starting"
      >
        <span class="relative flex h-2.5 w-2.5">
          <span class="absolute inline-flex h-full w-full rounded-full bg-primary-400 opacity-60 motion-safe:animate-ping"></span>
          <span class="relative inline-flex h-2.5 w-2.5 rounded-full bg-primary-500"></span>
        </span>
        {{ t('traffic.host.starting') }}
      </div>

      <!-- 还没有套餐：统计起点以来的整机用量与双向构成，引导设置套餐 -->
      <Transition v-else-if="mode === 'no_plan' && status" appear name="host-rise">
        <div class="mt-6 grid gap-6 lg:grid-cols-[minmax(0,1.65fr)_minmax(0,1fr)]" data-testid="host-no-plan">
          <div class="flex flex-col">
            <p class="text-xs font-medium text-gray-500 dark:text-dark-400">
              {{ t('traffic.host.noPlan.since', { time: formatLocalShort(status.since_start?.from) }) }}
            </p>
            <p class="mt-1 text-5xl font-semibold tracking-tight text-gray-900 dark:text-white" data-testid="host-since-total">{{ sinceTotalText }}</p>
            <div class="mt-6 flex flex-1 flex-col justify-between gap-4 rounded-2xl border border-dashed border-primary-200 bg-primary-50/50 p-5 dark:border-primary-800/50 dark:bg-primary-900/10 sm:flex-row sm:items-center">
              <p class="text-sm leading-relaxed text-gray-700 dark:text-dark-200">{{ t('traffic.host.noPlan.cta') }}</p>
              <button type="button" class="btn btn-primary btn-sm shrink-0" data-testid="host-setup" @click="emit('setup')">
                <Icon name="plus" size="xs" class="h-3.5 w-3.5" />
                {{ t('traffic.host.actions.setup') }}
              </button>
            </div>
          </div>
          <HostDirectionPanel
            :rx="status.since_start?.rx_bytes ?? null"
            :tx="status.since_start?.tx_bytes ?? null"
            :rx-rate="rates.rx"
            :tx-rate="rates.tx"
            :billing-mode="null"
            :unit-base="null"
          />
        </div>
      </Transition>

      <!-- 有套餐：本期用量、双向构成、剩余、预测与重置 -->
      <Transition v-else-if="mode === 'quota' && view && plan && period" appear name="host-rise">
        <div class="mt-6 grid gap-6 lg:grid-cols-[minmax(0,1.65fr)_minmax(0,1fr)]">
          <div class="min-w-0">
            <div class="flex flex-wrap items-end justify-between gap-x-6 gap-y-3">
              <div class="min-w-0">
                <p class="text-xs font-medium text-gray-500 dark:text-dark-400">{{ t('traffic.host.usedWithMode', { mode: view.modeLabel }) }}</p>
                <p class="mt-1 flex flex-wrap items-baseline gap-x-2" data-testid="host-used">
                  <span v-if="view.usedPrefix" class="text-sm font-medium text-gray-500 dark:text-dark-400">{{ view.usedPrefix }}</span>
                  <span class="text-5xl font-semibold tracking-tight transition-colors duration-500" :class="toneText">{{ usedText }}</span>
                  <span class="text-lg text-gray-400 dark:text-dark-500">{{ t('traffic.host.ofQuota', { quota: view.quotaText }) }}</span>
                </p>
              </div>
              <div class="flex flex-col items-end gap-1.5">
                <span class="inline-flex items-center gap-1 rounded-full px-2.5 py-1 text-xs font-medium" :class="toneChip.className" data-testid="host-tone">
                  <Icon :name="toneChip.icon" size="xs" class="h-3.5 w-3.5" :class="view.tone === 'over' || view.tone === 'critical' ? 'motion-safe:animate-pulse' : ''" />
                  {{ toneChip.label }}
                </span>
                <span class="text-2xl font-semibold tracking-tight transition-colors duration-500" :class="toneText" data-testid="host-percent">{{ view.percentText }}</span>
              </div>
            </div>

            <!-- 用量条：按方向分段，带提醒线与时间进度线 -->
            <div class="mt-5">
              <div
                class="relative h-3.5 rounded-full transition-colors duration-500"
                :class="trackClass"
                role="progressbar"
                :aria-valuenow="Math.round(view.ratio * 100)"
                aria-valuemin="0"
                aria-valuemax="100"
                :aria-label="t('traffic.host.usedWithMode', { mode: view.modeLabel })"
                data-testid="host-bar"
              >
                <div class="absolute inset-0 flex gap-[2px] overflow-hidden rounded-full">
                  <span
                    v-for="(segment, index) in meter.segments"
                    :key="segment.key"
                    class="h-full shrink-0 transition-[width] duration-[900ms] ease-out"
                    :style="segmentStyle(segment.key, segment.width, index)"
                    :data-testid="segment.key === 'adjustment' ? 'host-bar-adjustment' : `host-bar-${segment.key}`"
                  ></span>
                </div>
                <span
                  v-for="mark in meter.thresholds"
                  :key="mark.percent"
                  class="absolute inset-y-0 w-0.5 bg-white/90 dark:bg-dark-800/90"
                  :style="{ left: `calc(${mark.at}% - 1px)` }"
                  :title="t('traffic.host.legend.threshold', { percent: mark.percent })"
                ></span>
                <span
                  v-if="meter.quotaAt !== null"
                  class="absolute -inset-y-1 w-0.5 rounded-full bg-gray-900 dark:bg-white"
                  :style="{ left: `calc(${meter.quotaAt}% - 1px)` }"
                ></span>
                <span
                  v-if="meter.pace"
                  class="absolute -bottom-1.5 -top-1.5 w-0.5 rounded-full bg-gray-700/80 transition-[left] duration-[900ms] dark:bg-gray-200/80"
                  :style="{ left: ready ? `calc(${meter.pace.at}% - 1px)` : '0%' }"
                  data-testid="host-pace"
                >
                  <span class="absolute -top-1 left-1/2 h-1.5 w-1.5 -translate-x-1/2 rounded-full bg-gray-700 dark:bg-gray-200"></span>
                </span>
              </div>
              <div class="mt-2.5 flex flex-wrap items-center gap-x-4 gap-y-1 text-[11px] text-gray-500 dark:text-dark-400" data-testid="host-legend">
                <span v-for="item in legend" :key="item.key" class="inline-flex items-center gap-1.5">
                  <span class="h-2 w-3 rounded-sm" :style="item.style"></span>
                  {{ item.label }}
                </span>
                <span v-if="meter.thresholds.length" class="inline-flex items-center gap-1.5">
                  <span class="h-2.5 w-0.5 rounded-full bg-gray-300 dark:bg-dark-500"></span>
                  {{ t('traffic.host.legend.thresholds', { values: meter.thresholds.map((mark) => `${mark.percent}%`).join(' / ') }) }}
                </span>
                <span v-if="meter.pace" class="inline-flex items-center gap-1.5" :title="t('traffic.host.pace.hint')">
                  <span class="h-2.5 w-0.5 rounded-full bg-gray-700 dark:bg-gray-200"></span>
                  {{ t('traffic.host.pace.label', { percent: formatRatioPercent(meter.pace.elapsed) }) }}
                </span>
                <span v-if="meter.quotaAt !== null" class="inline-flex items-center gap-1.5">
                  <span class="h-2.5 w-0.5 rounded-full bg-gray-900 dark:bg-white"></span>
                  {{ t('traffic.host.legend.quotaLine') }}
                </span>
              </div>
            </div>

            <!-- 剩余、预测、重置 -->
            <dl class="mt-6 grid gap-3 sm:grid-cols-3">
              <div class="rounded-2xl bg-gray-50/80 px-4 py-3 ring-1 ring-gray-900/[0.04] dark:bg-dark-900/40 dark:ring-white/[0.04]" data-testid="host-remaining">
                <dt class="text-xs text-gray-500 dark:text-dark-400">{{ view.overText ? t('traffic.host.over') : t('traffic.host.remaining') }}</dt>
                <dd class="mt-1 text-xl font-semibold" :class="view.overText ? 'text-red-600 dark:text-red-400' : 'text-gray-900 dark:text-white'">
                  <span v-if="view.remainingPrefix && !view.overText" class="mr-1 text-xs font-medium text-gray-500 dark:text-dark-400">{{ view.remainingPrefix }}</span>
                  {{ view.overText || view.remainingText }}
                </dd>
                <dd class="mt-0.5 text-[11px] text-gray-400 dark:text-dark-500">{{ view.remainingSub }}</dd>
              </div>
              <div class="rounded-2xl bg-gray-50/80 px-4 py-3 ring-1 ring-gray-900/[0.04] dark:bg-dark-900/40 dark:ring-white/[0.04]" data-testid="host-projection">
                <dt class="text-xs text-gray-500 dark:text-dark-400">{{ t('traffic.host.projection') }}</dt>
                <template v-if="view.projection">
                  <dd class="mt-1 text-xl font-semibold" :class="view.projection.over ? 'text-red-600 dark:text-red-400' : 'text-gray-900 dark:text-white'">
                    <span v-if="view.projection.prefix" class="mr-1 text-xs font-medium text-gray-500 dark:text-dark-400">{{ view.projection.prefix }}</span>
                    {{ view.projection.bytes }}
                  </dd>
                  <dd class="mt-0.5 text-[11px]" :class="view.projection.over ? 'text-red-500 dark:text-red-400' : 'text-gray-400 dark:text-dark-500'">
                    {{ t('traffic.host.projectionOfQuota', { percent: view.projection.percent }) }}
                  </dd>
                </template>
                <dd v-else class="mt-1.5 text-xs leading-relaxed text-gray-400 dark:text-dark-500">{{ t('traffic.host.projectionPending') }}</dd>
              </div>
              <div class="rounded-2xl bg-gray-50/80 px-4 py-3 ring-1 ring-gray-900/[0.04] dark:bg-dark-900/40 dark:ring-white/[0.04]" data-testid="host-reset">
                <dt class="text-xs text-gray-500 dark:text-dark-400">{{ t('traffic.host.nextReset') }}</dt>
                <dd class="mt-1 text-xl font-semibold text-gray-900 dark:text-white">{{ view.countdown }}</dd>
                <dd class="mt-0.5 text-[11px] text-gray-400 dark:text-dark-500">{{ view.resetSub }}</dd>
              </div>
            </dl>
          </div>

          <HostDirectionPanel
            :rx="period.rx_bytes"
            :tx="period.tx_bytes"
            :rx-rate="rates.rx"
            :tx-rate="rates.tx"
            :billing-mode="plan.billing_mode"
            :unit-base="plan.unit_base"
            :footnote="directionNote"
          />
        </div>
      </Transition>

      <!-- 状态栏：数据可信度、最近读数、用量提醒 -->
      <div
        v-if="loaded && status && (mode === 'quota' || mode === 'no_plan')"
        class="mt-5 flex flex-col gap-3 border-t border-gray-100 pt-4 text-xs dark:border-dark-700 lg:flex-row lg:items-start lg:justify-between lg:gap-8"
      >
        <div v-if="mode === 'quota'" class="min-w-0 lg:flex-1" data-testid="host-coverage">
          <p class="flex flex-wrap items-center gap-x-2 gap-y-1">
            <span class="inline-flex items-center gap-1 font-medium" :class="coverage.className">
              <Icon :name="coverage.icon" size="xs" class="h-3.5 w-3.5 shrink-0" />
              {{ coverage.label }}
            </span>
            <button
              v-if="coverage.canFill"
              type="button"
              class="rounded-full bg-amber-50 px-2 py-0.5 font-medium text-amber-700 ring-1 ring-amber-200 transition-colors hover:bg-amber-100 dark:bg-amber-950/30 dark:text-amber-300 dark:ring-amber-900/50"
              data-testid="host-fill"
              @click="emit('calibrate')"
            >
              {{ t('traffic.host.actions.fill') }}
            </button>
          </p>
          <p v-for="line in coverage.lines" :key="line" class="mt-1 leading-relaxed text-gray-500 dark:text-dark-400">{{ line }}</p>
        </div>
        <div v-else></div>
        <div class="flex flex-wrap items-center gap-x-5 gap-y-1.5 text-gray-500 dark:text-dark-400 lg:max-w-[62%] lg:justify-end">
          <span class="inline-flex items-center gap-1.5" data-testid="host-last-sample">
            <span class="relative flex h-2 w-2">
              <span v-if="!stale" class="absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-60 motion-safe:animate-ping"></span>
              <span class="relative inline-flex h-2 w-2 rounded-full" :class="stale ? 'bg-amber-500' : 'bg-emerald-500'"></span>
            </span>
            {{ t('traffic.host.lastSample') }}
            <span class="tabular-nums" :class="stale ? 'font-medium text-amber-600 dark:text-amber-400' : 'text-gray-700 dark:text-dark-200'">{{ lastSampleText }}</span>
          </span>
          <span v-if="view" class="inline-flex items-center gap-1.5" data-testid="host-alerts">
            <Icon name="bell" size="xs" class="h-3.5 w-3.5" />
            {{ t('traffic.host.alerts.label') }}
            <span class="text-gray-700 dark:text-dark-200">{{ view.alertsText }}</span>
            <span v-if="view.alertsEnabled && !status.alerts.email_enabled" class="text-amber-600 dark:text-amber-400">· {{ t('traffic.host.alerts.emailOff') }}</span>
          </span>
        </div>
      </div>

      <!-- 提醒与异常：按严重程度排列 -->
      <TransitionGroup v-if="loaded && status && notices.length" tag="ul" name="host-rise" appear class="mt-4 space-y-2" data-testid="host-notices">
        <li
          v-for="notice in notices"
          :key="notice.key"
          class="flex flex-wrap items-start gap-x-2 gap-y-1.5 rounded-xl px-3.5 py-2.5 text-xs leading-relaxed"
          :class="NOTICE_CLASS[notice.tone]"
          :data-testid="`host-notice-${notice.key}`"
        >
          <Icon :name="NOTICE_ICON[notice.tone]" size="sm" class="mt-px h-4 w-4 shrink-0" />
          <span class="min-w-0 flex-1">{{ notice.text }}</span>
          <button
            v-if="notice.action"
            type="button"
            class="shrink-0 font-medium underline decoration-dotted underline-offset-2 hover:decoration-solid"
            :data-testid="`host-notice-action-${notice.key}`"
            @click="notice.action.run"
          >
            {{ notice.action.label }}
          </button>
        </li>
      </TransitionGroup>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useNow } from '@vueuse/core'
import Icon from '@/components/icons/Icon.vue'
import HostMountGuide from '@/components/admin/traffic/HostMountGuide.vue'
import type { HostTrafficStatus, HostUnitBase } from '@/api/admin/hostTraffic'
import {
  billingModeLabel,
  boundPrefix,
  daysUntil,
  formatAge,
  formatDeviation,
  formatDuration,
  formatLocalShort,
  formatPeriodRange,
  formatQuotaBytes,
  formatRatioPercent,
  formatSignedQuotaBytes,
  formatZonedDateTime,
  hostCardMode,
  parseByteString,
  quotaMeter,
  quotaTone,
  remainingPrefix,
  resetRuleText,
  selectedInterfaces,
  type MeterSegmentKey,
  type QuotaTone
} from '@/components/admin/traffic/hostFormat'
import { useCountUp } from '@/components/admin/traffic/useCountUp'
import { formatByteCount } from '@/utils/trafficFormat'
import { useBandwidthPalette } from '@/views/admin/bandwidth/useBandwidthPalette'
import HostDirectionPanel from './HostDirectionPanel.vue'

const props = defineProps<{
  status: HostTrafficStatus | null
  loaded: boolean
  loading: boolean
  error: string
  unsupported: boolean
}>()

const emit = defineEmits<{
  setup: []
  edit: [patch?: { unit_base: HostUnitBase }]
  calibrate: []
  detail: []
}>()

const { t } = useI18n()
const now = useNow({ interval: 10_000 })
const palette = useBandwidthPalette()

// 挂载后再展开用量条和时间进度线，形成从左到右的过渡。
const ready = ref(false)
onMounted(() => {
  if (typeof requestAnimationFrame === 'function') requestAnimationFrame(() => (ready.value = true))
  else ready.value = true
})

const STRIPES = 'repeating-linear-gradient(135deg, rgba(255,255,255,0.6) 0 3px, transparent 3px 6px)'

type NoticeTone = 'critical' | 'warning' | 'info' | 'muted'
const NOTICE_CLASS: Record<NoticeTone, string> = {
  critical: 'bg-red-50 text-red-700 ring-1 ring-red-200/70 dark:bg-red-950/30 dark:text-red-300 dark:ring-red-900/40',
  warning: 'bg-amber-50 text-amber-800 ring-1 ring-amber-200/70 dark:bg-amber-950/20 dark:text-amber-300 dark:ring-amber-900/40',
  info: 'bg-sky-50 text-sky-800 ring-1 ring-sky-200/70 dark:bg-sky-950/20 dark:text-sky-300 dark:ring-sky-900/40',
  muted: 'bg-gray-50 text-gray-600 dark:bg-dark-900/40 dark:text-dark-300'
}
const NOTICE_ICON = {
  critical: 'exclamationCircle',
  warning: 'exclamationTriangle',
  info: 'infoCircle',
  muted: 'clock'
} as const

// 用量状态：文字、底色和状态标签一起变化，状态标签带图标，不只靠颜色区分。
const TONE_STYLE: Record<QuotaTone, { text: string; track: string; glow: string; chip: string; icon: 'checkCircle' | 'exclamationTriangle' | 'exclamationCircle' }> = {
  normal: {
    text: 'text-gray-900 dark:text-white',
    track: 'bg-gray-100 dark:bg-dark-700',
    glow: 'from-primary-50/70 dark:from-primary-900/10',
    chip: 'bg-emerald-50 text-emerald-700 ring-1 ring-emerald-200/80 dark:bg-emerald-950/30 dark:text-emerald-300 dark:ring-emerald-900/50',
    icon: 'checkCircle'
  },
  warning: {
    text: 'text-amber-600 dark:text-amber-400',
    track: 'bg-amber-100/70 dark:bg-amber-950/40',
    glow: 'from-amber-50/80 dark:from-amber-900/10',
    chip: 'bg-amber-50 text-amber-700 ring-1 ring-amber-200/80 dark:bg-amber-950/30 dark:text-amber-300 dark:ring-amber-900/50',
    icon: 'exclamationTriangle'
  },
  critical: {
    text: 'text-red-600 dark:text-red-400',
    track: 'bg-red-100/70 dark:bg-red-950/40',
    glow: 'from-red-50/80 dark:from-red-900/10',
    chip: 'bg-red-50 text-red-700 ring-1 ring-red-200/80 dark:bg-red-950/30 dark:text-red-300 dark:ring-red-900/50',
    icon: 'exclamationCircle'
  },
  over: {
    text: 'text-red-600 dark:text-red-400',
    track: 'bg-red-100/70 dark:bg-red-950/40',
    glow: 'from-red-50/80 dark:from-red-900/10',
    chip: 'bg-red-50 text-red-700 ring-1 ring-red-200/80 dark:bg-red-950/30 dark:text-red-300 dark:ring-red-900/50',
    icon: 'exclamationCircle'
  }
}

const mode = computed(() => hostCardMode(props.status))
const plan = computed(() => props.status?.plan ?? null)
const period = computed(() => props.status?.period ?? null)
const stale = computed(() => props.status?.collection.state === 'stale' || props.status?.collection.state === 'error')

const interfaceNames = computed(() => (props.status ? selectedInterfaces(props.status).join(' + ') : ''))

// 计费网卡的当前速率合计；读数中断时为 null，不显示 0。
const rates = computed(() => {
  const selected = props.status?.interfaces.filter((item) => item.selected) ?? []
  const sum = (key: 'rx_bytes_per_second' | 'tx_bytes_per_second') =>
    stale.value || !selected.length || selected.some((item) => item[key] === null)
      ? null
      : selected.reduce((total, item) => total + (item[key] ?? 0), 0)
  return { rx: sum('rx_bytes_per_second'), tx: sum('tx_bytes_per_second') }
})

const chips = computed(() => {
  const list: { key: string; icon: 'calendar' | 'globe'; text: string }[] = []
  if (mode.value === 'quota' && plan.value && period.value) {
    list.push({ key: 'period', icon: 'calendar', text: `${formatPeriodRange(period.value, plan.value.timezone)} · ${plan.value.timezone}` })
  }
  if (interfaceNames.value && (mode.value === 'quota' || mode.value === 'no_plan')) {
    list.push({ key: 'interface', icon: 'globe', text: interfaceNames.value })
  }
  return list
})

const lastSampleText = computed(() => {
  const at = props.status?.collection.last_sample_at
  if (!at) return '—'
  return `${formatLocalShort(at)} · ${formatAge(at, now.value.getTime(), t)}`
})

const sinceTarget = computed(() => {
  const bytes = parseByteString(props.status?.since_start?.total_bytes)
  return bytes === null ? null : Number(bytes)
})
const sinceAnimated = useCountUp(sinceTarget)
const sinceTotalText = computed(() => (sinceAnimated.value === null ? '—' : formatByteCount(Math.round(sinceAnimated.value))))

const usedTarget = computed(() => {
  const bytes = parseByteString(period.value?.billed_bytes)
  return bytes === null ? null : Number(bytes)
})
const usedAnimated = useCountUp(usedTarget)
const usedText = computed(() =>
  usedAnimated.value === null || !plan.value ? '—' : formatQuotaBytes(BigInt(Math.round(usedAnimated.value)), plan.value.unit_base)
)

const meter = computed(() =>
  plan.value && period.value
    ? quotaMeter(period.value, plan.value, now.value.getTime())
    : { segments: [], quotaAt: null, thresholds: [], pace: null }
)

function segmentColor(key: MeterSegmentKey) {
  if (key === 'rx') return palette.value.in
  if (key === 'tx') return palette.value.out
  return palette.value.dark ? '#64748b' : '#94a3b8'
}

// 各段在挂载后依次展开，错开 120ms；最后一段的末端做 4px 圆角。
function segmentStyle(key: MeterSegmentKey, width: number, index: number) {
  const last = index === meter.value.segments.length - 1
  return {
    width: ready.value ? `${width}%` : '0%',
    backgroundColor: segmentColor(key),
    backgroundImage: key === 'adjustment' ? STRIPES : undefined,
    borderTopRightRadius: last ? '4px' : undefined,
    borderBottomRightRadius: last ? '4px' : undefined,
    transitionDelay: `${index * 120}ms`
  }
}

const legend = computed(() => {
  const keys = new Set(meter.value.segments.map((segment) => segment.key))
  const items: { key: string; label: string; style: Record<string, string> }[] = []
  if (keys.has('rx')) items.push({ key: 'rx', label: t('traffic.host.direction.inbound'), style: { backgroundColor: palette.value.in } })
  if (keys.has('tx')) items.push({ key: 'tx', label: t('traffic.host.direction.outbound'), style: { backgroundColor: palette.value.out } })
  if (keys.has('adjustment')) {
    items.push({ key: 'adjustment', label: t('traffic.host.legend.adjustment'), style: { backgroundColor: segmentColor('adjustment'), backgroundImage: STRIPES } })
  }
  return items
})

const view = computed(() => {
  const p = plan.value
  const d = period.value
  if (!p || !d) return null
  const base = p.unit_base
  const days = daysUntil(d.end, now.value.getTime())
  const projection = d.projection
  return {
    tone: quotaTone(d, p),
    modeLabel: billingModeLabel(p.billing_mode, t),
    usedPrefix: boundPrefix(d.bound, t),
    quotaText: formatQuotaBytes(d.quota_bytes, base),
    ratio: d.used_ratio,
    percentText: formatRatioPercent(d.used_ratio),
    remainingPrefix: remainingPrefix(d.bound, t),
    remainingText: formatQuotaBytes(d.remaining_bytes, base),
    remainingSub:
      d.overage_bytes !== null
        ? t('traffic.host.overSub')
        : t('traffic.host.remainingSub', { percent: formatRatioPercent(Math.max(0, 1 - d.used_ratio)) }),
    overText: d.overage_bytes !== null ? formatQuotaBytes(d.overage_bytes, base) : '',
    projection: projection
      ? {
          bytes: formatQuotaBytes(projection.billed_bytes, base),
          percent: formatRatioPercent(projection.ratio),
          over: projection.ratio > 1,
          // 已用量是下限时，预测同样只是下限。
          prefix: d.bound === 'lower' ? t('traffic.host.bound.atLeast') : ''
        }
      : null,
    countdown: days !== null && days >= 1 ? t('traffic.host.countdown.days', { days }) : t('traffic.host.countdown.soon'),
    resetSub: `${formatZonedDateTime(d.end, p.timezone)} · ${resetRuleText(p, t)}`,
    alertsEnabled: p.alerts.enabled,
    alertsText: p.alerts.enabled
      ? t(p.alerts.projection ? 'traffic.host.alerts.onWithProjection' : 'traffic.host.alerts.on', {
          warning: p.alerts.warning_percent,
          critical: p.alerts.critical_percent
        })
      : t('traffic.host.alerts.off')
  }
})

const tone = computed(() => TONE_STYLE[view.value?.tone ?? 'normal'])
const toneText = computed(() => tone.value.text)
const trackClass = computed(() => tone.value.track)
const glowClass = computed(() => (mode.value === 'quota' ? tone.value.glow : 'from-primary-50/60 dark:from-primary-900/10'))
const toneChip = computed(() => ({
  className: tone.value.chip,
  icon: tone.value.icon,
  label: t(`traffic.host.tone.${view.value?.tone ?? 'normal'}`)
}))

// 期初补齐量没有方向信息，双向面板只展示实测部分。
const directionNote = computed(() => {
  const d = period.value
  const p = plan.value
  if (!d || !p || d.adjustment_bytes === null) return ''
  return t('traffic.host.direction.measuredOnly', { bytes: formatSignedQuotaBytes(d.adjustment_bytes, p.unit_base) })
})

// 可信度：完整 / 含服务商补齐或校准 / 不完整（下限），附带说明与补齐入口。
const coverage = computed(() => {
  const d = period.value
  const p = plan.value
  const lines: string[] = []
  if (!d || !p) return { label: '', className: '', icon: 'checkCircle' as const, lines, canFill: false }
  const base = p.unit_base
  if (d.coverage === 'partial' && d.observed_from) {
    const key = d.adjustment_kind === 'fill' ? 'traffic.host.coverage.joinedFilled' : 'traffic.host.coverage.joined'
    lines.push(t(key, { time: formatLocalShort(d.observed_from) }))
  }
  if (d.gaps.lost_seconds > 0) lines.push(t('traffic.host.coverage.lost', { duration: formatDuration(d.gaps.lost_seconds, t) }))
  const unallocated = parseByteString(d.gaps.unallocated_bytes)
  if (unallocated !== null && unallocated > 0n) {
    lines.push(t('traffic.host.coverage.unallocated', { bytes: formatQuotaBytes(unallocated, base) }))
  }
  if (d.gaps.minor_restarts > 0) lines.push(t('traffic.host.coverage.minorRestarts', { count: d.gaps.minor_restarts }))
  if (d.bound === 'exact') {
    return { label: t('traffic.host.coverage.exact'), className: 'text-emerald-600 dark:text-emerald-400', icon: 'checkCircle' as const, lines, canFill: false }
  }
  if (d.bound === 'estimate') {
    if (d.adjustment_bytes !== null) {
      const key = d.adjustment_kind === 'override' ? 'traffic.host.coverage.overrideLine' : 'traffic.host.coverage.fillLine'
      lines.unshift(t(key, { bytes: formatSignedQuotaBytes(d.adjustment_bytes, base) }))
    }
    return {
      label: t(d.adjustment_kind === 'override' ? 'traffic.host.coverage.override' : 'traffic.host.coverage.estimate'),
      className: 'text-sky-600 dark:text-sky-400',
      icon: 'infoCircle' as const,
      lines,
      canFill: false
    }
  }
  return {
    label: t('traffic.host.coverage.lower'),
    className: 'text-amber-600 dark:text-amber-400',
    icon: 'exclamationTriangle' as const,
    lines,
    // 期初缺失且尚未补齐时，提供补齐入口。
    canFill: d.coverage === 'partial' && d.adjustment_bytes === null
  }
})

interface Notice {
  key: string
  tone: NoticeTone
  text: string
  action?: { label: string; run: () => void }
}

const notices = computed<Notice[]>(() => {
  const status = props.status
  if (!status) return []
  const list: Notice[] = []
  for (const firing of status.alerts.firing) {
    list.push({
      key: `alert-${firing.kind}-${firing.level}`,
      tone: firing.level === 'critical' ? 'critical' : 'warning',
      text: t(`traffic.host.alerts.${firing.kind}_${firing.level}`)
    })
  }
  const lastSample = status.collection.last_sample_at
  if (stale.value && lastSample && mode.value !== 'needs_host_mount') {
    const seconds = Math.max(0, Math.round((now.value.getTime() - Date.parse(lastSample)) / 1000))
    list.push({
      key: 'stale',
      tone: 'warning',
      text: t('traffic.host.stale', { duration: formatDuration(seconds, t), time: formatLocalShort(lastSample) })
    })
  }
  if (status.interface_selection.ambiguous && (mode.value === 'quota' || mode.value === 'no_plan')) {
    list.push({
      key: 'ambiguous',
      tone: 'warning',
      text: t('traffic.host.ambiguous'),
      action: { label: t(status.plan ? 'traffic.host.actions.edit' : 'traffic.host.actions.setup'), run: () => (status.plan ? emit('edit') : emit('setup')) }
    })
  }
  if (status.unit_hint && status.plan && status.unit_hint.suggested_base !== status.plan.unit_base) {
    const suggested = status.unit_hint.suggested_base
    list.push({
      key: 'unit',
      tone: 'info',
      text: t('traffic.host.unitHint', { samples: status.unit_hint.samples, base: suggested }),
      action: { label: t('traffic.host.unitHintAction', { base: suggested }), run: () => emit('edit', { unit_base: suggested }) }
    })
  }
  const reconcile = status.period?.last_reconcile
  if (reconcile && status.plan) {
    list.push({
      key: 'reconcile',
      tone: 'muted',
      text: t('traffic.host.lastReconcile', {
        time: formatLocalShort(reconcile.at),
        provider: formatQuotaBytes(reconcile.provider_bytes, status.plan.unit_base),
        measured: formatQuotaBytes(reconcile.measured_bytes, status.plan.unit_base),
        deviation: formatDeviation(reconcile.deviation_ratio)
      })
    })
  }
  return list
})
</script>

<style scoped>
.host-rise-enter-active {
  transition:
    opacity 0.45s ease-out,
    transform 0.45s ease-out;
}

.host-rise-enter-from {
  opacity: 0;
  transform: translateY(8px);
}

@media (prefers-reduced-motion: reduce) {
  .host-rise-enter-active {
    transition: none;
  }
}
</style>
