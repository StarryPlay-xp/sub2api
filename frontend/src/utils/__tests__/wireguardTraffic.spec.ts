import { describe, expect, it } from 'vitest'
import {
  firstSelectableDate,
  formatDateInput,
  formatDateTimeLabel,
  formatRFC3339,
  isUtcBucketAligned,
  nextCalendarDay,
  parseDateInput,
  utcOffsetLabel
} from '../wireguardTraffic'

describe('wireguardTraffic', () => {
  it('parses date inputs in local or UTC calendar terms and rejects invalid dates', () => {
    expect(parseDateInput('2026-10-01', 'utc')?.getTime()).toBe(Date.UTC(2026, 9, 1))
    const local = parseDateInput('2026-10-01', 'local')
    expect(local?.getFullYear()).toBe(2026)
    expect(local?.getMonth()).toBe(9)
    expect(local?.getDate()).toBe(1)
    expect(local?.getHours()).toBe(0)
    expect(parseDateInput('2026-02-31', 'utc')).toBeNull()
    expect(parseDateInput('2026-13-01', 'local')).toBeNull()
    expect(parseDateInput('', 'local')).toBeNull()
  })

  it('adds one calendar day across month and year boundaries', () => {
    const endOfMonth = parseDateInput('2026-09-30', 'utc')!
    expect(formatDateInput(nextCalendarDay(endOfMonth, 'utc'), 'utc')).toBe('2026-10-01')
    const endOfYear = parseDateInput('2026-12-31', 'utc')!
    expect(formatDateInput(nextCalendarDay(endOfYear, 'utc'), 'utc')).toBe('2027-01-01')
    const local = parseDateInput('2026-09-30', 'local')!
    expect(formatDateInput(nextCalendarDay(local, 'local'), 'local')).toBe('2026-10-01')
  })

  it('formats RFC3339 timestamps with a timezone and preserves the instant', () => {
    const utc = parseDateInput('2026-10-01', 'utc')!
    expect(formatRFC3339(utc, 'utc')).toBe('2026-10-01T00:00:00.000Z')

    const local = parseDateInput('2026-10-01', 'local')!
    const encoded = formatRFC3339(local, 'local')
    expect(encoded).toMatch(/[+-]\d{2}:\d{2}$/)
    expect(new Date(encoded).getTime()).toBe(local.getTime())
  })

  it('checks UTC bucket alignment for hour and minute grains', () => {
    expect(isUtcBucketAligned(new Date('2026-10-01T00:00:00Z'))).toBe(true)
    expect(isUtcBucketAligned(new Date('2026-10-01T00:30:00Z'))).toBe(false)
    expect(isUtcBucketAligned(new Date('2026-10-01T00:30:00Z'), 'minute')).toBe(true)
    expect(isUtcBucketAligned(new Date('2026-10-01T00:00:30Z'), 'minute')).toBe(false)
  })

  it('advances to the first selectable midnight when retained_from is inside a day', () => {
    const retained = new Date('2026-08-02T06:30:00Z')
    const utc = firstSelectableDate(retained, 'utc')
    expect(formatDateInput(utc, 'utc')).toBe('2026-08-03')

    const local = firstSelectableDate(retained, 'local')
    const localMidnight = new Date(local.getFullYear(), local.getMonth(), local.getDate())
    expect(localMidnight.getTime()).toBe(local.getTime())
    expect(local.getTime()).toBeGreaterThanOrEqual(retained.getTime())
    const previous = new Date(local.getFullYear(), local.getMonth(), local.getDate() - 1)
    expect(previous.getTime()).toBeLessThan(retained.getTime())
  })

  it('labels offsets and handles unparseable dates', () => {
    const date = new Date('2026-10-01T00:00:00Z')
    const offset = -date.getTimezoneOffset()
    const sign = offset >= 0 ? '+' : '-'
    const absolute = Math.abs(offset)
    expect(utcOffsetLabel(date)).toBe(
      `UTC${sign}${String(Math.floor(absolute / 60)).padStart(2, '0')}:${String(absolute % 60).padStart(2, '0')}`
    )
    expect(formatDateTimeLabel('not-a-date')).toBe('—')
  })
})
