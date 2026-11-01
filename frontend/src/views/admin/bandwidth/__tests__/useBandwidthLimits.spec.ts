import { beforeEach, describe, expect, it } from 'vitest'
import { nextTick } from 'vue'
import { BANDWIDTH_LIMITS_KEY, normalizeLimit, useBandwidthLimits } from '../useBandwidthLimits'

beforeEach(() => {
  localStorage.clear()
})

describe('useBandwidthLimits', () => {
  it('accepts only positive finite limits', () => {
    expect(normalizeLimit('100')).toBe(100)
    expect(normalizeLimit(0.5)).toBe(0.5)
    expect(normalizeLimit(0)).toBeNull()
    expect(normalizeLimit(-1)).toBeNull()
    expect(normalizeLimit('abc')).toBeNull()
    expect(normalizeLimit(Number.POSITIVE_INFINITY)).toBeNull()
  })

  it('stores host and per-node limits in this browser and clears them', async () => {
    const limits = useBandwidthLimits()
    limits.setHostLimit(200)
    limits.setNodeLimit(11, 50)
    expect(limits.hostLimit.value).toBe(200)
    expect(limits.nodeLimit(11)).toBe(50)
    expect(limits.nodeLimit(12)).toBeNull()
    await nextTick()
    expect(JSON.parse(localStorage.getItem(BANDWIDTH_LIMITS_KEY) || '{}')).toEqual({ host: 200, nodes: { '11': 50 } })

    limits.setNodeLimit(11, null)
    limits.setHostLimit(null)
    expect(limits.nodeLimit(11)).toBeNull()
    expect(limits.hostLimit.value).toBeNull()
  })

  it('ignores corrupted stored values', () => {
    localStorage.setItem(BANDWIDTH_LIMITS_KEY, JSON.stringify({ host: 'oops', nodes: { '11': -5 } }))
    const limits = useBandwidthLimits()
    expect(limits.hostLimit.value).toBeNull()
    expect(limits.nodeLimit(11)).toBeNull()
  })
})
