import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import type { TrafficRange } from '@/api/admin/wireguard'

export type PresetRange = Exclude<TrafficRange, 'custom'>

export const PRESET_RANGES: PresetRange[] = ['24h', '7d', '30d', '90d']

export function isPresetRange(value: unknown): value is PresetRange {
  return typeof value === 'string' && (PRESET_RANGES as string[]).includes(value)
}

/** 带宽、流量两页共用的统计范围：写入地址栏，刷新或分享链接时保持。 */
export function usePresetRange() {
  const { t } = useI18n()
  const route = useRoute()
  const router = useRouter()

  const range = ref<PresetRange>(isPresetRange(route.query.range) ? route.query.range : '24h')

  watch(range, (value) => {
    const query = { ...route.query }
    if (value === '24h') delete query.range
    else query.range = value
    void router.replace({ query })
  })

  const options = computed(() => PRESET_RANGES.map((value) => ({ value, label: t(`bandwidth.range.${value}`) })))

  return { range, options }
}
