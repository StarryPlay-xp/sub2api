<template>
  <button type="button" class="rounded px-2 py-1 text-left text-xs text-primary-600 hover:bg-primary-50 dark:text-primary-400" :title="t('wireguard.exitIPDetails')" @click="open">
    <ExitIPValue v-if="showSummary" :summary="summary" :loading="summaryLoading" :error="summaryError" />
    <template v-else>{{ t('wireguard.exitIP') }}</template>
  </button>
  <BaseDialog :show="visible" :title="t('wireguard.exitIPDetails')" width="wide" @close="visible = false">
    <p class="mb-4 text-xs text-gray-500">{{ t('wireguard.evidenceHint') }}</p>
    <p v-if="loading" role="status">{{ t('wireguard.loading') }}</p>
    <p v-else-if="error" role="alert" class="text-red-600">{{ error }}</p>
    <p v-else-if="!records.length" class="text-sm text-gray-500">{{ t('wireguard.noAttempts') }}</p>
    <div v-for="item in records" :key="item.attempt_id" class="mb-3 rounded-lg border border-gray-200 p-4 text-sm dark:border-dark-700">
      <div class="flex flex-wrap justify-between gap-2"><strong>{{ item.snapshot.route }}<span v-if="item.snapshot.node_id"> #{{ item.snapshot.node_id }}</span></strong><span>{{ t('wireguard.account') }} #{{ item.account_id }}</span></div>
      <div class="mt-2 font-mono">{{ item.snapshot.family }} · {{ item.snapshot.evidence.ip || t('wireguard.unknown') }}</div>
      <div class="mt-1 text-xs text-gray-500">{{ t(item.snapshot.connection_reused ? 'wireguard.reused' : 'wireguard.newConnection') }} · {{ item.snapshot.evidence.state }} · {{ t('wireguard.version') }} {{ item.snapshot.config_version || '—' }}</div>
      <div v-if="item.snapshot.evidence.observed_at && item.snapshot.evidence.state === 'observed'" class="mt-1 text-xs text-gray-500">{{ t('wireguard.observed') }}: {{ new Date(item.snapshot.evidence.observed_at).toLocaleString() }}</div>
      <div class="mt-1 text-xs text-gray-500">{{ new Date(item.started_at).toLocaleString() }} · {{ t('wireguard.outcome') }}: {{ item.outcome }}</div>
    </div>
  </BaseDialog>
</template>
<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ExitIPValue from '@/components/common/ExitIPValue.vue'
import { attempts, type EgressAttempt, type EgressSummary } from '@/api/admin/wireguard'
const props = defineProps<{ requestId: string; accountId?: number; showSummary?: boolean; summary?: EgressSummary; summaryLoading?: boolean; summaryError?: boolean }>()
const { t } = useI18n()
const visible = ref(false), loading = ref(false), error = ref(''), records = ref<EgressAttempt[]>([])
let generation = 0
watch(() => [props.requestId, props.accountId], () => { generation++; visible.value = false; records.value = [] })
async function open() {
  const current = ++generation
  visible.value = true; loading.value = true; error.value = ''; records.value = []
  try {
    const result = await attempts(props.requestId)
    if (current === generation) records.value = props.accountId ? result.filter(item => item.account_id === props.accountId) : result
  } catch {
    if (current === generation) error.value = t('wireguard.error')
  } finally {
    if (current === generation) loading.value = false
  }
}
</script>
