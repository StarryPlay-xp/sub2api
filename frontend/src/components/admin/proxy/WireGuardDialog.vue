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
        <p class="text-sm text-gray-500">{{ t('wireguard.failureHint') }}</p>
        <div class="grid grid-cols-2 gap-3 rounded-xl bg-gray-50 p-4 dark:bg-dark-800">
          <div><div class="text-xs text-gray-500">{{ t('wireguard.sent') }}</div><strong>{{ bytes(data.node.tx_bytes) }}</strong><div class="text-sm">{{ rate(data.rate.tx_bytes_per_second) }}</div></div>
          <div><div class="text-xs text-gray-500">{{ t('wireguard.received') }}</div><strong>{{ bytes(data.node.rx_bytes) }}</strong><div class="text-sm">{{ rate(data.rate.rx_bytes_per_second) }}</div></div>
          <p class="col-span-2 text-xs text-gray-500">{{ t('wireguard.saved') }}: {{ date(data.node.saved_at) }} · {{ t(data.rate.state === 'not_running' ? 'wireguard.notRunning' : 'wireguard.running') }}</p>
        </div>
        <p class="text-xs text-gray-500">{{ t('wireguard.basis') }}</p>
        <p v-if="data.node.incomplete" role="status" class="text-sm text-amber-600">{{ t('wireguard.incomplete') }}</p>
        <p v-if="data.dropped_attempt_records" class="text-sm text-amber-600">{{ t('wireguard.telemetryGap') }}</p>
        <div class="space-y-2">
          <label class="input-label" for="wg-history">{{ t('wireguard.history') }}</label>
          <select id="wg-history" v-model="interval" class="input" @change="refresh"><option value="minute">{{ t('wireguard.minute') }}</option><option value="hour">{{ t('wireguard.hour') }}</option></select>
          <div class="h-44"><Line v-if="data.history.length" :data="chartData" :options="chartOptions" /></div>
        </div>
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
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Line } from 'vue-chartjs'
import { Chart as ChartJS, CategoryScale, LinearScale, PointElement, LineElement, Tooltip, Legend, type ChartOptions } from 'chart.js'
import BaseDialog from '@/components/common/BaseDialog.vue'
import * as api from '@/api/admin/wireguard'
ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, Tooltip, Legend)
const props = defineProps<{ nodeId: number | null }>()
const emit = defineEmits<{ close: []; changed: [] }>()
const { t } = useI18n()
const id = ref(props.nodeId), data = ref<api.WGDetails | null>(null)
const name = ref(''), configuration = ref(''), expiry = ref(''), version = ref<number>()
const keyConfigured = ref(true), busy = ref(false), saved = ref(false), error = ref(''), interval = ref('minute')
const controller = new AbortController()
let timer: ReturnType<typeof setTimeout> | undefined
let disposed = false
const date = (v?: string) => v ? new Date(v).toLocaleString() : '—'
function bytes(v: string | number) { const n = Number(v); return n < 1024 ? `${n.toFixed(0)} B` : n < 1048576 ? `${(n / 1024).toFixed(2)} KiB` : n < 1073741824 ? `${(n / 1048576).toFixed(2)} MiB` : `${(n / 1073741824).toFixed(2)} GiB` }
const rate = (n: number) => data.value?.rate.state === 'not_running' ? '—' : `${bytes(n)}/s`
const chartData = computed(() => ({ labels: data.value?.history.map(b => date(b.start)) || [], datasets: [{ label: t('wireguard.sent'), data: data.value?.history.map(b => Number(b.tx_bytes) / 1024) || [], borderColor: '#2563eb', pointRadius: 0 }, { label: t('wireguard.received'), data: data.value?.history.map(b => Number(b.rx_bytes) / 1024) || [], borderColor: '#10b981', pointRadius: 0 }] }))
const chartOptions: ChartOptions<'line'> = { responsive: true, maintainAspectRatio: false, animation: false, scales: { x: { ticks: { maxTicksLimit: 6 } }, y: { beginAtZero: true, title: { display: true, text: 'KiB' } } } }
function fillForm() { if (!data.value) return; name.value = data.value.node.name; version.value = data.value.node.config_version; const v = data.value.node.expires_at; expiry.value = v ? new Date(new Date(v).getTime() - new Date(v).getTimezoneOffset() * 60000).toISOString().slice(0, 16) : '' }
async function refresh() { if (!id.value || disposed) return; data.value = await api.details(id.value, interval.value, controller.signal) }
function message(e: unknown) { const value = e as { response?: { data?: { message?: string } } }; return value.response?.data?.message || t('wireguard.error') }
async function run(action: () => Promise<void>) { if (busy.value) return; busy.value = true; error.value = ''; try { await action() } catch (e) { if (!disposed) error.value = message(e) } finally { busy.value = false } }
async function saveNode() { await run(async () => { const node = await api.save(id.value, { name: name.value, config: configuration.value, config_version: version.value, expires_at: expiry.value ? new Date(expiry.value).toISOString() : null }); configuration.value = ''; id.value = node.id; version.value = node.config_version; saved.value = true; await refresh(); fillForm(); emit('changed') }) }
async function changeState() { await run(async () => { if (!data.value || !id.value) return; await api.setPaused(id.value, data.value.node.config_version, !data.value.node.paused); await refresh(); fillForm(); emit('changed') }) }
async function verify(family: string) { await run(async () => { if (!id.value) return; await api.probe(id.value, family); await refresh() }) }
async function poll() { if (disposed) return; if (!busy.value) { try { await refresh() } catch { if (!disposed) error.value = t('wireguard.error') } } if (!disposed) timer = setTimeout(poll, 2000) }
onMounted(async () => { await run(async () => { keyConfigured.value = (await api.capabilities()).key_configured; await refresh(); fillForm() }); if (!disposed) timer = setTimeout(poll, 2000) })
onUnmounted(() => { disposed = true; configuration.value = ''; controller.abort(); if (timer) clearTimeout(timer) })
</script>
