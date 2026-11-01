import { describe, expect, it } from 'vitest'
import {
  formatByteCount,
  formatByteRate,
  formatMbps,
  formatSampleTime,
  isMissingTime
} from '../trafficFormat'

describe('formatByteCount', () => {
  it('formats decimal strings with binary units', () => {
    expect(formatByteCount('0')).toBe('0 B')
    expect(formatByteCount('1023')).toBe('1023 B')
    expect(formatByteCount('1024')).toBe('1 KiB')
    expect(formatByteCount('536870912')).toBe('512 MiB')
    expect(formatByteCount('1610612736')).toBe('1.5 GiB')
  })

  it('parses decimal strings beyond Number.MAX_SAFE_INTEGER with BigInt', () => {
    // 2^53 + 1 与 2^60 + 1 无法用 Number 精确表示，按字符串换算单位。
    expect(formatByteCount('9007199254740993')).toBe('8 PiB')
    expect(formatByteCount('1152921504606846977')).toBe('1 EiB')
    expect(formatByteCount(String(3n * 2n ** 60n / 2n))).toBe('1.5 EiB')
  })

  it('carries into the next unit when rounding reaches 1024', () => {
    expect(formatByteCount(String(1024 * 1024 - 1))).toBe('1 MiB')
  })

  it('returns a dash for unknown values instead of zero', () => {
    expect(formatByteCount('')).toBe('—')
    expect(formatByteCount(undefined)).toBe('—')
    expect(formatByteCount(null)).toBe('—')
    expect(formatByteCount('-1')).toBe('—')
    expect(formatByteCount('12abc')).toBe('—')
  })
})

describe('formatByteRate', () => {
  it('formats B/s, KiB/s and MiB/s', () => {
    expect(formatByteRate(0)).toBe('0 B/s')
    expect(formatByteRate(512)).toBe('512 B/s')
    expect(formatByteRate(2048)).toBe('2 KiB/s')
    expect(formatByteRate(2658525)).toBe('2.54 MiB/s')
  })

  it('keeps small non-zero rates distinguishable from zero', () => {
    expect(formatByteRate(0.4)).toBe('0.4 B/s')
  })

  it('returns a dash for invalid rates', () => {
    expect(formatByteRate(undefined)).toBe('—')
    expect(formatByteRate(Number.NaN)).toBe('—')
    expect(formatByteRate(-1)).toBe('—')
  })
})

describe('formatMbps', () => {
  it('converts bytes per second with bytes * 8 / 1_000_000', () => {
    expect(formatMbps(0)).toBe('0 Mbps')
    expect(formatMbps(125_000)).toBe('1 Mbps')
    expect(formatMbps(2658525)).toBe('21.27 Mbps')
    expect(formatMbps(2048)).toBe('0.016 Mbps')
    expect(formatMbps(125_000_000)).toBe('1000 Mbps')
  })
})

describe('sample times', () => {
  it('treats the Go zero time as missing', () => {
    expect(isMissingTime('0001-01-01T00:00:00Z')).toBe(true)
    expect(isMissingTime(null)).toBe(true)
    expect(isMissingTime('not a date')).toBe(true)
    expect(isMissingTime('2026-11-01T03:00:00Z')).toBe(false)
    expect(formatSampleTime('0001-01-01T00:00:00Z')).toBe('—')
  })

  it('includes seconds for peak times', () => {
    const text = formatSampleTime('2026-11-01T03:04:05Z')
    expect(text).toMatch(/\d{2}:\d{2}:\d{2}/)
  })
})
