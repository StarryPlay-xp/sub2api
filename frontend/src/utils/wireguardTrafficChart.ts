import type { TrafficBucket, TrafficInterval } from '@/api/admin/wireguard'
import { formatDateLabel, formatDateTimeLabel } from './wireguardTraffic'

export const MINUTE_MS = 60_000
export const HOUR_MS = 3_600_000
export const DAY_MS = 86_400_000

// 柱宽与刻度共用的候选步长，均为存储粒度（分钟/小时）的整数倍。
const STEPS = [
  ...[1, 2, 5, 10, 15, 30].map((minutes) => minutes * MINUTE_MS),
  ...[1, 2, 3, 6, 12].map((hours) => hours * HOUR_MS),
  ...[1, 2, 7, 14].map((days) => days * DAY_MS)
]

export interface TimeSpan {
  start: number
  end: number
}

export interface TrafficBin extends TimeSpan {
  tx: number
  rx: number
  // 柱内实际保存的时间桶数；0 表示整柱没有记录，不等于流量为零。
  recorded: number
  expected: number
}

export interface DurationParts {
  unit: 'minute' | 'hour' | 'day'
  count: number
}

const pad2 = (value: number) => String(value).padStart(2, '0')

export function grainMs(interval: TrafficInterval): number {
  return interval === 'minute' ? MINUTE_MS : HOUR_MS
}

function localOffset(time: number): number {
  return -new Date(time).getTimezoneOffset() * MINUTE_MS
}

// 按本地墙钟对齐，例如 6 小时一柱时从本地 0/6/12/18 点开始。
export function floorLocal(time: number, step: number): number {
  const offset = localOffset(time)
  return Math.floor((time + offset) / step) * step - offset
}

// 本地对齐落不到存储粒度上（如 UTC+05:30 的整小时）时退回 UTC 对齐，避免一个桶被拆到两柱。
export function alignBinStart(time: number, size: number, grain: number): number {
  const local = floorLocal(time, size)
  return local % grain === 0 ? local : Math.floor(time / size) * size
}

export function pickBinSize(span: number, grain: number, maxBins: number): number {
  const candidates = STEPS.filter((step) => step >= grain && step % grain === 0)
  return (
    candidates.find((step) => Math.ceil(span / step) <= maxBins) ??
    candidates[candidates.length - 1]
  )
}

export function recordedSpan(history: TrafficBucket[], grain: number): TimeSpan | null {
  let start = Infinity
  let end = -Infinity
  for (const bucket of history) {
    const time = Date.parse(bucket.start)
    if (Number.isNaN(time)) continue
    start = Math.min(start, time)
    end = Math.max(end, time + grain)
  }
  return Number.isFinite(start) ? { start, end } : null
}

// 把原始时间桶合并成固定宽度的柱，柱数不超过 maxBins（对齐可能多出一柱）。
export function buildTrafficBins(
  history: TrafficBucket[],
  options: { grain: number; view: TimeSpan; range: TimeSpan; maxBins: number }
): { size: number; bins: TrafficBin[] } {
  const { grain, view, range, maxBins } = options
  const size = pickBinSize(Math.max(view.end - view.start, grain), grain, maxBins)
  const anchor = alignBinStart(view.start, size, grain)
  const count = Math.max(1, Math.ceil((view.end - anchor) / size))
  const bins: TrafficBin[] = Array.from({ length: count }, (_, index) => {
    const start = anchor + index * size
    const end = start + size
    const covered = Math.min(end, range.end) - Math.max(start, range.start)
    return { start, end, tx: 0, rx: 0, recorded: 0, expected: Math.max(1, Math.ceil(covered / grain)) }
  })
  for (const bucket of history) {
    const index = Math.floor((Date.parse(bucket.start) - anchor) / size)
    if (!(index >= 0 && index < count)) continue
    const bin = bins[index]
    bin.tx += Number(bucket.tx_bytes) || 0
    bin.rx += Number(bucket.rx_bytes) || 0
    bin.recorded++
  }
  return { size, bins }
}

// 连续的无记录柱合并为一段，供图表绘制阴影。
export function gapSpans(bins: TrafficBin[]): TimeSpan[] {
  const spans: TimeSpan[] = []
  for (const bin of bins) {
    if (bin.recorded > 0) continue
    const last = spans[spans.length - 1]
    if (last && last.end === bin.start) last.end = bin.end
    else spans.push({ start: bin.start, end: bin.end })
  }
  return spans
}

// 刻度落在本地整点/整日上，且不会比柱更密，避免 Chart.js 按刻度间距压窄柱宽。
export function buildTimeTicks(
  min: number,
  max: number,
  maxTicks: number,
  minStep = MINUTE_MS
): { values: number[]; step: number } {
  const span = max - min
  const candidates = STEPS.filter((step) => step >= minStep)
  const step =
    candidates.find((candidate) => span / candidate <= maxTicks) ??
    candidates[candidates.length - 1] ??
    minStep
  const values: number[] = []
  if (step >= DAY_MS) {
    const days = Math.round(step / DAY_MS)
    const cursor = new Date(min)
    cursor.setHours(0, 0, 0, 0)
    if (cursor.getTime() < min) cursor.setDate(cursor.getDate() + 1)
    while (cursor.getTime() <= max) {
      values.push(cursor.getTime())
      cursor.setDate(cursor.getDate() + days)
    }
  } else {
    let cursor = floorLocal(min, step)
    if (cursor < min) cursor += step
    for (; cursor <= max; cursor += step) values.push(cursor)
  }
  return { values, step }
}

function clockLabel(date: Date): string {
  return `${pad2(date.getHours())}:${pad2(date.getMinutes())}`
}

function dayLabel(date: Date): string {
  return `${pad2(date.getMonth() + 1)}-${pad2(date.getDate())}`
}

// 跨天的刻度在本地零点显示日期，其余显示时刻。
export function formatTimeTick(value: number, step: number): string {
  const date = new Date(value)
  if (step >= DAY_MS || (date.getHours() === 0 && date.getMinutes() === 0)) {
    return dayLabel(date)
  }
  return clockLabel(date)
}

export function formatBinRange(start: number, end: number, size: number): string {
  if (size >= DAY_MS) {
    const first = formatDateLabel(new Date(start))
    return size === DAY_MS ? first : `${first} – ${formatDateLabel(new Date(end - 1))}`
  }
  const from = new Date(start)
  const to = new Date(end)
  const nextMidnight = new Date(from)
  nextMidnight.setHours(24, 0, 0, 0)
  const sameDay = end <= nextMidnight.getTime()
  return `${formatDateTimeLabel(from)} – ${sameDay ? clockLabel(to) : formatDateTimeLabel(to)}`
}

export function durationParts(size: number): DurationParts {
  if (size % DAY_MS === 0) return { unit: 'day', count: size / DAY_MS }
  if (size % HOUR_MS === 0) return { unit: 'hour', count: size / HOUR_MS }
  return { unit: 'minute', count: Math.round(size / MINUTE_MS) }
}
