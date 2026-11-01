import { beforeEach, describe, expect, it, vi } from 'vitest'
import { getTrafficRanking } from '@/api/admin/trafficSubjects'
import { EXPORT_MAX_ROWS, EXPORT_PAGE_SIZE, fetchRankingForExport, rankingCsv, rankingFileName } from '../rankingExport'
import { keyIdentity, rankingData, subjectStats, userIdentity } from './fixtures'

vi.mock('@/api/admin/trafficSubjects', () => ({ getTrafficRanking: vi.fn() }))

const t = (key: string) => key

beforeEach(() => {
  vi.mocked(getTrafficRanking).mockReset()
})

describe('ranking export', () => {
  it('pages through the full ranking with the current filters', async () => {
    const page = (count: number) => Array.from({ length: count }, (_, index) => subjectStats(userIdentity(index + 1)))
    vi.mocked(getTrafficRanking)
      .mockResolvedValueOnce(rankingData({ items: page(EXPORT_PAGE_SIZE), total: EXPORT_PAGE_SIZE + 3 }))
      .mockResolvedValueOnce(rankingData({ items: page(3), total: EXPORT_PAGE_SIZE + 3 }))

    const result = await fetchRankingForExport({ range: '7d', subject: 'user', sort: 'peak_total', order: 'asc', q: 'a' })

    expect(result.items).toHaveLength(EXPORT_PAGE_SIZE + 3)
    expect(result.truncated).toBe(false)
    expect(vi.mocked(getTrafficRanking).mock.calls.map((call) => call[0])).toEqual([
      { range: '7d', subject: 'user', sort: 'peak_total', order: 'asc', q: 'a', page: 1, page_size: EXPORT_PAGE_SIZE },
      { range: '7d', subject: 'user', sort: 'peak_total', order: 'asc', q: 'a', page: 2, page_size: EXPORT_PAGE_SIZE }
    ])
  })

  it('stops at the row limit and reports truncation', async () => {
    const full = Array.from({ length: EXPORT_PAGE_SIZE }, (_, index) => subjectStats(userIdentity(index + 1)))
    vi.mocked(getTrafficRanking).mockResolvedValue(rankingData({ items: full, total: EXPORT_MAX_ROWS * 2 }))
    const result = await fetchRankingForExport({ range: '24h', subject: 'user' })
    expect(result.items).toHaveLength(EXPORT_MAX_ROWS)
    expect(result.truncated).toBe(true)
  })

  it('writes raw numbers and guards user-provided text against formulas', () => {
    const csv = rankingCsv(
      [
        subjectStats(keyIdentity(9, 7, { api_key_name: '=HYPERLINK("x")', user_email: '+evil@example.com', group_name: 'a,b' }), {
          share: 0.123456,
          growth_bytes: '-2048',
          previous: { total_bytes: '4096', peaks: null }
        })
      ],
      t
    )
    const [header, row] = csv.split('\r\n')
    expect(header.startsWith('traffic.csv.rank,traffic.csv.subjectType')).toBe(true)
    const cells = row.match(/("([^"]|"")*"|[^,]*)(,|$)/g)!.map((cell) => cell.replace(/,$/, ''))
    expect(cells[1]).toBe('traffic.subject.api_key')
    expect(cells[3]).toBe("'+evil@example.com")
    expect(cells[7]).toBe(`"'=HYPERLINK(""x"")"`)
    expect(cells[9]).toBe('"a,b"')
    expect(cells[10]).toBe('1610615808')
    expect(cells[17]).toBe('12.3456')
    expect(cells[27]).toBe('-2048')
  })

  it('names files by subject, range and local time', () => {
    expect(rankingFileName('api_key', '7d', new Date(2026, 10, 1, 9, 5))).toBe('traffic-ranking_api_key_7d_20261101-0905.csv')
  })
})
