// 用户 / API Key 流量的展示换算：字节仍按十进制字符串用 BigInt 处理，速率按 B/s 浮点数处理。
import type {
  TrafficSubjectIdentity,
  TrafficSubjectStats,
  TrafficSubjectWindow
} from '@/api/admin/trafficSubjects'
import type { TrafficPeak } from '@/api/admin/wireguard'
import { toMbps } from '@/utils/bandwidthSeries'
import { formatByteCount, isMissingTime } from '@/utils/trafficFormat'

/** 解析有符号十进制整数字符串；无法解析时返回 null，不当作 0。 */
export function parseSignedBytes(value: string | null | undefined): bigint | null {
  if (typeof value !== 'string' || !/^-?\d+$/.test(value.trim())) return null
  return BigInt(value.trim())
}

export function subjectKey(identity: TrafficSubjectIdentity): string {
  return identity.subject_type === 'user' ? `user:${identity.user_id}` : `api_key:${identity.api_key_id}`
}

/** 用户显示邮箱，没有邮箱时显示用户名；Key 显示名称，没有名称时显示编号。 */
export function subjectTitle(identity: TrafficSubjectIdentity): string {
  if (identity.subject_type === 'api_key') {
    return identity.api_key_name?.trim() || `#${identity.api_key_id ?? '—'}`
  }
  return identity.user_email?.trim() || identity.username?.trim() || `#${identity.user_id}`
}

export function userTitle(identity: TrafficSubjectIdentity): string {
  return identity.user_email?.trim() || identity.username?.trim() || `#${identity.user_id}`
}

export function isSubjectDeleted(identity: TrafficSubjectIdentity): boolean {
  return identity.user_deleted || Boolean(identity.api_key_deleted)
}

/** 0–1 比例显示为百分比：小于 10% 保留一位小数，极小的非零值显示 “<0.1%”。 */
export function formatShare(value: number | null | undefined): string {
  if (typeof value !== 'number' || !Number.isFinite(value) || value < 0) return '—'
  const percent = value * 100
  if (percent > 0 && percent < 0.1) return '<0.1%'
  return `${percent < 10 ? parseFloat(percent.toFixed(1)) : Math.round(percent)}%`
}

export function formatCount(value: number | null | undefined): string {
  return typeof value === 'number' && Number.isFinite(value) ? value.toLocaleString() : '—'
}

/**
 * 分维度数据的实际观察时长：所选期间与归因开始时间、数据截止时间的交集。
 * 升级前没有分维度数据，平均速率只按开始统计之后的时长折算。
 */
export function attributionSeconds(window: TrafficSubjectWindow | null | undefined): number | null {
  if (!window || isMissingTime(window.attribution_started_at)) return null
  const start = Math.max(Date.parse(window.start), Date.parse(window.attribution_started_at as string))
  const end = Math.min(Date.parse(window.end), Date.parse(window.as_of))
  if (!Number.isFinite(start) || !Number.isFinite(end)) return null
  const seconds = (end - start) / 1000
  return seconds > 0 ? seconds : null
}

/** 期间平均速率 = 总量 ÷ 观察时长；与按总量排序的先后一致。 */
export function averageRate(stats: TrafficSubjectStats, window: TrafficSubjectWindow | null | undefined): number | null {
  const seconds = attributionSeconds(window)
  const total = parseSignedBytes(stats.total_bytes)
  if (seconds === null || total === null || total < 0n) return null
  return Number(total) / seconds
}

/** 归因从所选期间开始之后才有数据时，返回开始时间用于提示。 */
export function lateAttributionStart(window: TrafficSubjectWindow | null | undefined): string | null {
  if (!window || isMissingTime(window.attribution_started_at)) return null
  const started = Date.parse(window.attribution_started_at as string)
  return started - Date.parse(window.start) > 60_000 ? (window.attribution_started_at as string) : null
}

/**
 * 峰值占带宽上限：云服务器通常按方向分别限速，取入站、出站峰值中较高的一个对比。
 * 任一方向未知或未设置上限时返回 null。
 */
export function peakUsage(
  peaks: Partial<Record<'in' | 'out' | 'total', TrafficPeak>> | null | undefined,
  limitMbps: number | null | undefined
): number | null {
  if (!limitMbps || !peaks?.in || !peaks?.out) return null
  return toMbps(Math.max(peaks.in.bytes_per_second, peaks.out.bytes_per_second)) / limitMbps
}

export interface GrowthView {
  direction: 'up' | 'down' | 'flat' | 'new' | 'unknown'
  bytesText: string
  percentText: string
}

/** 环比（用户、Key 与出口通用）：上一时段为 0 而本期有流量时标为新增，不计算无意义的百分比。 */
export function growthView(stats: Pick<TrafficSubjectStats, 'growth_bytes'> & { previous: { total_bytes: string } | null }): GrowthView {
  const growth = parseSignedBytes(stats.growth_bytes)
  const previous = parseSignedBytes(stats.previous?.total_bytes)
  if (growth === null || previous === null) return { direction: 'unknown', bytesText: '—', percentText: '' }
  if (growth === 0n) return { direction: 'flat', bytesText: formatByteCount(0), percentText: '0%' }
  const magnitude = formatByteCount(growth < 0n ? -growth : growth)
  if (previous === 0n) return { direction: 'new', bytesText: `+${magnitude}`, percentText: '' }
  const percent = (Number(growth) / Number(previous)) * 100
  const absolute = Math.abs(percent)
  const rounded = absolute < 10 ? parseFloat(absolute.toFixed(1)) : Math.round(absolute)
  const sign = growth > 0n ? '+' : '−'
  return {
    direction: growth > 0n ? 'up' : 'down',
    bytesText: `${sign}${magnitude}`,
    percentText: `${sign}${rounded}%`
  }
}
