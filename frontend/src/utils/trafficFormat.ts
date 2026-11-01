// 流量与带宽展示：字节总量是十进制整数字符串，用 BigInt 换算避免超过 2^53 后丢精度；
// 速率是 B/s 浮点数。字节单位按 1024 进位，Mbps 按 bytes_per_second * 8 / 1_000_000 计算，二者不混用。

const BYTE_UNITS = ['B', 'KiB', 'MiB', 'GiB', 'TiB', 'PiB', 'EiB']
const RATE_UNITS = ['B/s', 'KiB/s', 'MiB/s', 'GiB/s', 'TiB/s']

function toBigInt(value: string | number | bigint | null | undefined): bigint | null {
  if (typeof value === 'bigint') return value
  if (typeof value === 'number') {
    return Number.isFinite(value) ? BigInt(Math.round(value)) : null
  }
  const text = String(value ?? '').trim()
  // BigInt('') 为 0n，空串必须视为未知而不是 0。
  return /^\d+$/.test(text) ? BigInt(text) : null
}

/** 格式化累计字节数；无法解析时返回 “—”，不会伪装成 0。 */
export function formatByteCount(
  value: string | number | bigint | null | undefined,
  decimals = 2
): string {
  const bytes = toBigInt(value)
  if (bytes === null || bytes < 0n) return '—'
  let unit = 0
  let divisor = 1n
  while (unit < BYTE_UNITS.length - 1 && bytes >= divisor * 1024n) {
    divisor *= 1024n
    unit++
  }
  if (unit === 0) return `${bytes} B`
  const scale = 10n ** BigInt(decimals)
  let scaled = (bytes * scale + divisor / 2n) / divisor
  // 四舍五入后达到 1024 时进到下一个单位，避免出现 “1024 KiB”。
  if (scaled >= 1024n * scale && unit < BYTE_UNITS.length - 1) {
    unit++
    divisor *= 1024n
    scaled = (bytes * scale + divisor / 2n) / divisor
  }
  const text = String(parseFloat((Number(scaled) / Number(scale)).toFixed(decimals)))
  return `${text} ${BYTE_UNITS[unit]}`
}

/** 格式化 B/s 速率；负数或非有限值返回 “—”。 */
export function formatByteRate(bytesPerSecond: number | null | undefined, decimals = 2): string {
  if (typeof bytesPerSecond !== 'number' || !Number.isFinite(bytesPerSecond) || bytesPerSecond < 0) {
    return '—'
  }
  let size = bytesPerSecond
  let unit = 0
  while (size >= 1024 && unit < RATE_UNITS.length - 1) {
    size /= 1024
    unit++
  }
  if (unit === 0) {
    // 小于 10 B/s 的非零值保留一位小数，避免四舍五入成 “0 B/s” 被误读为零速率。
    const text = size > 0 && size < 10 ? String(parseFloat(size.toFixed(1))) : String(Math.round(size))
    return `${text} ${RATE_UNITS[0]}`
  }
  return `${parseFloat(size.toFixed(decimals))} ${RATE_UNITS[unit]}`
}

/** 将 B/s 换算为十进制 Mbps（1 Mbps = 1,000,000 bit/s）。 */
export function formatMbps(bytesPerSecond: number | null | undefined): string {
  if (typeof bytesPerSecond !== 'number' || !Number.isFinite(bytesPerSecond) || bytesPerSecond < 0) {
    return '—'
  }
  const mbps = (bytesPerSecond * 8) / 1_000_000
  if (mbps === 0) return '0 Mbps'
  const digits = mbps < 1 ? 3 : mbps < 100 ? 2 : 1
  return `${parseFloat(mbps.toFixed(digits))} Mbps`
}

/** Go 零值时间（0001-01-01）及无法解析的时间都视为缺失。 */
export function isMissingTime(value: string | null | undefined): boolean {
  if (!value) return true
  const ms = Date.parse(value)
  return Number.isNaN(ms) || new Date(ms).getUTCFullYear() < 1970
}

/** 精确到秒的本地时间，用于峰值采样结束时间和最近采样时间。 */
export function formatSampleTime(value: string | null | undefined): string {
  if (isMissingTime(value)) return '—'
  return new Intl.DateTimeFormat(undefined, {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false
  }).format(new Date(value as string))
}
