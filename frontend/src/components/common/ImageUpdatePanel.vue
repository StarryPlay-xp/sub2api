<template>
  <div class="space-y-3 text-sm" aria-live="polite">
    <p class="text-xs text-gray-500 dark:text-dark-400">{{ t('version.imageHint') }}</p>
    <p v-if="disconnected" class="text-amber-600">{{ t('version.imageReconnecting') }}</p>
    <p v-else-if="status" :class="failed ? 'text-red-600' : 'text-gray-700 dark:text-dark-200'">
      {{ phaseLabel }}
    </p>
    <p v-if="status?.error || error" class="break-words text-xs text-red-600">
      {{ status?.error || error }}
    </p>
    <p v-if="status?.phase === 'needs_attention'" class="text-xs text-gray-500">
      {{ t('version.imageRecovery') }}
    </p>
    <button
      v-if="hasUpdate && status?.enabled && !active && status.phase !== 'needs_attention'"
      type="button"
      :disabled="submitting || disconnected"
      class="w-full rounded-lg bg-primary-500 px-3 py-2 text-white disabled:opacity-50"
      @click="start"
    >
      {{ t('version.imageInstall', { version: latestVersion }) }}
    </button>
    <p v-if="hasUpdate && status?.enabled && !active" class="text-xs text-gray-500">
      {{ t('version.imageInterruption') }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { getImageUpdateStatus, startImageUpdate, type ImageUpdateStatus } from '@/api/admin/system'

const props = defineProps<{ hasUpdate: boolean; latestVersion: string; currentVersion: string }>()
const emit = defineEmits<{ updated: [] }>()
const { t, te } = useI18n()
const status = ref<ImageUpdateStatus | null>(null)
const error = ref('')
const disconnected = ref(false)
const submitting = ref(false)
const active = computed(() => !!status.value && ['queued', 'preflight', 'pulling', 'stopping', 'backing_up', 'recreating', 'verifying'].includes(status.value.phase))
const failed = computed(() => ['failed', 'needs_attention'].includes(status.value?.phase || ''))
const phaseLabel = computed(() => {
  const key = `version.imagePhases.${status.value?.phase || 'idle'}`
  return te(key) ? t(key) : t('version.imagePhases.unknown')
})
let timer: ReturnType<typeof setTimeout> | undefined
let disposed = false
let announced = ''

async function poll() {
  try {
    const value = await getImageUpdateStatus()
    if (disposed) return
    status.value = value
    disconnected.value = false
    error.value = ''
    if (value.phase === 'succeeded' && value.version !== props.currentVersion && value.id !== announced) {
      announced = value.id || ''
      emit('updated')
    }
  } catch {
    if (!disposed) disconnected.value = true
  } finally {
    if (!disposed) timer = setTimeout(poll, active.value || disconnected.value ? 3000 : 15000)
  }
}

async function start() {
  if (submitting.value || active.value) return
  submitting.value = true
  error.value = ''
  try {
    status.value = await startImageUpdate()
  } catch {
    // The host may have accepted the job before a connection was lost. Poll
    // durable state; never report success merely because the request returned.
    error.value = t('version.imageSubmitUncertain')
  } finally {
    submitting.value = false
    if (timer) clearTimeout(timer)
    if (!disposed) void poll()
  }
}

onMounted(() => void poll())
onBeforeUnmount(() => { disposed = true; if (timer) clearTimeout(timer) })
</script>
