import { describe, expect, it } from 'vitest'
import {
  amountToBytes,
  formatDeviation,
  formatPeriodRange,
  formatQuotaBytes,
  formatRatioPercent,
  formatSignedQuotaBytes,
  formatZonedDateTime,
  hostCardMode,
  isAmountText,
  periodElapsed,
  quotaMeter,
  quotaTone,
  resetDayFromDate
} from '../hostFormat'
import { HOST_SCENARIOS, hostPeriod, hostPlan, hostScenario } from './hostFixtures'

describe('formatQuotaBytes', () => {
  it('uses the plan unit base and its own unit names', () => {
    expect(formatQuotaBytes('102920000000', 1000)).toBe('102.92 GB')
    expect(formatQuotaBytes('1500000000000', 1000)).toBe('1.5 TB')
    expect(formatQuotaBytes(String(1024 ** 4), 1024)).toBe('1 TiB')
    expect(formatQuotaBytes('999', 1000)).toBe('999 B')
  })

  it('rounds up to the next unit instead of showing 1000 GB', () => {
    expect(formatQuotaBytes('999999000000', 1000)).toBe('1 TB')
  })

  it('never turns unknown values into zero', () => {
    expect(formatQuotaBytes(null, 1000)).toBe('—')
    expect(formatQuotaBytes('', 1000)).toBe('—')
    expect(formatQuotaBytes('-5', 1000)).toBe('—')
  })

  it('formats signed adjustments', () => {
    expect(formatSignedQuotaBytes('95100000000', 1000)).toBe('+95.1 GB')
    expect(formatSignedQuotaBytes('-1200000000', 1000)).toBe('−1.2 GB')
    expect(formatSignedQuotaBytes('0', 1000)).toBe('0 B')
  })
})

describe('amount input', () => {
  it('accepts positive decimals with up to four places', () => {
    expect(isAmountText('1.5')).toBe(true)
    expect(isAmountText('102.92')).toBe(true)
    expect(isAmountText('0')).toBe(false)
    expect(isAmountText('0', true)).toBe(true)
    expect(isAmountText('1.23456')).toBe(false)
    expect(isAmountText('1e3')).toBe(false)
    expect(isAmountText('-1')).toBe(false)
  })

  it('converts to bytes with the chosen base', () => {
    expect(amountToBytes('1.5', 'TB', 1000)).toBe(1_500_000_000_000n)
    expect(amountToBytes('102.92', 'GB', 1000)).toBe(102_920_000_000n)
    expect(amountToBytes('1', 'GB', 1024)).toBe(1_073_741_824n)
    expect(amountToBytes('abc', 'GB', 1000)).toBeNull()
  })
})

describe('reset day', () => {
  it('derives the day of month from the next reset date', () => {
    expect(resetDayFromDate('2026-11-24')).toEqual({ day: 24, maybeClamped: false })
    expect(resetDayFromDate('2026-12-31')).toEqual({ day: 31, maybeClamped: false })
  })

  it('flags month-end dates that may come from a later reset day in a short month', () => {
    expect(resetDayFromDate('2027-02-28')).toEqual({ day: 28, maybeClamped: true })
    expect(resetDayFromDate('2026-11-30')).toEqual({ day: 30, maybeClamped: true })
  })

  it('rejects invalid dates', () => {
    expect(resetDayFromDate('2026-02-30')).toBeNull()
    expect(resetDayFromDate('')).toBeNull()
  })
})

describe('period text', () => {
  it('shows times in the plan time zone and drops midnight', () => {
    expect(formatZonedDateTime('2026-11-24T00:00:00Z', 'UTC')).toBe('11-24')
    expect(formatZonedDateTime('2026-11-24T00:00:00Z', 'Asia/Shanghai')).toBe('11-24 08:00')
    expect(formatPeriodRange({ start: '2026-10-24T00:00:00Z', end: '2026-11-24T00:00:00Z' }, 'UTC')).toBe('10-24 → 11-24')
    expect(formatPeriodRange({ start: '2026-12-24T00:00:00Z', end: '2027-01-24T00:00:00Z' }, 'UTC')).toBe('2026-12-24 → 2027-01-24')
  })

  it('formats ratios and deviations', () => {
    expect(formatRatioPercent(0.0686)).toBe('6.86%')
    expect(formatRatioPercent(0.8403)).toBe('84%')
    expect(formatRatioPercent(null)).toBe('—')
    expect(formatDeviation(-0.0688)).toBe('−6.9%')
    expect(formatDeviation(0.0112)).toBe('+1.1%')
  })
})

describe('quota meter and tone', () => {
  const now = Date.parse('2026-11-02T10:00:00Z')

  it('splits billed usage by direction for both-direction plans', () => {
    const meter = quotaMeter(hostPeriod(), hostPlan(), now)
    expect(meter.segments.map((segment) => segment.key)).toEqual(['rx', 'tx'])
    expect(meter.segments[0].width).toBeCloseTo(2.747, 2)
    expect(meter.segments[1].width).toBeCloseTo(4.115, 2)
    expect(meter.thresholds).toEqual([
      { percent: 80, at: 80 },
      { percent: 95, at: 95 }
    ])
    expect(meter.quotaAt).toBeNull()
    // 10-24 到 11-24 共 31 天，11-02 10:00 已过 9 天 10 小时，约 30.4%。
    expect(meter.pace?.elapsed).toBeCloseTo(0.3038, 3)
    expect(meter.pace?.at).toBeCloseTo(30.38, 1)
  })

  it('shows only the billed direction and adds provider fill-in as its own segment', () => {
    expect(quotaMeter(hostPeriod(), hostPlan({ billing_mode: 'outbound' }), now).segments.map((s) => s.key)).toEqual(['tx'])
    expect(quotaMeter(hostPeriod(), hostPlan({ billing_mode: 'max' }), now).segments.map((s) => s.key)).toEqual(['tx'])
    const filled = quotaMeter(hostScenario('filled').period!, hostPlan(), now)
    expect(filled.segments.map((segment) => segment.key)).toEqual(['rx', 'tx', 'adjustment'])
    expect(filled.segments[2].width).toBeCloseTo(6.34, 2)
  })

  it('scales to billed usage and marks the quota when over', () => {
    const meter = quotaMeter(hostScenario('over').period!, hostPlan(), now)
    const total = meter.segments.reduce((sum, segment) => sum + segment.width, 0)
    expect(total).toBeCloseTo(100, 1)
    expect(meter.quotaAt).toBeCloseTo(94.94, 1)
    expect(meter.thresholds[0].at).toBeCloseTo(75.95, 1)
  })

  it('shrinks measured segments when the provider value is lower', () => {
    const period = hostPeriod({ adjustment_bytes: '-2920000000', adjustment_kind: 'override', billed_bytes: '100000000000' })
    const meter = quotaMeter(period, hostPlan(), now)
    const total = meter.segments.reduce((sum, segment) => sum + segment.width, 0)
    expect(meter.segments.map((segment) => segment.key)).toEqual(['rx', 'tx'])
    expect(total).toBeCloseTo(6.667, 2)
  })

  it('measures elapsed time within the period', () => {
    expect(periodElapsed({ start: '2026-10-24T00:00:00Z', end: '2026-11-24T00:00:00Z' }, Date.parse('2026-10-20T00:00:00Z'))).toBe(0)
    expect(periodElapsed({ start: '2026-10-24T00:00:00Z', end: '2026-11-24T00:00:00Z' }, Date.parse('2026-12-01T00:00:00Z'))).toBe(1)
  })

  it('uses the plan thresholds', () => {
    const plan = hostPlan()
    expect(quotaTone(hostPeriod(), plan)).toBe('normal')
    expect(quotaTone(hostScenario('warning').period!, plan)).toBe('warning')
    expect(quotaTone(hostPeriod({ used_ratio: 0.96 }), plan)).toBe('critical')
    expect(quotaTone(hostScenario('over').period!, plan)).toBe('over')
  })
})

describe('card mode', () => {
  it('maps every scenario to a card state', () => {
    const modes = Object.fromEntries(HOST_SCENARIOS.map((name) => [name, hostCardMode(hostScenario(name))]))
    expect(modes).toEqual({
      complete: 'quota',
      filled: 'quota',
      partial: 'quota',
      warning: 'quota',
      over: 'quota',
      unitHint: 'quota',
      stale: 'quota',
      noPlan: 'no_plan',
      starting: 'starting',
      ambiguous: 'no_plan',
      needsMount: 'needs_host_mount',
      unsupported: 'unsupported'
    })
    expect(hostCardMode(null)).toBe('unavailable')
  })
})
