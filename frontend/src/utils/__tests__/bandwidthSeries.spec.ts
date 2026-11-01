import { describe, expect, it } from 'vitest'
import type { ServiceTrafficBucket } from '@/api/admin/ops'
import type { TrafficBucket } from '@/api/admin/wireguard'
import {
  buildPeakBins,
  busierPeak,
  headroomLevel,
  hostSamples,
  maxPeak,
  nodeSamples,
  toMbps,
  topPeakWindows
} from '../bandwidthSeries'

const MINUTE = 60_000
const T0 = Date.parse('2026-11-01T00:00:00Z')
const peak = (bytesPerSecond: number, at: number) => ({
  bytes_per_second: bytesPerSecond,
  at: new Date(at).toISOString(),
  window_seconds: 1
})

describe('toMbps', () => {
  it('uses decimal megabits', () => {
    expect(toMbps(125_000)).toBe(1)
    expect(toMbps(0)).toBe(0)
  })
})

describe('maxPeak', () => {
  it('keeps the higher rate and the earlier sample on ties', () => {
    const early = peak(100, T0)
    const late = peak(100, T0 + 1000)
    expect(maxPeak(late, early)).toBe(early)
    expect(maxPeak(early, peak(200, T0 + 5000))?.bytes_per_second).toBe(200)
    expect(maxPeak(undefined, early)).toBe(early)
  })
})

describe('buildPeakBins', () => {
  it('merges peaks by maximum and bytes by sum without filling gaps with zero', () => {
    const samples = [
      { start: T0, peaks: { in: peak(100, T0 + 1000) }, bytes: { in: 10 } },
      { start: T0 + MINUTE, peaks: { in: peak(300, T0 + MINUTE + 2000) }, bytes: { in: 20 } },
      { start: T0 + 5 * MINUTE, peaks: null, bytes: { in: 5 } }
    ]
    const { size, bins } = buildPeakBins(samples, ['in'] as const, {
      grain: MINUTE,
      range: { start: T0, end: T0 + 6 * MINUTE },
      maxBins: 3
    })
    expect(size).toBe(2 * MINUTE)
    expect(bins).toHaveLength(3)
    expect(bins[0].peaks.in?.bytes_per_second).toBe(300)
    expect(bins[0].bytes.in).toBe(30)
    expect(bins[0].recorded).toBe(2)
    // 中间一柱没有任何记录：recorded 为 0，不能当作零速率
    expect(bins[1].recorded).toBe(0)
    expect(bins[1].peaks.in).toBeUndefined()
    // 有字节但没有峰值（如多实例重叠）：峰值保持未知
    expect(bins[2].recorded).toBe(1)
    expect(bins[2].bytes.in).toBe(5)
    expect(bins[2].peaks.in).toBeUndefined()
  })
})

describe('busierPeak', () => {
  it('picks the higher direction and keeps the other for context', () => {
    const result = busierPeak(peak(300, T0), peak(200, T0 + 1000))
    expect(result?.direction).toBe('in')
    expect(result?.other.bytes_per_second).toBe(200)
    expect(result?.otherDirection).toBe('out')
  })

  it('prefers outbound on ties and stays unknown when a direction is missing', () => {
    expect(busierPeak(peak(200, T0), peak(200, T0))?.direction).toBe('out')
    expect(busierPeak(peak(200, T0), undefined)).toBeNull()
    expect(busierPeak(null, peak(200, T0))).toBeNull()
  })
})

describe('topPeakWindows', () => {
  it('ranks buckets by their busier direction', () => {
    const windows = topPeakWindows(
      [
        { start: T0, peaks: { in: peak(100, T0), out: peak(400, T0) }, bytes: {} },
        { start: T0 + MINUTE, peaks: { in: peak(500, T0 + MINUTE), out: peak(50, T0 + MINUTE) }, bytes: {} },
        { start: T0 + 2 * MINUTE, peaks: null, bytes: {} },
        { start: T0 + 3 * MINUTE, peaks: { in: peak(10, T0), out: peak(20, T0) }, bytes: {} }
      ],
      MINUTE,
      2
    )
    expect(windows.map((item) => [item.start, item.direction, item.peak.bytes_per_second])).toEqual([
      [T0 + MINUTE, 'in', 500],
      [T0, 'out', 400]
    ])
    expect(windows[0].end).toBe(T0 + 2 * MINUTE)
  })
})

describe('sample mapping', () => {
  it('maps host buckets to inbound and outbound sums', () => {
    const bucket: ServiceTrafficBucket = {
      start: '2026-11-01T00:00:00Z',
      interval: 'minute',
      bytes: { client_in: '10', client_out: '200', upstream_out: '11', upstream_in: '190' },
      peaks: { in: peak(5, T0), out: peak(6, T0) },
      quality: { first_sample_at: null, last_sample_at: null, valid_samples: 1, invalid_samples: 0 }
    }
    const [sample] = hostSamples([bucket])
    expect(sample.start).toBe(T0)
    expect(sample.bytes).toEqual({ in: 200, out: 211 })
    expect(sample.peaks?.out?.bytes_per_second).toBe(6)
    expect(hostSamples([{ ...bucket, peaks: null }])[0].peaks).toBeNull()
  })

  it('keeps legacy WireGuard buckets without peaks as unknown', () => {
    const buckets: TrafficBucket[] = [
      { start: '2026-11-01T00:00:00Z', tx_bytes: '3', rx_bytes: '4', peak_history: null },
      {
        start: '2026-11-01T00:01:00Z',
        tx_bytes: '1',
        rx_bytes: '2',
        peak_history: {
          peaks: { total: peak(9, T0 + MINUTE) },
          quality: { first_sample_at: null, last_sample_at: null, valid_samples: 1, invalid_samples: 0 }
        }
      }
    ]
    const samples = nodeSamples(buckets)
    expect(samples[0].peaks).toBeNull()
    expect(samples[0].bytes).toEqual({ tx: 3, rx: 4, total: 7 })
    expect(samples[1].peaks?.total?.bytes_per_second).toBe(9)
  })
})

describe('headroomLevel', () => {
  it('warns from 70% and becomes critical from 90%', () => {
    expect(headroomLevel(0.69)).toBe('normal')
    expect(headroomLevel(0.7)).toBe('warning')
    expect(headroomLevel(0.9)).toBe('critical')
    expect(headroomLevel(1.4)).toBe('critical')
  })
})
