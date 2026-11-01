import type { ServiceTrafficReport } from '@/api/admin/ops'
import { formatSampleTime, isMissingTime } from '@/utils/trafficFormat'

type Translate = (key: string, params?: Record<string, unknown>) => string

export interface ReportWarning {
  key: string
  text: string
}

// 首个样本明显晚于所选期间开始（超过一个时间桶）时，提示峰值不覆盖整个期间。
export function coverageStartText(report: ServiceTrafficReport | null): string {
  const first = report?.quality?.first_sample_at
  if (!report?.has_records || !first || isMissingTime(first)) return ''
  const grain = report.interval === 'minute' ? 60_000 : 3_600_000
  return Date.parse(first) - Date.parse(report.start) > grain ? formatSampleTime(first) : ''
}

/** 整实例服务流量的数据可信度提示，带宽、流量两页集中放在最上方。 */
export function reportWarnings(
  report: ServiceTrafficReport | null,
  persistenceError: boolean,
  t: Translate
): ReportWarning[] {
  const list: ReportWarning[] = []
  if (report?.concurrent_runtimes) list.push({ key: 'concurrent', text: t('bandwidth.warnings.concurrent') })
  if (report?.incomplete) list.push({ key: 'incomplete', text: t('bandwidth.warnings.incomplete') })
  const invalid = report?.quality?.invalid_samples ?? 0
  if (invalid > 0) list.push({ key: 'invalid', text: t('bandwidth.warnings.invalidSamples', { count: invalid }) })
  const coverage = coverageStartText(report)
  if (coverage) list.push({ key: 'coverage', text: t('bandwidth.warnings.partialCoverage', { time: coverage }) })
  if (report && !report.has_records) list.push({ key: 'empty', text: t('bandwidth.warnings.noRecords') })
  if (persistenceError) list.push({ key: 'persistence', text: t('bandwidth.warnings.persistenceError') })
  return list
}
