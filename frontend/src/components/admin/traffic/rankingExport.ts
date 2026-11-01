import { getTrafficRanking, type TrafficRankingParams, type TrafficSubjectStats } from '@/api/admin/trafficSubjects'

export const EXPORT_PAGE_SIZE = 200
export const EXPORT_MAX_ROWS = 10_000

type Translate = (key: string, params?: Record<string, unknown>) => string

/** 按当前筛选和排序逐页取完整排行；超过上限时截断并告知调用方。 */
export async function fetchRankingForExport(
  params: Omit<TrafficRankingParams, 'page' | 'page_size'>,
  signal?: AbortSignal
): Promise<{ items: TrafficSubjectStats[]; truncated: boolean }> {
  const items: TrafficSubjectStats[] = []
  for (let page = 1; items.length < EXPORT_MAX_ROWS; page++) {
    const result = await getTrafficRanking({ ...params, page, page_size: EXPORT_PAGE_SIZE }, { signal })
    items.push(...result.items)
    if (result.items.length < EXPORT_PAGE_SIZE || items.length >= result.total) {
      return { items, truncated: false }
    }
  }
  return { items: items.slice(0, EXPORT_MAX_ROWS), truncated: true }
}

// 名称和邮箱由用户填写：以 = + - @ 开头时加前缀，防止表格软件当作公式执行。
function textCell(value: string | null | undefined): string {
  const text = value ?? ''
  const guarded = /^[=+\-@\t\r]/.test(text) ? `'${text}` : text
  return /[",\r\n]/.test(guarded) ? `"${guarded.replace(/"/g, '""')}"` : guarded
}

function numberCell(value: string | number | null | undefined): string {
  if (value === null || value === undefined) return ''
  if (typeof value === 'number') return Number.isFinite(value) ? String(value) : ''
  return /^-?\d+(\.\d+)?$/.test(value) ? value : ''
}

const flag = (value: boolean | null | undefined) => (value === null || value === undefined ? '' : value ? '1' : '0')

/** 原始字节和 B/s 数值导出，方便在表格中自行换算；时间为 RFC3339。 */
export function rankingCsv(items: TrafficSubjectStats[], t: Translate): string {
  const columns: Array<[string, (item: TrafficSubjectStats, index: number) => string]> = [
    ['rank', (_item, index) => String(index + 1)],
    ['subjectType', (item) => textCell(t(`traffic.subject.${item.subject_type}`))],
    ['userId', (item) => numberCell(item.user_id)],
    ['userEmail', (item) => textCell(item.user_email)],
    ['username', (item) => textCell(item.username)],
    ['userDeleted', (item) => flag(item.user_deleted)],
    ['apiKeyId', (item) => numberCell(item.api_key_id)],
    ['apiKeyName', (item) => textCell(item.api_key_name)],
    ['apiKeyDeleted', (item) => flag(item.api_key_deleted)],
    ['groupName', (item) => textCell(item.group_name)],
    ['totalBytes', (item) => numberCell(item.total_bytes)],
    ['inBytes', (item) => numberCell(item.in_bytes)],
    ['outBytes', (item) => numberCell(item.out_bytes)],
    ['clientIn', (item) => numberCell(item.bytes.client_in)],
    ['clientOut', (item) => numberCell(item.bytes.client_out)],
    ['upstreamOut', (item) => numberCell(item.bytes.upstream_out)],
    ['upstreamIn', (item) => numberCell(item.bytes.upstream_in)],
    ['sharePercent', (item) => numberCell(item.share === null ? null : Number((item.share * 100).toFixed(4)))],
    ['requests', (item) => numberCell(item.requests)],
    ['bytesPerRequest', (item) => numberCell(item.bytes_per_request)],
    ['activeSeconds', (item) => numberCell(item.active_seconds)],
    ['activeAvgRate', (item) => numberCell(item.active_avg_bytes_per_second)],
    ['peakTotal', (item) => numberCell(item.peaks?.total?.bytes_per_second)],
    ['peakTotalAt', (item) => textCell(item.peaks?.total?.at)],
    ['peakIn', (item) => numberCell(item.peaks?.in?.bytes_per_second)],
    ['peakOut', (item) => numberCell(item.peaks?.out?.bytes_per_second)],
    ['previousTotalBytes', (item) => numberCell(item.previous?.total_bytes)],
    ['growthBytes', (item) => numberCell(item.growth_bytes)]
  ]
  const header = columns.map(([key]) => textCell(t(`traffic.csv.${key}`))).join(',')
  const rows = items.map((item, index) => columns.map(([, cell]) => cell(item, index)).join(','))
  return [header, ...rows].join('\r\n')
}

export function rankingFileName(subject: string, range: string, now = new Date()): string {
  const pad = (value: number) => String(value).padStart(2, '0')
  const stamp = `${now.getFullYear()}${pad(now.getMonth() + 1)}${pad(now.getDate())}-${pad(now.getHours())}${pad(now.getMinutes())}`
  return `traffic-ranking_${subject}_${range}_${stamp}.csv`
}
