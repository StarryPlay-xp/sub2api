import { describe, expect, it } from 'vitest'
import {
  attributionSeconds,
  averageRate,
  formatShare,
  growthView,
  lateAttributionStart,
  parseSignedBytes,
  peakUsage,
  subjectKey,
  subjectTitle
} from '../subjectFormat'
import { subjectPeak, keyIdentity, subjectStats, subjectWindow, userIdentity } from './fixtures'

describe('subject formatting', () => {
  it('parses signed byte strings without treating garbage as zero', () => {
    expect(parseSignedBytes('-1024')).toBe(-1024n)
    expect(parseSignedBytes('18446744073709551615')).toBe(18446744073709551615n)
    expect(parseSignedBytes('')).toBeNull()
    expect(parseSignedBytes('1.5')).toBeNull()
    expect(parseSignedBytes(null)).toBeNull()
  })

  it('names users by email, then username, and keys by name, then id', () => {
    expect(subjectTitle(userIdentity(7))).toBe('user7@example.com')
    expect(subjectTitle(userIdentity(7, { user_email: '' }))).toBe('user7')
    expect(subjectTitle(keyIdentity(9, 7))).toBe('key-9')
    expect(subjectTitle(keyIdentity(9, 7, { api_key_name: ' ' }))).toBe('#9')
    expect(subjectKey(keyIdentity(9, 7))).toBe('api_key:9')
    expect(subjectKey(userIdentity(7))).toBe('user:7')
  })

  it('formats shares with precision for small values', () => {
    expect(formatShare(0.256)).toBe('26%')
    expect(formatShare(0.0123)).toBe('1.2%')
    expect(formatShare(0.0004)).toBe('<0.1%')
    expect(formatShare(0)).toBe('0%')
    expect(formatShare(null)).toBe('—')
  })

  it('averages only over the time since per-subject counting started', () => {
    const window = subjectWindow({ start: '2026-10-31T03:00:00Z', end: '2026-11-01T03:00:00Z', attribution_started_at: '2026-11-01T01:00:00Z' })
    expect(attributionSeconds(window)).toBe(7200)
    const stats = subjectStats(userIdentity(7), { bytes: { client_in: '7200', client_out: '0', upstream_out: '0', upstream_in: '0' } })
    expect(averageRate(stats, window)).toBe(1)
    expect(lateAttributionStart(window)).toBe('2026-11-01T01:00:00Z')
    expect(lateAttributionStart(subjectWindow())).toBeNull()
    expect(attributionSeconds(subjectWindow({ attribution_started_at: null }))).toBeNull()
  })

  it('compares the busier direction with the bandwidth limit and stays unknown without both peaks', () => {
    // 出站 1,250,000 B/s = 10 Mbps
    const peaks = { in: subjectPeak(625_000), out: subjectPeak(1_250_000), total: subjectPeak(1_875_000) }
    expect(peakUsage(peaks, 20)).toBeCloseTo(0.5)
    expect(peakUsage({ out: subjectPeak(1_250_000) }, 20)).toBeNull()
    expect(peakUsage(peaks, null)).toBeNull()
  })

  it('describes growth, new subjects and unknown comparisons', () => {
    const base = subjectStats(userIdentity(7))
    expect(growthView({ ...base, previous: { total_bytes: '1048576', peaks: null }, growth_bytes: '524288' })).toEqual({
      direction: 'up',
      bytesText: '+512 KiB',
      percentText: '+50%'
    })
    expect(growthView({ ...base, previous: { total_bytes: '1048576', peaks: null }, growth_bytes: '-104858' })).toMatchObject({
      direction: 'down',
      percentText: '−10%'
    })
    expect(growthView({ ...base, previous: { total_bytes: '0', peaks: null }, growth_bytes: '2048' })).toMatchObject({
      direction: 'new',
      bytesText: '+2 KiB'
    })
    expect(growthView(base).direction).toBe('unknown')
  })
})
