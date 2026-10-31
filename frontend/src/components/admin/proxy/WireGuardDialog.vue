<template>
  <BaseDialog :show="true" :title="t('wireguard.title')" width="wide" @close="emit('close')">
    <div class="space-y-5">
      <p v-if="error" role="alert" class="rounded-lg bg-red-50 p-3 text-sm text-red-700 dark:bg-red-950 dark:text-red-200">{{ error }}</p>
      <p v-if="!keyConfigured" class="rounded-lg bg-amber-50 p-3 text-sm text-amber-800 dark:bg-amber-950 dark:text-amber-200">{{ t('wireguard.keyMissing') }}</p>
      <template v-if="data">
        <div class="flex flex-wrap items-center gap-2">
          <span class="badge" :class="data.node.paused || !data.node.active ? 'badge-danger' : 'badge-success'">{{ t(data.node.paused ? 'wireguard.blocked' : data.node.active ? 'wireguard.ready' : 'wireguard.inactive') }}</span>
          <span class="text-xs text-gray-500">{{ t('wireguard.version') }} {{ data.node.config_version }}</span>
          <button class="btn btn-secondary" :disabled="busy" @click="changeState">{{ t(data.node.paused ? 'wireguard.resume' : 'wireguard.pause') }}</button>
          <button class="btn btn-secondary" :disabled="busy || data.node.paused || !data.node.active" @click="verify('ipv4')">{{ t('wireguard.probe4') }}</button>
          <button class="btn btn-secondary" :disabled="busy || data.node.paused || !data.node.active" @click="verify('ipv6')">{{ t('wireguard.probe6') }}</button>
        </div>
        <p class="flex items-start gap-1.5 text-xs text-gray-500 dark:text-dark-400">
          <Icon name="infoCircle" size="xs" class="mt-px h-3.5 w-3.5 shrink-0" />
          {{ t('wireguard.failureHint') }}
        </p>
        <WireGuardTrafficPanel
          :node="data.node"
          :rate="data.rate"
          :report="trafficReport"
          :selected-range="selectedRange"
          :loading="trafficLoading"
          :error="trafficError"
          :telemetry-gap="Boolean(data.dropped_attempt_records)"
          :retained-from="trafficRetainedFrom"
          @select="handleTrafficSelect"
        />
        <div class="space-y-2">
          <h4 class="font-semibold">{{ t('wireguard.evidence') }}</h4>
          <p class="text-xs text-gray-500">{{ t('wireguard.evidenceHint') }}</p>
          <p v-if="!data.observations.length" class="text-sm text-gray-500">{{ t('wireguard.noEvidence') }}</p>
          <div v-for="item in data.observations" :key="item.family" class="rounded-lg border border-gray-200 p-3 text-sm dark:border-dark-700">
            <span class="font-mono">{{ item.family }} · {{ item.ip || '—' }}</span>
            <span class="ml-2" :class="item.state === 'observed' ? 'text-green-600' : 'text-amber-600'">{{ t(item.state === 'observed' ? 'wireguard.observed' : 'wireguard.stale') }}</span>
            <div class="text-xs text-gray-500">{{ date(item.observed_at) }} · v{{ item.version }}</div>
          </div>
        </div>
      </template>
      <div v-else class="animate-pulse space-y-4" aria-hidden="true">
        <div class="flex gap-2">
          <div class="h-6 w-20 rounded-full bg-gray-200 dark:bg-dark-700"></div>
          <div class="h-6 w-16 rounded-full bg-gray-200 dark:bg-dark-700"></div>
        </div>
        <div class="rounded-2xl border border-gray-200 p-4 dark:border-dark-700">
          <div class="mb-3 h-4 w-28 rounded bg-gray-200 dark:bg-dark-700"></div>
          <div class="grid gap-3 md:grid-cols-3">
            <div class="h-20 rounded-xl bg-gray-100 dark:bg-dark-700/60"></div>
            <div class="h-20 rounded-xl bg-gray-100 dark:bg-dark-700/60"></div>
            <div class="h-20 rounded-xl bg-gray-100 dark:bg-dark-700/60"></div>
          </div>
          <div class="mt-3 h-48 rounded-xl bg-gray-100 dark:bg-dark-700/60"></div>
        </div>
      </div>
      <form class="space-y-3 border-t border-gray-200 pt-4 dark:border-dark-700" @submit.prevent="saveNode">
        <div><label for="wg-name" class="input-label">{{ t('wireguard.name') }}</label><input id="wg-name" v-model="name" class="input" required maxlength="100" /></div>
        <div><label for="wg-config" class="input-label">{{ t(id ? 'wireguard.replace' : 'wireguard.configuration') }}</label><textarea id="wg-config" v-model="configuration" class="input min-h-36 font-mono text-xs" :required="!id" maxlength="32768" autocomplete="off" autocapitalize="off" spellcheck="false" /><p class="mt-1 text-xs text-gray-500">{{ t('wireguard.configurationHint') }} {{ t('wireguard.peer') }}</p></div>
        <div><label for="wg-expiry" class="input-label">{{ t('wireguard.expiry') }}</label><input id="wg-expiry" v-model="expiry" type="datetime-local" class="input" /></div>
        <button class="btn btn-primary" :disabled="busy || !keyConfigured">{{ t('wireguard.save') }}</button>
        <p v-if="saved" role="status" class="text-sm text-green-600">{{ t('wireguard.created') }}</p>
      </form>
    </div>
  </BaseDialog>
</template>
<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import WireGuardTrafficPanel from './WireGuardTrafficPanel.vue'
import * as api from '@/api/admin/wireguard'
const props = defineProps<{ nodeId: number | null }>()
const emit = defineEmits<{ close: []; changed: [] }>()
const { t } = useI18n()
const id = ref(props.nodeId), data = ref<api.WGDetails | null>(null)
const name = ref(''), configuration = ref(''), expiry = ref(''), version = ref<number>()
const keyConfigured = ref(true), busy = ref(false), saved = ref(false), error = ref('')
const selectedRange = ref<api.TrafficRange>('24h'), trafficReport = ref<api.TrafficReport | null>(null)
const trafficSelection = ref<api.TrafficSelection>({ range: '24h' })
const trafficError = ref(''), trafficLoading = ref(false), trafficRetainedFallback = ref('')
const controller = new AbortController()
let trafficController: AbortController | null = null
let timer: ReturnType<typeof setTimeout> | undefined
let trafficSeq = 0, lastTrafficAt = 0, disposed = false
const TRAFFIC_REFRESH_MS = 15000
const trafficRetainedFrom = computed(() => trafficReport.value?.interval === 'hour' ? trafficReport.value.retained_from : (trafficRetainedFallback.value || undefined))
const date = (v?: string) => v ? new Date(v).toLocaleString() : '—'
function fillForm() { if (!data.value) return; name.value = data.value.node.name; version.value = data.value.node.config_version; const v = data.value.node.expires_at; expiry.value = v ? new Date(new Date(v).getTime() - new Date(v).getTimezoneOffset() * 60000).toISOString().slice(0, 16) : '' }
async function refresh() { if (!id.value || disposed) return; data.value = await api.details(id.value, 'minute', controller.signal) }
function trafficMessage(e: unknown) {
  const value = e as { status?: number; reason?: string; metadata?: { retained_from?: string } }
  const retained = value.metadata?.retained_from ? new Date(value.metadata.retained_from).toLocaleString() : ''
  switch (value.reason) {
    case 'TRAFFIC_RANGE_INVALID': return t('wireguard.trafficRangeInvalid')
    case 'TRAFFIC_INTERVAL_INVALID': return t('wireguard.trafficIntervalInvalid')
    case 'TRAFFIC_DATE_INVALID': return t('wireguard.trafficDateInvalid')
    case 'TRAFFIC_BOUNDARY_UNALIGNED': return t('wireguard.trafficBoundaryUnaligned')
    case 'TRAFFIC_RETENTION_EXCEEDED': return retained ? t('wireguard.trafficRetentionExceeded', { date: retained }) : t('wireguard.trafficRetentionExceededGeneric')
  }
  if (value.status === 403) return t('wireguard.trafficForbidden')
  if (value.status === 503) return t('wireguard.trafficUnavailable')
  return t('wireguard.trafficLoadError')
}
async function loadTraffic(selection: api.TrafficSelection, reset: boolean) {
  if (!id.value || disposed) return
  const seq = ++trafficSeq
  trafficController?.abort()
  const requestController = new AbortController()
  trafficController = requestController
  trafficSelection.value = selection
  selectedRange.value = selection.range
  if (reset) { trafficReport.value = null; trafficError.value = '' }
  trafficLoading.value = true
  try {
    const params: api.TrafficParams = { range: selection.range }
    if (selection.range === 'custom') {
      if (selection.start) params.start = selection.start
      if (selection.end) params.end = selection.end
    }
    const result = await api.traffic(id.value, params, requestController.signal)
    if (seq !== trafficSeq || requestController.signal.aborted || disposed) return
    trafficReport.value = result
    trafficError.value = ''
    trafficRetainedFallback.value = ''
  } catch (e) {
    if (seq !== trafficSeq || requestController.signal.aborted || disposed) return
    trafficReport.value = null
    trafficError.value = trafficMessage(e)
    const retained = (e as { metadata?: { retained_from?: string } }).metadata?.retained_from
    if (selection.range === 'custom' && retained) trafficRetainedFallback.value = retained
  } finally {
    if (seq === trafficSeq) {
      trafficLoading.value = false
      // 失败也记录尝试时间，避免轮询在错误状态下每 2 秒重复请求。
      lastTrafficAt = Date.now()
    }
  }
}
function handleTrafficSelect(selection: api.TrafficSelection) { void loadTraffic(selection, true) }
function message(e: unknown) { const value = e as { response?: { data?: { message?: string } } }; return value.response?.data?.message || t('wireguard.error') }
async function run(action: () => Promise<void>) { if (busy.value) return; busy.value = true; error.value = ''; try { await action() } catch (e) { if (!disposed) error.value = message(e) } finally { busy.value = false } }
async function saveNode() { await run(async () => { const created = !id.value; const node = await api.save(id.value, { name: name.value, config: configuration.value, config_version: version.value, expires_at: expiry.value ? new Date(expiry.value).toISOString() : null }); configuration.value = ''; id.value = node.id; version.value = node.config_version; saved.value = true; await refresh(); fillForm(); if (created) await loadTraffic({ range: '24h' }, true); emit('changed') }) }
async function changeState() { await run(async () => { if (!data.value || !id.value) return; await api.setPaused(id.value, data.value.node.config_version, !data.value.node.paused); await refresh(); fillForm(); emit('changed') }) }
async function verify(family: string) { await run(async () => { if (!id.value) return; await api.probe(id.value, family); await refresh() }) }
async function poll() { if (disposed) return; if (!busy.value) { try { await refresh() } catch { if (!disposed) error.value = t('wireguard.error') } } if (!disposed && !trafficLoading.value && Date.now() - lastTrafficAt >= TRAFFIC_REFRESH_MS) { void loadTraffic(trafficSelection.value, false) } if (!disposed) timer = setTimeout(poll, 2000) }
onMounted(async () => { await run(async () => { keyConfigured.value = (await api.capabilities()).key_configured; await refresh(); fillForm() }); if (id.value) await loadTraffic({ range: '24h' }, true); if (!disposed) timer = setTimeout(poll, 2000) })
watch(() => props.nodeId, (value) => {
  if (value === id.value) return
  id.value = value
  data.value = null
  trafficReport.value = null
  trafficError.value = ''
  trafficRetainedFallback.value = ''
  trafficSelection.value = { range: '24h' }
  selectedRange.value = '24h'
  trafficSeq++
  trafficController?.abort()
  if (!value) return
  void run(async () => { await refresh(); fillForm() })
  void loadTraffic({ range: '24h' }, true)
})
onUnmounted(() => { disposed = true; configuration.value = ''; controller.abort(); trafficController?.abort(); if (timer) clearTimeout(timer) })
</script>
