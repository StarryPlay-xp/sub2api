import type {
  HostBillingMode,
  HostBound,
  HostTrafficPeriod,
  HostTrafficPlan,
  HostTrafficStatus,
  HostUnitBase
} from '@/api/admin/hostTraffic'

type Translate = (key: string, params?: Record<string, unknown>) => string

// 套餐额度按服务商的进制显示：1000 进制用 GB/TB，1024 进制用 GiB/TiB，与页面其他字节量（固定 1024 进制）分开处理。
const DECIMAL_UNITS = ['B', 'KB', 'MB', 'GB', 'TB', 'PB', 'EB']
const BINARY_UNITS = ['B', 'KiB', 'MiB', 'GiB', 'TiB', 'PiB', 'EiB']

export const UNIT_POWER = { MB: 2, GB: 3, TB: 4 } as const

export function parseByteString(value: string | null | undefined): bigint | null {
  return typeof value === 'string' && /^\d+$/.test(value) ? BigInt(value) : null
}

export function parseSignedByteString(value: string | null | undefined): bigint | null {
  return typeof value === 'string' && /^-?\d+$/.test(value) ? BigInt(value) : null
}

/** 按套餐进制格式化字节数；无法解析时返回 “—”。 */
export function formatQuotaBytes(value: string | bigint | null | undefined, base: HostUnitBase, decimals = 2): string {
  const bytes = typeof value === 'bigint' ? value : parseByteString(value)
  if (bytes === null || bytes < 0n) return '—'
  const units = base === 1024 ? BINARY_UNITS : DECIMAL_UNITS
  const step = BigInt(base)
  let unit = 0
  let divisor = 1n
  while (unit < units.length - 1 && bytes >= divisor * step) {
    divisor *= step
    unit++
  }
  if (unit === 0) return `${bytes} B`
  const scale = 10n ** BigInt(decimals)
  let scaled = (bytes * scale + divisor / 2n) / divisor
  if (scaled >= step * scale && unit < units.length - 1) {
    unit++
    divisor *= step
    scaled = (bytes * scale + divisor / 2n) / divisor
  }
  const integer = scaled / scale
  const fraction = (scaled % scale).toString().padStart(decimals, '0').replace(/0+$/, '')
  return `${integer}${fraction ? `.${fraction}` : ''} ${units[unit]}`
}

/** 带正负号的字节数，用于服务商校准量。 */
export function formatSignedQuotaBytes(value: string | null | undefined, base: HostUnitBase): string {
  const bytes = parseSignedByteString(value)
  if (bytes === null) return '—'
  if (bytes === 0n) return formatQuotaBytes(0n, base)
  return `${bytes < 0n ? '−' : '+'}${formatQuotaBytes(bytes < 0n ? -bytes : bytes, base)}`
}

/** 校验用户输入的数量：非负十进制，最多 4 位小数。 */
export function isAmountText(text: string, allowZero = false): boolean {
  const value = text.trim()
  if (!/^\d{1,9}(\.\d{1,4})?$/.test(value)) return false
  return allowZero || Number(value) > 0
}

/** 把 “1.5” + TB 换算为字节，用于表单预览；结果与后端一致取整到字节。 */
export function amountToBytes(text: string, unit: keyof typeof UNIT_POWER, base: HostUnitBase): bigint | null {
  const value = text.trim()
  const match = /^(\d{1,9})(?:\.(\d{1,4}))?$/.exec(value)
  if (!match) return null
  const fraction = (match[2] ?? '').padEnd(4, '0')
  const scaled = BigInt(match[1]) * 10000n + BigInt(fraction)
  return (scaled * BigInt(base) ** BigInt(UNIT_POWER[unit])) / 10000n
}

/** 百分比：小于 10% 保留两位小数，否则一位。 */
export function formatRatioPercent(ratio: number | null | undefined): string {
  if (typeof ratio !== 'number' || !Number.isFinite(ratio) || ratio < 0) return '—'
  const percent = ratio * 100
  const digits = percent < 10 ? 2 : 1
  return `${parseFloat(percent.toFixed(digits))}%`
}

export function formatDeviation(ratio: number | null | undefined): string {
  if (typeof ratio !== 'number' || !Number.isFinite(ratio)) return '—'
  const percent = parseFloat((ratio * 100).toFixed(1))
  if (percent === 0) return '0%'
  return `${percent > 0 ? '+' : '−'}${Math.abs(percent)}%`
}

function zonedParts(iso: string, timeZone: string) {
  const parts = new Intl.DateTimeFormat('en-CA', {
    timeZone,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false
  }).formatToParts(new Date(iso))
  const get = (type: string) => parts.find((part) => part.type === type)?.value ?? ''
  return { year: get('year'), month: get('month'), day: get('day'), hour: get('hour') === '24' ? '00' : get('hour'), minute: get('minute') }
}

export function isValidTimeZone(timeZone: string): boolean {
  try {
    new Intl.DateTimeFormat('en-US', { timeZone })
    return true
  } catch {
    return false
  }
}

/** 浏览器本地时间 “MM-DD HH:mm”，用于统计起点、最近读数等与套餐时区无关的时刻。 */
export function formatLocalShort(iso: string | null | undefined): string {
  if (!iso || Number.isNaN(Date.parse(iso))) return '—'
  const date = new Date(iso)
  const pad = (value: number) => String(value).padStart(2, '0')
  return `${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`
}

/** 套餐时区下的 “MM-DD HH:mm”；跨年时带年份。 */
export function formatZonedDateTime(iso: string | null | undefined, timeZone: string, withYear = false): string {
  if (!iso || Number.isNaN(Date.parse(iso)) || !isValidTimeZone(timeZone)) return '—'
  const p = zonedParts(iso, timeZone)
  const date = withYear ? `${p.year}-${p.month}-${p.day}` : `${p.month}-${p.day}`
  return p.hour === '00' && p.minute === '00' ? date : `${date} ${p.hour}:${p.minute}`
}

/** 套餐时区下的日期 “YYYY-MM-DD”。 */
export function zonedDate(iso: string, timeZone: string): string {
  const p = zonedParts(iso, timeZone)
  return `${p.year}-${p.month}-${p.day}`
}

/** 账期 [start, end) 的显示文本；end 是下次重置时刻。 */
export function formatPeriodRange(period: Pick<HostTrafficPeriod, 'start' | 'end'>, timeZone: string): string {
  const crossYear = zonedParts(period.start, timeZone).year !== zonedParts(period.end, timeZone).year
  return `${formatZonedDateTime(period.start, timeZone, crossYear)} → ${formatZonedDateTime(period.end, timeZone, crossYear)}`
}

/** 距离下次重置的剩余天数（向上取整）。 */
export function daysUntil(iso: string, now: number): number | null {
  const end = Date.parse(iso)
  if (Number.isNaN(end)) return null
  return Math.max(0, Math.ceil((end - now) / 86_400_000))
}

/** 时间差的简短文本：秒、分钟、小时、天。 */
export function formatAge(iso: string | null | undefined, now: number, t: Translate): string {
  const at = iso ? Date.parse(iso) : NaN
  if (Number.isNaN(at)) return '—'
  const seconds = Math.max(0, Math.round((now - at) / 1000))
  if (seconds < 60) return t('traffic.host.age.seconds', { count: seconds })
  if (seconds < 3600) return t('traffic.host.age.minutes', { count: Math.floor(seconds / 60) })
  if (seconds < 86_400) return t('traffic.host.age.hours', { count: Math.floor(seconds / 3600) })
  return t('traffic.host.age.days', { count: Math.floor(seconds / 86_400) })
}

export function formatDuration(seconds: number, t: Translate): string {
  if (seconds < 3600) return t('traffic.host.duration.minutes', { count: Math.max(1, Math.round(seconds / 60)) })
  if (seconds < 86_400) return t('traffic.host.duration.hours', { count: parseFloat((seconds / 3600).toFixed(1)) })
  return t('traffic.host.duration.days', { count: parseFloat((seconds / 86_400).toFixed(1)) })
}

export function billingModeLabel(mode: HostBillingMode, t: Translate): string {
  return t(`traffic.host.billing.${mode}`)
}

/** 每月重置规则：“每月 24 日 00:00（UTC）”。 */
export function resetRuleText(plan: Pick<HostTrafficPlan, 'reset_day' | 'reset_time' | 'timezone'>, t: Translate): string {
  return t('traffic.host.resetRule', { day: plan.reset_day, time: plan.reset_time, timezone: plan.timezone })
}

/** 从 “下次重置日期” 推出每月重置日；31 日之外的月末日期可能是被短月截断，需要提示用户确认。 */
export function resetDayFromDate(date: string): { day: number; maybeClamped: boolean } | null {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(date)
  if (!match) return null
  const year = Number(match[1])
  const month = Number(match[2])
  const day = Number(match[3])
  if (month < 1 || month > 12 || day < 1) return null
  const lastDay = new Date(Date.UTC(year, month, 0)).getUTCDate()
  if (day > lastDay) return null
  return { day, maybeClamped: day === lastDay && day < 31 }
}

/** 根据重置日推算下一个重置日期（套餐时区下的日期），用于编辑时回填表单。 */
export function nextResetDate(period: HostTrafficPeriod | null, plan: HostTrafficPlan | null): string {
  if (period && plan && isValidTimeZone(plan.timezone)) return zonedDate(period.end, plan.timezone)
  return ''
}

export type QuotaTone = 'normal' | 'warning' | 'critical' | 'over'

export function quotaTone(period: HostTrafficPeriod, plan: HostTrafficPlan): QuotaTone {
  if (period.overage_bytes !== null) return 'over'
  const percent = period.used_ratio * 100
  if (percent >= plan.alerts.critical_percent) return 'critical'
  if (percent >= plan.alerts.warning_percent) return 'warning'
  return 'normal'
}

export type MeterSegmentKey = 'rx' | 'tx' | 'adjustment'

export interface QuotaMeter {
  // 各段宽度为整条的百分比；超额时整条代表计费量，额度线落在 quotaAt。
  segments: { key: MeterSegmentKey; width: number }[]
  quotaAt: number | null
  // 提醒线、严重线的位置（百分比）。
  thresholds: { percent: number; at: number }[]
  // 时间进度：按账期已过时间均匀使用时应达到的位置；elapsed 为 0–1。
  pace: { at: number; elapsed: number } | null
}

/** 账期已过去的比例（0–1）。 */
export function periodElapsed(period: Pick<HostTrafficPeriod, 'start' | 'end'>, now: number): number | null {
  const start = Date.parse(period.start)
  const end = Date.parse(period.end)
  if (Number.isNaN(start) || Number.isNaN(end) || end <= start) return null
  return Math.min(1, Math.max(0, (now - start) / (end - start)))
}

/**
 * 用量条：按计费方式拆成入站、出站（较大方向或单方向）与服务商补齐段，
 * 段宽相对 max(额度, 计费量)；校准量为负时按比例缩小实测段，使各段之和等于计费量。
 */
export function quotaMeter(period: HostTrafficPeriod, plan: HostTrafficPlan, now: number): QuotaMeter {
  const quota = parseByteString(period.quota_bytes)
  const billed = parseByteString(period.billed_bytes)
  const empty: QuotaMeter = { segments: [], quotaAt: null, thresholds: [], pace: null }
  if (quota === null || quota <= 0n || billed === null) return empty
  const rx = parseByteString(period.rx_bytes) ?? 0n
  const tx = parseByteString(period.tx_bytes) ?? 0n
  const adjustment = parseSignedByteString(period.adjustment_bytes) ?? 0n
  const scale = billed > quota ? billed : quota
  const pct = (value: bigint) => Number((value * 100000n) / scale) / 1000

  let parts: { key: 'rx' | 'tx'; bytes: bigint }[]
  switch (plan.billing_mode) {
    case 'outbound':
      parts = [{ key: 'tx', bytes: tx }]
      break
    case 'inbound':
      parts = [{ key: 'rx', bytes: rx }]
      break
    case 'max':
      parts = [rx >= tx ? { key: 'rx', bytes: rx } : { key: 'tx', bytes: tx }]
      break
    default:
      parts = [
        { key: 'rx', bytes: rx },
        { key: 'tx', bytes: tx }
      ]
  }
  const measured = parts.reduce((sum, part) => sum + part.bytes, 0n)
  if (adjustment < 0n && measured > 0n) {
    parts = parts.map((part) => ({ ...part, bytes: (part.bytes * billed) / measured }))
  }
  const segments: QuotaMeter['segments'] = parts.filter((part) => part.bytes > 0n).map((part) => ({ key: part.key, width: pct(part.bytes) }))
  if (adjustment > 0n) segments.push({ key: 'adjustment', width: pct(adjustment) })

  const ratio = Number((quota * 100000n) / scale) / 100000
  const thresholds = plan.alerts.enabled
    ? [plan.alerts.warning_percent, plan.alerts.critical_percent]
        .filter((value) => value > 0 && value < 100)
        .map((value) => ({ percent: value, at: value * ratio }))
    : []
  const elapsed = periodElapsed(period, now)
  return {
    segments,
    quotaAt: billed > quota ? ratio * 100 : null,
    thresholds,
    pace: elapsed === null ? null : { at: elapsed * ratio * 100, elapsed }
  }
}

/** 已用量的修饰词：精确不加，含校准为 “约”，下限为 “至少”。 */
export function boundPrefix(bound: HostBound, t: Translate): string {
  if (bound === 'estimate') return t('traffic.host.bound.about')
  if (bound === 'lower') return t('traffic.host.bound.atLeast')
  return ''
}

/** 剩余量的修饰词：已用量是下限时，剩余量是上限。 */
export function remainingPrefix(bound: HostBound, t: Translate): string {
  if (bound === 'estimate') return t('traffic.host.bound.about')
  if (bound === 'lower') return t('traffic.host.bound.atMost')
  return ''
}

export type HostCardMode = 'unavailable' | 'needs_host_mount' | 'unsupported' | 'starting' | 'no_plan' | 'quota'

export function hostCardMode(status: HostTrafficStatus | null): HostCardMode {
  if (!status) return 'unavailable'
  const state = status.collection.state
  if (state === 'needs_host_mount') return 'needs_host_mount'
  if (state === 'unsupported') return 'unsupported'
  if (status.plan && status.period) return 'quota'
  if (state === 'starting' && !status.since_start) return 'starting'
  return 'no_plan'
}

/** 计费网卡的显示名；自动选择且有歧义时由调用方提示。 */
export function selectedInterfaces(status: HostTrafficStatus): string[] {
  return status.interfaces.filter((item) => item.selected).map((item) => item.name)
}
