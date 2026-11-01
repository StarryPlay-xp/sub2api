// 把不同来源的时间桶换算成趋势图统一的 in / out / total 样本。
import type { EgressHistoryBucket } from '@/api/admin/trafficEgress'
import type { TrafficSubjectBucket } from '@/api/admin/trafficSubjects'
import type { TrafficBucket } from '@/api/admin/wireguard'
import type { PeakSample } from '@/utils/bandwidthSeries'

export type TrendKey = 'in' | 'out' | 'total'

const toNumber = (value: string | undefined) => {
  const number = Number(value)
  return Number.isFinite(number) ? number : 0
}

/** 用户 / Key 桶：入站 = 客户端请求 + 上游响应，出站 = 客户端响应 + 上游请求。 */
export function subjectTrendSamples(history: TrafficSubjectBucket[]): PeakSample<TrendKey>[] {
  return history
    .map((bucket) => {
      const inbound = toNumber(bucket.bytes?.client_in) + toNumber(bucket.bytes?.upstream_in)
      const outbound = toNumber(bucket.bytes?.client_out) + toNumber(bucket.bytes?.upstream_out)
      return {
        start: Date.parse(bucket.start),
        peaks: bucket.peaks,
        bytes: { in: inbound, out: outbound, total: inbound + outbound }
      }
    })
    .filter((sample) => !Number.isNaN(sample.start))
}

/**
 * WG 节点桶：从节点接收（rx）进入本站记为入站，发往节点（tx）记为出站，
 * 与流量路径中“上游响应 / 上游请求”的方向和配色一致。升级前的桶没有峰值，保留为未知。
 */
export function nodeTrendSamples(history: TrafficBucket[]): PeakSample<TrendKey>[] {
  return history
    .map((bucket) => {
      const tx = toNumber(bucket.tx_bytes)
      const rx = toNumber(bucket.rx_bytes)
      const peaks = bucket.peak_history?.peaks
      return {
        start: Date.parse(bucket.start),
        peaks: peaks ? { in: peaks.rx, out: peaks.tx, total: peaks.total } : null,
        bytes: { in: rx, out: tx, total: tx + rx }
      }
    })
    .filter((sample) => !Number.isNaN(sample.start))
}

/** 出口桶：上游响应（上游 → 本站）记为入站，上游请求（本站 → 上游）记为出站。 */
export function egressTrendSamples(history: EgressHistoryBucket[]): PeakSample<TrendKey>[] {
  return history
    .map((bucket) => {
      const inbound = toNumber(bucket.bytes?.upstream_in)
      const outbound = toNumber(bucket.bytes?.upstream_out)
      return {
        start: Date.parse(bucket.start),
        peaks: bucket.peaks,
        bytes: { in: inbound, out: outbound, total: inbound + outbound }
      }
    })
    .filter((sample) => !Number.isNaN(sample.start))
}
