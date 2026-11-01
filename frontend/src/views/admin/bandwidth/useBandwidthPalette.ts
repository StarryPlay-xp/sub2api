import { computed, ref } from 'vue'
import { useMutationObserver } from '@vueuse/core'

// 与 WG 历史卡片同一组配色：出站青色、入站紫色。
// 已用 dataviz 校验脚本验证：浅色 #0d9488/#6250d6 色盲 ΔE 19.4，深色 #0d9488/#9085e9 色盲 ΔE 13.8，均高于 8。
export function useBandwidthPalette() {
  const root = typeof document !== 'undefined' ? document.documentElement : null
  const dark = ref(Boolean(root?.classList.contains('dark')))
  if (root) {
    useMutationObserver(
      root,
      () => {
        dark.value = root.classList.contains('dark')
      },
      { attributes: true, attributeFilter: ['class'] }
    )
  }
  return computed(() => ({
    dark: dark.value,
    in: dark.value ? '#9085e9' : '#6250d6',
    out: '#0d9488',
    inHover: dark.value ? '#a59cf0' : '#4f3fc0',
    outHover: dark.value ? '#14b8a6' : '#0f766e',
    limit: dark.value ? '#cbd5e1' : '#475569',
    grid: dark.value ? 'rgba(148, 163, 184, 0.14)' : 'rgba(15, 23, 42, 0.06)',
    ticks: dark.value ? '#94a3b8' : '#64748b',
    gapFill: dark.value ? 'rgba(148, 163, 184, 0.06)' : 'rgba(100, 116, 139, 0.05)',
    tooltipBg: dark.value ? '#0f172a' : '#ffffff',
    tooltipTitle: dark.value ? '#f8fafc' : '#0f172a',
    tooltipBody: dark.value ? '#cbd5e1' : '#475569',
    tooltipBorder: dark.value ? '#334155' : '#e2e8f0'
  }))
}
