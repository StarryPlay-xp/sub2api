<template>
  <div
    class="inline-flex shrink-0 self-start rounded-xl bg-gray-100 p-1 dark:bg-dark-900/60"
    role="group"
    :aria-label="t('traffic.subject.switchLabel')"
  >
    <button
      v-for="option in options"
      :key="option.value"
      type="button"
      class="inline-flex items-center gap-1.5 rounded-lg px-3 py-1 text-xs font-medium transition-colors duration-150 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/40"
      :class="
        modelValue === option.value
          ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-700 dark:text-white'
          : 'text-gray-500 hover:text-gray-800 dark:text-dark-400 dark:hover:text-dark-100'
      "
      :aria-pressed="modelValue === option.value"
      :data-testid="`subject-${option.value}`"
      @click="emit('update:modelValue', option.value)"
    >
      <Icon :name="option.icon" size="xs" class="h-3.5 w-3.5" />
      {{ option.label }}
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import type { TrafficSubjectType } from '@/api/admin/trafficSubjects'

defineProps<{ modelValue: TrafficSubjectType }>()
const emit = defineEmits<{ 'update:modelValue': [value: TrafficSubjectType] }>()

const { t } = useI18n()

const options = computed(() => [
  { value: 'user' as const, label: t('traffic.subject.user'), icon: 'user' as const },
  { value: 'api_key' as const, label: t('traffic.subject.api_key'), icon: 'key' as const }
])
</script>
