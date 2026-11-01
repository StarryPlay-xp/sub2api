// WG 节点的状态、速率与峰值展示：带宽页排行、流量页节点区和节点详情共用。
import type { TrafficPeak, TrafficReport, WGDetails } from '@/api/admin/wireguard'
import { toMbps } from '@/utils/bandwidthSeries'
import { isMissingTime } from '@/utils/trafficFormat'

type Translate = (key: string, params?: Record<string, unknown>) => string

export interface NodeStatus {
  key: 'paused' | 'inactive' | 'running' | 'notRunning'
  label: string
  textClass: string
  dotClass: string
}

export function nodeStatus(details: WGDetails | null, t: Translate): NodeStatus {
  if (details?.node.paused) {
    return { key: 'paused', label: t('bandwidth.nodes.paused'), textClass: 'text-red-600 dark:text-red-400', dotClass: 'bg-red-500' }
  }
  if (details && !details.node.active) {
    return { key: 'inactive', label: t('bandwidth.nodes.inactive'), textClass: 'text-gray-500 dark:text-dark-400', dotClass: 'bg-gray-400' }
  }
  if (details && details.rate.state !== 'not_running') {
    return {
      key: 'running',
      label: t('bandwidth.nodes.running'),
      textClass: 'text-emerald-600 dark:text-emerald-400',
      dotClass: 'bg-emerald-500 motion-safe:animate-pulse'
    }
  }
  return { key: 'notRunning', label: t('bandwidth.nodes.notRunning'), textClass: 'text-gray-500 dark:text-dark-400', dotClass: 'bg-gray-400' }
}

export interface NodeRate {
  tx: number
  rx: number
  total: number
}

/** 只有正常约 1 秒采样的速率才可信；采样延迟、等待采样或未运行返回 null。 */
export function nodeCurrentRate(details: WGDetails | null): NodeRate | null {
  const rate = details?.rate
  if (!rate || rate.state !== 'sampled') return null
  return { tx: rate.tx_bytes_per_second, rx: rate.rx_bytes_per_second, total: rate.tx_bytes_per_second + rate.rx_bytes_per_second }
}

/** 速率不可用时的说明文字，可用时返回空串。 */
export function nodeRateNote(details: WGDetails | null, t: Translate): string {
  const state = details?.rate.state
  if (state === 'invalid_interval') return t('bandwidth.nodes.rateDelayed')
  if (state === 'warming_up') return t('bandwidth.nodes.rateWarmingUp')
  if (state === 'not_running') return t('bandwidth.nodes.notRunning')
  return ''
}

export function nodePeak(report: TrafficReport | null, key: 'tx' | 'rx' | 'total'): TrafficPeak | null {
  return report?.peak_history?.peaks?.[key] ?? null
}

/** 自建中转节点每个方向都承担本站的发送和接收，上限与同一秒合计峰值比较。 */
export function nodeUsage(report: TrafficReport | null, limitMbps: number | null | undefined): number | null {
  const peak = nodePeak(report, 'total')
  return peak && limitMbps ? toMbps(peak.bytes_per_second) / limitMbps : null
}

/** 峰值缺失的原因：没有记录、升级前的历史，或没有有效的 1 秒样本。 */
export function nodePeakNote(report: TrafficReport | null, t: Translate): string {
  if (!report) return ''
  if (!report.has_records) return t('bandwidth.nodes.noRecords')
  return report.peak_history ? t('bandwidth.host.noValidSample') : t('bandwidth.nodes.legacy')
}

/** 实际观察时长：首末有效样本与所选期间的交集；没有样本质量信息时返回 null，不拿范围冒充。 */
export function nodeObservedSeconds(report: TrafficReport | null): number | null {
  const quality = report?.peak_history?.quality
  if (!report || !quality || isMissingTime(quality.first_sample_at) || isMissingTime(quality.last_sample_at)) return null
  const start = Math.max(Date.parse(report.start), Date.parse(quality.first_sample_at as string))
  const end = Math.min(Date.parse(report.end), Date.parse(quality.last_sample_at as string))
  const seconds = (end - start) / 1000
  return Number.isFinite(seconds) && seconds > 0 ? seconds : null
}

export function nodeWarning(report: TrafficReport | null, t: Translate): string {
  if (!report) return ''
  const invalid = report.peak_history?.quality?.invalid_samples ?? 0
  if (invalid > 0) return t('bandwidth.nodes.invalidSamples', { count: invalid })
  if (report.incomplete) return t('bandwidth.nodes.incomplete')
  return ''
}
