<template>
  <div
    class="mt-5 rounded-2xl bg-amber-50/70 p-5 ring-1 ring-amber-200/70 dark:bg-amber-950/20 dark:ring-amber-900/40"
    data-testid="host-mount-guide"
  >
    <div class="flex items-start gap-3">
      <Icon name="exclamationTriangle" size="md" class="mt-0.5 shrink-0 text-amber-500 dark:text-amber-400" />
      <div class="min-w-0 flex-1">
        <h4 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('traffic.host.mount.title') }}</h4>
        <p class="mt-1 text-sm leading-relaxed text-gray-600 dark:text-dark-300">{{ t('traffic.host.mount.body') }}</p>

        <ol class="mt-4 space-y-3 text-sm text-gray-700 dark:text-dark-200">
          <li class="flex gap-2.5">
            <span class="flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-white text-xs font-semibold text-amber-700 ring-1 ring-amber-200 dark:bg-dark-800 dark:text-amber-300 dark:ring-amber-900/60">1</span>
            <div class="min-w-0 flex-1">
              <p>{{ t('traffic.host.mount.step1') }}</p>
              <div class="relative mt-2">
                <pre
                  class="overflow-x-auto rounded-xl bg-gray-900 px-4 py-3 font-mono text-xs leading-relaxed text-gray-100 dark:bg-black/40"
                  data-testid="host-mount-snippet"
                >{{ COMPOSE_SNIPPET }}</pre>
                <button
                  type="button"
                  class="absolute right-2 top-2 inline-flex items-center gap-1 rounded-lg bg-white/10 px-2 py-1 text-[11px] font-medium text-gray-100 transition-colors hover:bg-white/20 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-400/60"
                  data-testid="host-mount-copy"
                  @click="copy"
                >
                  <Icon :name="copied ? 'check' : 'copy'" size="xs" class="h-3.5 w-3.5" />
                  {{ t('traffic.host.actions.copy') }}
                </button>
              </div>
            </div>
          </li>
          <li class="flex gap-2.5">
            <span class="flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-white text-xs font-semibold text-amber-700 ring-1 ring-amber-200 dark:bg-dark-800 dark:text-amber-300 dark:ring-amber-900/60">2</span>
            <p class="min-w-0 flex-1">{{ t('traffic.host.mount.step2') }}</p>
          </li>
          <li class="flex gap-2.5">
            <span class="flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-white text-xs font-semibold text-amber-700 ring-1 ring-amber-200 dark:bg-dark-800 dark:text-amber-300 dark:ring-amber-900/60">3</span>
            <p class="min-w-0 flex-1">{{ t('traffic.host.mount.step3') }}</p>
          </li>
        </ol>

        <div class="mt-4 flex flex-col gap-2 border-t border-amber-200/60 pt-3 text-xs text-gray-500 dark:border-amber-900/40 dark:text-dark-400 sm:flex-row sm:items-center sm:justify-between">
          <p>{{ t('traffic.host.mount.note') }}</p>
          <a
            :href="DOCS_URL"
            target="_blank"
            rel="noopener noreferrer"
            class="inline-flex shrink-0 items-center gap-1 font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300"
          >
            {{ t('traffic.host.actions.docs') }}
            <Icon name="externalLink" size="xs" class="h-3.5 w-3.5" />
          </a>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { useClipboard } from '@/composables/useClipboard'

// 只读挂载宿主机 /sys：容器默认只能看到自己的虚拟网卡。不挂 /proc，避免暴露宿主机进程信息。
const COMPOSE_SNIPPET = `services:
  sub2api:
    volumes:
      - type: bind
        source: /sys
        target: /host/sys
        read_only: true
        bind:
          create_host_path: false`

const DOCS_URL = 'https://github.com/StarryPlay-xp/sub2api/blob/main/deploy/HOST_TRAFFIC_CN.md'

const { t } = useI18n()
const { copied, copyToClipboard } = useClipboard()

function copy() {
  void copyToClipboard(COMPOSE_SNIPPET, t('traffic.host.mount.copied'))
}
</script>
