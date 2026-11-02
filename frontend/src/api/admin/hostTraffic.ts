import { apiClient } from '../client'
import type { TrafficRange } from './wireguard'

export type HostUsageRange = Exclude<TrafficRange, 'custom'>

// 本机流量套餐：sub2api 每整分钟读取所在 VPS 的网卡累计计数（整机口径，含其他程序与协议开销），
// 按套餐账期汇总。与服务流量（service_payload_v1）口径不同，二者不能相加。
// 字节量为十进制整数字符串；adjustment_bytes 可带前导 "-"。rx = 入站，tx = 出站，均以 VPS 为参照。

// stale：最近一次读数早于 3 分钟；needs_host_mount：容器内看不到宿主机网卡。
export type HostCollectionState = 'ok' | 'starting' | 'stale' | 'needs_host_mount' | 'unsupported' | 'error'
export type HostBillingMode = 'both' | 'outbound' | 'inbound' | 'max'
export type HostQuotaUnit = 'GB' | 'TB'
export type HostUsedUnit = 'MB' | 'GB' | 'TB'
export type HostUnitBase = 1000 | 1024
// exact：期初完整且无明显缺口；estimate：含服务商校准；lower：已用量只是下限。
export type HostBound = 'exact' | 'estimate' | 'lower'

export interface HostCollection {
  state: HostCollectionState
  deployment: 'docker' | 'binary' | 'unknown'
  source: 'host_mount' | 'native' | null
  started_at: string | null
  last_sample_at: string | null
  host_boot_at: string | null
  error_reason: string | null
}

export interface HostInterface {
  name: string
  selected: boolean
  physical: boolean | null
  // 容器内读不到宿主机路由表时为 null。
  default_route: { ipv4: boolean; ipv6: boolean } | null
  rx_bytes_per_second: number | null
  tx_bytes_per_second: number | null
}

export interface HostAlertSettings {
  enabled: boolean
  warning_percent: number
  critical_percent: number
  projection: boolean
}

export interface HostTrafficPlan {
  version: string
  quota_value: string
  quota_unit: HostQuotaUnit
  quota_bytes: string
  billing_mode: HostBillingMode
  reset_day: number
  // HH:MM，分钟为 0/15/30/45。
  reset_time: string
  timezone: string
  unit_base: HostUnitBase
  // 空数组表示自动选择。
  interfaces: string[]
  alerts: HostAlertSettings
  updated_at: string
}

// at 是配对的基准读数时间；deviation_ratio =（服务商 − 实测）/ 实测，实测为 0 时为 null。
export interface HostReconcile {
  at: string
  provider_bytes: string
  measured_bytes: string
  deviation_ratio: number | null
}

export interface HostTrafficPeriod {
  // 半开区间 [start, end)，end 即下次重置时间。
  start: string
  end: string
  observed_from: string | null
  coverage: 'complete' | 'partial'
  bound: HostBound
  rx_bytes: string
  tx_bytes: string
  measured_billed_bytes: string
  adjustment_bytes: string | null
  adjustment_kind: 'fill' | 'override' | null
  billed_bytes: string
  quota_bytes: string
  remaining_bytes: string | null
  overage_bytes: string | null
  used_ratio: number
  gaps: { lost_seconds: number; minor_restarts: number; unallocated_bytes: string }
  projection: { billed_bytes: string; ratio: number; basis_days: number } | null
  last_reconcile: HostReconcile | null
}

export interface HostAlertFiring {
  level: 'warning' | 'critical'
  kind: 'usage' | 'projection'
  since: string
}

export interface HostTrafficStatus {
  as_of: string
  collection: HostCollection
  interfaces: HostInterface[]
  interface_selection: { mode: 'auto' | 'manual'; ambiguous: boolean }
  since_start: { from: string; rx_bytes: string; tx_bytes: string; total_bytes: string; bound: 'exact' | 'lower' } | null
  plan: HostTrafficPlan | null
  period: HostTrafficPeriod | null
  alerts: { email_enabled: boolean; firing: HostAlertFiring[] }
  unit_hint: { suggested_base: HostUnitBase; samples: number } | null
}

export interface HostUsedValue {
  value: string
  unit: HostUsedUnit
}

export interface HostTrafficPlanInput {
  version: string | null
  quota_value: string
  quota_unit: HostQuotaUnit
  billing_mode: HostBillingMode
  reset_day: number
  reset_time: string
  timezone: string
  unit_base: HostUnitBase
  interfaces: string[]
  alerts: HostAlertSettings
  // 等同保存后立即按 auto 校准；没有新读数时整个请求失败，套餐也不保存。
  current_used: HostUsedValue | null
}

export type HostCalibrationMode = 'auto' | 'override'
export type HostCalibrationKind = 'fill' | 'reconcile' | 'override'

export interface HostCalibrationInput extends HostUsedValue {
  mode: HostCalibrationMode
  plan_version: string
}

export interface HostCalibrationResult {
  kind: HostCalibrationKind
  status: HostTrafficStatus
}

export interface HostTrafficUsage {
  window: { range: HostUsageRange; start: string; end: string; as_of: string; retained_from: string }
  state: HostCollectionState
  interfaces: string[]
  has_records: boolean
  rx_bytes: string
  tx_bytes: string
  total_bytes: string
  observed_seconds: number
  expected_seconds: number
}

// 只含当天可归属的实测，不摊入服务商校准量；跨日恢复的量无法分到具体日期，因此每日合计可能低于账期合计。
// 有套餐时包含尚未到来的日子（expected_seconds 为 0）。
export interface HostTrafficDay {
  date: string
  start: string
  end: string
  rx_bytes: string
  tx_bytes: string
  billed_bytes: string
  observed_seconds: number
  expected_seconds: number
}

export interface HostClosedPeriod {
  start: string
  end: string
  quota_bytes: string
  billing_mode: HostBillingMode
  rx_bytes: string
  tx_bytes: string
  billed_bytes: string
  adjustment_bytes: string | null
  coverage: 'complete' | 'partial'
  bound: HostBound
  closed_at: string
}

export interface HostCalibrationRecord {
  id: number
  at: string
  kind: HostCalibrationKind
  provider_bytes: string
  measured_bytes: string
  deviation_ratio: number | null
  period_start: string
}

export interface HostTrafficDetail {
  // 有套餐时为套餐时区（days 为当前账期每一天），否则为 UTC（最近 30 天）。
  days_timezone: string
  days: HostTrafficDay[]
  periods: HostClosedPeriod[]
  calibrations: HostCalibrationRecord[]
}

interface RequestOptions {
  signal?: AbortSignal
}

const BASE = '/admin/ops/host-traffic'

export async function getHostTraffic(options: RequestOptions = {}): Promise<HostTrafficStatus> {
  const { data } = await apiClient.get<HostTrafficStatus>(BASE, { signal: options.signal })
  return data
}

export async function saveHostTrafficPlan(input: HostTrafficPlanInput, options: RequestOptions = {}): Promise<HostTrafficStatus> {
  const { data } = await apiClient.put<HostTrafficStatus>(`${BASE}/plan`, input, { signal: options.signal })
  return data
}

export async function calibrateHostTraffic(
  input: HostCalibrationInput,
  options: RequestOptions = {}
): Promise<HostCalibrationResult> {
  const { data } = await apiClient.post<HostCalibrationResult>(`${BASE}/calibrations`, input, { signal: options.signal })
  return data
}

export async function getHostTrafficUsage(range: HostUsageRange, options: RequestOptions = {}): Promise<HostTrafficUsage> {
  const { data } = await apiClient.get<HostTrafficUsage>(`${BASE}/usage`, { params: { range }, signal: options.signal })
  return data
}

export async function getHostTrafficDetail(options: RequestOptions = {}): Promise<HostTrafficDetail> {
  const { data } = await apiClient.get<HostTrafficDetail>(`${BASE}/detail`, { signal: options.signal })
  return data
}
