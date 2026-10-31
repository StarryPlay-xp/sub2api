export type DateMode = 'local' | 'utc'

const pad2 = (value: number) => String(value).padStart(2, '0')

export function parseDateInput(value: string, mode: DateMode): Date | null {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value.trim())
  if (!match) return null
  const year = Number(match[1])
  const month = Number(match[2])
  const day = Number(match[3])
  if (month < 1 || month > 12 || day < 1 || day > 31) return null
  const date =
    mode === 'utc'
      ? new Date(Date.UTC(year, month - 1, day))
      : new Date(year, month - 1, day)
  const valid =
    mode === 'utc'
      ? date.getUTCFullYear() === year &&
        date.getUTCMonth() === month - 1 &&
        date.getUTCDate() === day
      : date.getFullYear() === year &&
        date.getMonth() === month - 1 &&
        date.getDate() === day
  return valid ? date : null
}

export function formatDateInput(date: Date, mode: DateMode): string {
  if (mode === 'utc') {
    return `${date.getUTCFullYear()}-${pad2(date.getUTCMonth() + 1)}-${pad2(date.getUTCDate())}`
  }
  return `${date.getFullYear()}-${pad2(date.getMonth() + 1)}-${pad2(date.getDate())}`
}

// 按日历加一天，跨月、跨年以及夏令时切换都不依赖固定 24 小时。
export function nextCalendarDay(date: Date, mode: DateMode): Date {
  if (mode === 'utc') {
    return new Date(
      Date.UTC(date.getUTCFullYear(), date.getUTCMonth(), date.getUTCDate() + 1)
    )
  }
  return new Date(date.getFullYear(), date.getMonth(), date.getDate() + 1)
}

export function utcOffsetLabel(date: Date): string {
  const offset = -date.getTimezoneOffset()
  const sign = offset >= 0 ? '+' : '-'
  const absolute = Math.abs(offset)
  return `UTC${sign}${pad2(Math.floor(absolute / 60))}:${pad2(absolute % 60)}`
}

// 本地模式下保留当天实际偏移，避免把 +08:00 之类的时区信息丢掉。
export function formatRFC3339(date: Date, mode: DateMode): string {
  if (mode === 'utc') return date.toISOString()
  const offset = -date.getTimezoneOffset()
  const sign = offset >= 0 ? '+' : '-'
  const absolute = Math.abs(offset)
  return `${date.getFullYear()}-${pad2(date.getMonth() + 1)}-${pad2(date.getDate())}T${pad2(date.getHours())}:${pad2(date.getMinutes())}:${pad2(date.getSeconds())}${sign}${pad2(Math.floor(absolute / 60))}:${pad2(absolute % 60)}`
}

export function isUtcBucketAligned(date: Date, grain: 'minute' | 'hour' = 'hour'): boolean {
  const size = grain === 'minute' ? 60_000 : 3_600_000
  return date.getTime() % size === 0
}

// 最早可选的本地/UTC 日期：该日零点必须不早于 retained_from，否则顺延一天。
export function firstSelectableDate(retainedFrom: Date, mode: DateMode): Date {
  const midnight =
    mode === 'utc'
      ? new Date(
          Date.UTC(
            retainedFrom.getUTCFullYear(),
            retainedFrom.getUTCMonth(),
            retainedFrom.getUTCDate()
          )
        )
      : new Date(
          retainedFrom.getFullYear(),
          retainedFrom.getMonth(),
          retainedFrom.getDate()
        )
  return midnight.getTime() < retainedFrom.getTime()
    ? nextCalendarDay(midnight, mode)
    : midnight
}

export function timeZoneName(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC'
  } catch {
    return 'UTC'
  }
}

export function formatDateTimeLabel(value: string | Date): string {
  const date = typeof value === 'string' ? new Date(value) : value
  if (Number.isNaN(date.getTime())) return '—'
  return new Intl.DateTimeFormat(undefined, {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false
  }).format(date)
}

export function formatDateLabel(value: string | Date): string {
  const date = typeof value === 'string' ? new Date(value) : value
  if (Number.isNaN(date.getTime())) return '—'
  return new Intl.DateTimeFormat(undefined, {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit'
  }).format(date)
}
