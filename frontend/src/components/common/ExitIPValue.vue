<template>
  <span v-if="loading">{{ t('wireguard.loading') }}</span>
  <span v-else-if="error">{{ t('wireguard.exitIPLoadError') }}</span>
  <template v-else-if="summary?.ips.length">
    <span v-for="ip in summary.ips" :key="ip" class="block whitespace-nowrap font-mono">{{ ip }}</span>
    <span v-if="summary.unobserved || summary.expired" class="block text-amber-600">{{ t('wireguard.exitIPPartial') }}</span>
  </template>
  <span v-else-if="summary?.expired">{{ t('wireguard.exitIPExpired') }}</span>
  <span v-else-if="summary?.recorded">{{ t('wireguard.exitIPUnobserved') }}</span>
  <span v-else>{{ t('wireguard.exitIPMissing') }}</span>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { UsageExitIP } from '@/types'
defineProps<{ summary?: UsageExitIP; loading?: boolean; error?: boolean }>()
const { t } = useI18n()
</script>
