// 带宽监控的数据整理：把分钟/小时桶里的 1 秒峰值合并成图表柱，挑出单方向最忙的时段。
// 峰值只能取最大值合并，不能相加；没有记录的桶不补 0。
import type {
  ServiceTrafficBucket,
  ServiceTrafficDirection,
  ServiceTrafficReport
} from '@/api/admin/ops'
import type { TrafficBucket, TrafficPeak } from '@/api/admin/wireguard'
import { isMissingTime } from './trafficFormat'
import { alignBinStart, pickBinSize, type TimeSpan } from './wireguardTrafficChart'

export type HostDirection = 'in' | 'out'
export type NodePeakKey = 'tx' | 'rx' | 'total'

/** B/s 换算为十进制 Mbps，带宽套餐按此单位售卖。 */
export function toMbps(bytesPerSecond: number): number {
  return (bytesPerSecond * 8) / 1_000_000
}

export interface PeakSample<K extends string> {
  start: number
  peaks: Partial<Record<K, TrafficPeak>> | null
  bytes: Partial<Record<K, number>>
}

export interface PeakBin<K extends string> extends TimeSpan {
  peaks: Partial<Record<K, TrafficPeak>>
  bytes: Record<K, number>
  // 柱内实际有记录的时间桶数；0 表示整柱没有记录，不等于零速率。
  recorded: number
  expected: number
}

// 同一速率取更早的样本，和后端合并规则一致。
export function maxPeak(a: TrafficPeak | undefined, b: TrafficPeak | undefined): TrafficPeak | undefined {
  if (!a) return b
  if (!b) return a
  if (b.bytes_per_second > a.bytes_per_second) return b
  if (b.bytes_per_second === a.bytes_per_second && Date.parse(b.at) < Date.parse(a.at)) return b
  return a
}

/** 把原始桶合并成固定宽度的柱：峰值取最大，字节相加，柱数不超过 maxBins（对齐可能多一柱）。 */
export function buildPeakBins<K extends string>(
  samples: PeakSample<K>[],
  keys: readonly K[],
  options: { grain: number; range: TimeSpan; maxBins: number }
): { size: number; bins: PeakBin<K>[] } {
  const { grain, range, maxBins } = options
  const size = pickBinSize(Math.max(range.end - range.start, grain), grain, maxBins)
  const anchor = alignBinStart(range.start, size, grain)
  const count = Math.max(1, Math.ceil((range.end - anchor) / size))
  const emptyBytes = () => Object.fromEntries(keys.map((key) => [key, 0])) as Record<K, number>
  const bins: PeakBin<K>[] = Array.from({ length: count }, (_, index) => {
    const start = anchor + index * size
    const end = start + size
    const covered = Math.min(end, range.end) - Math.max(start, range.start)
    return {
      start,
      end,
      peaks: {},
      bytes: emptyBytes(),
      recorded: 0,
      expected: Math.max(1, Math.ceil(covered / grain))
    }
  })
  for (const sample of samples) {
    const index = Math.floor((sample.start - anchor) / size)
    if (!(index >= 0 && index < count)) continue
    const bin = bins[index]
    bin.recorded++
    for (const key of keys) {
      bin.bytes[key] += sample.bytes[key] ?? 0
      const merged = maxPeak(bin.peaks[key], sample.peaks?.[key])
      if (merged) bin.peaks[key] = merged
    }
  }
  return { size, bins }
}

export interface BusierPeak {
  direction: HostDirection
  peak: TrafficPeak
  otherDirection: HostDirection
  other: TrafficPeak
}

// 入站、出站峰值可能发生在不同时刻；取较高的一个即单方向最忙的那 1 秒。
// 相等时按出站，因为云服务器通常限制出站；任一方向未知时整体未知，不做猜测。
export function busierPeak(
  inbound: TrafficPeak | null | undefined,
  outbound: TrafficPeak | null | undefined
): BusierPeak | null {
  if (!inbound || !outbound) return null
  return outbound.bytes_per_second >= inbound.bytes_per_second
    ? { direction: 'out', peak: outbound, otherDirection: 'in', other: inbound }
    : { direction: 'in', peak: inbound, otherDirection: 'out', other: outbound }
}

export interface PeakWindow extends BusierPeak, TimeSpan {}

/** 按单方向峰值从高到低列出最忙的时间桶，用于“峰值排行”。 */
export function topPeakWindows(
  samples: PeakSample<HostDirection>[],
  grain: number,
  limit = 10
): PeakWindow[] {
  const windows: PeakWindow[] = []
  for (const sample of samples) {
    const busier = busierPeak(sample.peaks?.in, sample.peaks?.out)
    if (busier) windows.push({ ...busier, start: sample.start, end: sample.start + grain })
  }
  return windows
    .sort((a, b) => b.peak.bytes_per_second - a.peak.bytes_per_second || a.start - b.start)
    .slice(0, limit)
}

const toNumber = (value: string | undefined) => {
  const number = Number(value)
  return Number.isFinite(number) ? number : 0
}

/** 服务流量桶：入站 = 客户端→本站 + 上游→本站，出站 = 本站→客户端 + 本站→上游。 */
export function hostSamples(history: ServiceTrafficBucket[]): PeakSample<HostDirection>[] {
  return history
    .map((bucket) => ({
      start: Date.parse(bucket.start),
      peaks: bucket.peaks ? { in: bucket.peaks.in, out: bucket.peaks.out } : null,
      bytes: {
        in: toNumber(bucket.bytes?.client_in) + toNumber(bucket.bytes?.upstream_in),
        out: toNumber(bucket.bytes?.client_out) + toNumber(bucket.bytes?.upstream_out)
      }
    }))
    .filter((sample) => !Number.isNaN(sample.start))
}

function parseBytes(value: string | null | undefined): bigint | null {
  if (typeof value !== 'string' || !/^\d+$/.test(value)) return null
  return BigInt(value)
}

/** 实际观察时长：取首末样本与所选范围的交集；无法确认时返回 null，不拿范围时长冒充。 */
export function observedTrafficSeconds(report: ServiceTrafficReport): number | null {
  const quality = report.quality
  if (!quality || isMissingTime(quality.first_sample_at) || isMissingTime(quality.last_sample_at)) {
    return null
  }
  const start = Date.parse(report.start)
  const end = Date.parse(report.end)
  const first = Date.parse(quality.first_sample_at ?? '')
  const last = Date.parse(quality.last_sample_at ?? '')
  if (![start, end, first, last].every(Number.isFinite)) return null
  const seconds = (Math.min(last, end) - Math.max(first, start)) / 1000
  return seconds > 0 ? seconds : null
}

export interface TrafficSummary {
  total: bigint | null
  inbound: bigint | null
  outbound: bigint | null
  observedSeconds: number | null
  dailyBytes: bigint | null
  averageBytesPerSecond: number | null
}

/** 所选期间的累计流量、日均与平均带宽；日均和均速只按实际观察时长折算。 */
export function trafficSummary(report: ServiceTrafficReport | null): TrafficSummary {
  if (!report?.has_records) {
    return {
      total: null,
      inbound: null,
      outbound: null,
      observedSeconds: null,
      dailyBytes: null,
      averageBytesPerSecond: null
    }
  }
  const total = parseBytes(report.total_bytes)
  const inbound = parseBytes(report.in_bytes)
  const outbound = parseBytes(report.out_bytes)
  const observedSeconds = observedTrafficSeconds(report)
  const averageBytesPerSecond =
    total !== null && observedSeconds !== null ? Number(total) / observedSeconds : null
  const dailyBytes =
    total !== null && observedSeconds !== null
      ? (total * 86_400n) / BigInt(Math.max(1, Math.round(observedSeconds)))
      : null
  return { total, inbound, outbound, observedSeconds, dailyBytes, averageBytesPerSecond }
}

export interface TrafficDirectionShare {
  key: ServiceTrafficDirection
  bytes: bigint | null
  percent: number | null
}

const DIRECTION_KEYS = ['client_in', 'client_out', 'upstream_out', 'upstream_in'] as const

/** 四方向流量及占双向总量的百分比，按 0.1% 向下取整，四段相加不会超过 100%。 */
export function trafficDirectionShares(report: ServiceTrafficReport | null): TrafficDirectionShare[] {
  const total = report?.has_records ? parseBytes(report.total_bytes) : null
  return DIRECTION_KEYS.map((key) => {
    const bytes = report?.has_records ? parseBytes(report.bytes?.[key]) : null
    const percent =
      bytes !== null && total !== null && total > 0n
        ? Number((bytes * 1000n) / total) / 10
        : null
    return { key, bytes, percent }
  })
}

/** WG 桶：升级前的历史 peak_history 为 null，保留为未知。 */
export function nodeSamples(history: TrafficBucket[]): PeakSample<NodePeakKey>[] {
  return history
    .map((bucket) => ({
      start: Date.parse(bucket.start),
      peaks: bucket.peak_history?.peaks ?? null,
      bytes: { tx: toNumber(bucket.tx_bytes), rx: toNumber(bucket.rx_bytes), total: toNumber(bucket.tx_bytes) + toNumber(bucket.rx_bytes) }
    }))
    .filter((sample) => !Number.isNaN(sample.start))
}

/** 峰值占上限的比例及状态：70% 起接近上限，90% 起严重。 */
export type HeadroomLevel = 'normal' | 'warning' | 'critical'

export function headroomLevel(ratio: number): HeadroomLevel {
  if (ratio >= 0.9) return 'critical'
  if (ratio >= 0.7) return 'warning'
  return 'normal'
}
