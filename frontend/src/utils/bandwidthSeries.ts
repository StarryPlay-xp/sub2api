// 带宽监控的数据整理：把分钟/小时桶里的 1 秒峰值合并成图表柱，挑出单方向最忙的时段。
// 峰值只能取最大值合并，不能相加；没有记录的桶不补 0。
import type { ServiceTrafficBucket } from '@/api/admin/ops'
import type { TrafficBucket, TrafficPeak } from '@/api/admin/wireguard'
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
