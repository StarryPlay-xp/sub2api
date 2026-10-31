import { describe, expect, it } from 'vitest'
import {
  DAY_MS,
  HOUR_MS,
  MINUTE_MS,
  buildTimeTicks,
  buildTrafficBins,
  durationParts,
  formatBinRange,
  formatTimeTick,
  gapSpans,
  pickBinSize,
  recordedSpan
} from '../wireguardTrafficChart'

// 使用本地时间构造，避免依赖运行测试的时区；日期避开夏令时切换。
const local = (hour: number, minute = 0, day = 14) => new Date(2026, 9, day, hour, minute).getTime()
const bucket = (time: number, tx: number, rx = 0) => ({
  start: new Date(time).toISOString(),
  tx_bytes: String(tx),
  rx_bytes: String(rx)
})

describe('wireguardTrafficChart', () => {
  it('picks the finest bin size that stays within the bar budget', () => {
    expect(pickBinSize(103 * MINUTE_MS, MINUTE_MS, 120)).toBe(MINUTE_MS)
    expect(pickBinSize(DAY_MS, MINUTE_MS, 120)).toBe(15 * MINUTE_MS)
    expect(pickBinSize(7 * DAY_MS, HOUR_MS, 120)).toBe(2 * HOUR_MS)
    expect(pickBinSize(90 * DAY_MS, HOUR_MS, 120)).toBe(DAY_MS)
    // 小时粒度不会被拆成分钟柱
    expect(pickBinSize(3 * HOUR_MS, HOUR_MS, 120)).toBe(HOUR_MS)
  })

  it('finds the recorded span from saved buckets', () => {
    const history = [bucket(local(10, 5), 1), bucket(local(10, 1), 1)]
    expect(recordedSpan(history, MINUTE_MS)).toEqual({ start: local(10, 1), end: local(10, 6) })
    expect(recordedSpan([], MINUTE_MS)).toBeNull()
  })

  it('sums buckets into bins and keeps empty bins distinguishable from zero traffic', () => {
    const history = [
      bucket(local(10, 0), 100, 10),
      bucket(local(10, 1), 200, 20),
      bucket(local(10, 7), 0, 0)
    ]
    const { size, bins } = buildTrafficBins(history, {
      grain: MINUTE_MS,
      view: { start: local(10, 0), end: local(10, 8) },
      range: { start: local(9, 0), end: local(10, 8) },
      maxBins: 4
    })
    expect(size).toBe(2 * MINUTE_MS)
    expect(bins.map((bin) => bin.start)).toEqual([local(10, 0), local(10, 2), local(10, 4), local(10, 6)])
    expect(bins[0]).toMatchObject({ tx: 300, rx: 30, recorded: 2, expected: 2 })
    expect(bins[1].recorded).toBe(0)
    // 有记录但流量为 0 的柱仍算作有记录
    expect(bins[3]).toMatchObject({ tx: 0, rx: 0, recorded: 1, expected: 2 })
    expect(gapSpans(bins)).toEqual([{ start: local(10, 2), end: local(10, 6) }])
  })

  it('counts expected buckets only inside the queried range', () => {
    const { bins } = buildTrafficBins([bucket(local(10, 3), 1)], {
      grain: MINUTE_MS,
      view: { start: local(10, 0), end: local(10, 3) + 30_000 },
      range: { start: local(10, 1), end: local(10, 3) + 30_000 },
      maxBins: 2
    })
    expect(bins).toHaveLength(2)
    expect(bins[0].expected).toBe(1)
    expect(bins[1]).toMatchObject({ recorded: 1, expected: 2 })
  })

  it('places ticks on local clock boundaries no denser than the bars', () => {
    const { values, step } = buildTimeTicks(local(15, 41), local(15, 41, 15), 8, 15 * MINUTE_MS)
    expect(step).toBe(3 * HOUR_MS)
    expect(values).toHaveLength(8)
    expect(new Date(values[0]).getHours()).toBe(18)
    expect(values.every((value) => new Date(value).getMinutes() === 0)).toBe(true)
    expect(formatTimeTick(values[2], step)).toBe('10-15')
    expect(formatTimeTick(values[0], step)).toBe('18:00')

    const coarse = buildTimeTicks(local(10, 0), local(13, 0), 8, HOUR_MS)
    expect(coarse.step).toBe(HOUR_MS)
    expect(coarse.values).toEqual([local(10), local(11), local(12), local(13)])
  })

  it('uses local midnights for multi-day ticks', () => {
    const { values, step } = buildTimeTicks(local(15, 0, 1), local(15, 0, 8), 8, HOUR_MS)
    expect(step).toBe(DAY_MS)
    expect(values).toHaveLength(7)
    expect(values.every((value) => new Date(value).getHours() === 0)).toBe(true)
    expect(formatTimeTick(values[0], step)).toBe('10-02')
  })

  it('formats bin ranges and durations', () => {
    expect(formatBinRange(local(10, 0), local(10, 15), 15 * MINUTE_MS)).toMatch(/– 10:15$/)
    expect(formatBinRange(local(0, 0), local(0, 0, 15), DAY_MS)).not.toContain('–')
    expect(durationParts(15 * MINUTE_MS)).toEqual({ unit: 'minute', count: 15 })
    expect(durationParts(6 * HOUR_MS)).toEqual({ unit: 'hour', count: 6 })
    expect(durationParts(DAY_MS)).toEqual({ unit: 'day', count: 1 })
  })
})
