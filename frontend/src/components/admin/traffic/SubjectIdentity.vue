<template>
  <div class="min-w-0">
    <div class="flex min-w-0 items-center gap-1.5">
      <span
        class="truncate font-medium text-gray-900 dark:text-white"
        :class="[deleted ? 'text-gray-500 line-through decoration-gray-300 dark:text-dark-400 dark:decoration-dark-500' : '', titleClass]"
        :title="title"
      >
        {{ title }}
      </span>
      <span v-if="identity.api_key_deleted" class="badge badge-gray shrink-0 text-[10px]">{{ t('traffic.subject.keyDeleted') }}</span>
      <span v-if="identity.user_deleted" class="badge badge-gray shrink-0 text-[10px]">{{ t('traffic.subject.userDeleted') }}</span>
    </div>
    <p v-if="secondary" class="mt-0.5 truncate text-xs text-gray-500 dark:text-dark-400" :title="secondary">{{ secondary }}</p>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { TrafficSubjectIdentity } from '@/api/admin/trafficSubjects'
import { isSubjectDeleted, subjectTitle, userTitle } from './subjectFormat'

const props = withDefaults(
  defineProps<{
    identity: TrafficSubjectIdentity
    // 只看某个用户名下的 Key 时，副标题不再重复用户。
    hideUser?: boolean
    activeKeys?: number | null
    titleClass?: string
  }>(),
  { hideUser: false, activeKeys: null, titleClass: 'text-sm' }
)

const { t } = useI18n()

const title = computed(() => subjectTitle(props.identity))
const deleted = computed(() => isSubjectDeleted(props.identity))

const secondary = computed(() => {
  const value = props.identity
  const parts: string[] = []
  if (value.subject_type === 'user') {
    if (value.user_email && value.username) parts.push(value.username)
    parts.push(`ID ${value.user_id}`)
    if (typeof props.activeKeys === 'number') parts.push(t('traffic.subject.activeKeys', { count: props.activeKeys }))
  } else {
    if (!props.hideUser) parts.push(userTitle(value))
    if (value.group_name) parts.push(value.group_name)
    parts.push(`ID ${value.api_key_id ?? '—'}`)
  }
  return parts.join(' · ')
})
</script>
