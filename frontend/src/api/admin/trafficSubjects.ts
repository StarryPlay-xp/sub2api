import { apiClient } from '../client'
import type { ServiceTrafficBytes } from './ops'
import type { TrafficInterval, TrafficPeak, TrafficRange } from './wireguard'

// 服务流量按用户 / API Key 归因（service_payload_v1）：只含字节计数和主体标识，不含请求内容或 Key 明文。
// 字节总数是十进制整数字符串，速率是 B/s 浮点数；接口只接受下列参数，多余参数会被拒绝。
export type TrafficSubjectType = 'user' | 'api_key'
export type SubjectPeakKey = 'in' | 'out' | 'total'
// 缺少某个键或整体为 null 表示峰值未知（多实例重叠、升级前历史），不能当作 0 速率。
export type SubjectPeaks = Partial<Record<SubjectPeakKey, TrafficPeak>> | null
export type TrafficRankingSort =
  | 'total_bytes'
  | 'in_bytes'
  | 'out_bytes'
  | 'requests'
  | 'bytes_per_request'
  | 'peak_total'
  | 'peak_in'
  | 'peak_out'
  | 'active_avg_rate'
  | 'growth'
export type TrafficSortOrder = 'desc' | 'asc'

export interface TrafficSubjectIdentity {
  subject_type: TrafficSubjectType
  user_id: number
  user_email: string
  username: string
  user_deleted: boolean
  api_key_id: number | null
  api_key_name: string | null
  api_key_deleted: boolean | null
  group_id: number | null
  group_name: string | null
}

export interface TrafficSubjectStats extends TrafficSubjectIdentity {
  bytes: ServiceTrafficBytes
  in_bytes: string
  out_bytes: string
  total_bytes: string
  // 占全站（已归属 + 未归属）的比例，0–1；全站为 0 时 null。
  share: number | null
  requests: number
  bytes_per_request: string | null
  active_seconds: number
  active_avg_bytes_per_second: number | null
  peaks: SubjectPeaks
  active_api_keys: number | null
  previous: { total_bytes: string; peaks: SubjectPeaks } | null
  // 有符号十进制字符串：当前期间减上一时段。
  growth_bytes: string | null
}

export interface TrafficSubjectWindow {
  range: TrafficRange
  interval: TrafficInterval
  start: string
  end: string
  requested_start: string
  requested_end: string
  retained_from: string
  as_of: string
  end_clamped: boolean
  metric_basis: 'service_payload_v1'
  // 最早的用户/Key 维度样本时间；更早的期间没有分维度数据。
  attribution_started_at: string | null
  has_records: boolean
  incomplete: boolean
  concurrent_runtimes: boolean
  subject_overflow: boolean
}

export interface TrafficPreviousWindow {
  start: string
  end: string
  complete: boolean
}

export interface TrafficRankingTotals {
  attributed: { bytes: ServiceTrafficBytes; total_bytes: string; requests: number }
  unattributed: { bytes: ServiceTrafficBytes; total_bytes: string }
  all_total_bytes: string
}

export interface TrafficConcentration {
  subjects: number
  top1: number | null
  top5: number | null
  top10: number | null
}

export interface TrafficRankingData {
  window: TrafficSubjectWindow
  previous_window: TrafficPreviousWindow | null
  subject: TrafficSubjectType
  sort: TrafficRankingSort
  order: TrafficSortOrder
  page: number
  page_size: number
  total: number
  items: TrafficSubjectStats[]
  totals: TrafficRankingTotals
  concentration: TrafficConcentration
}

export interface TrafficSubjectBucket {
  start: string
  interval: 'hour'
  bytes: ServiceTrafficBytes
  requests: number
  active_seconds: number
  peaks: SubjectPeaks
}

export interface TrafficSubjectDetailData {
  window: TrafficSubjectWindow
  previous_window: TrafficPreviousWindow | null
  summary: TrafficSubjectStats
  history: TrafficSubjectBucket[]
}

export interface RateTriple {
  in: number
  out: number
  total: number
}

export type TrafficLiveState = 'warming_up' | 'sampled' | 'invalid_interval'
export type TrafficLiveSort = 'avg_60s' | 'rate'

export interface TrafficLiveItem extends TrafficSubjectIdentity {
  rate: RateTriple
  avg_60s: RateTriple
  avg_60s_seconds: number
  in_flight: number
  last_active_at: string | null
}

export interface TrafficLiveData {
  subject: TrafficSubjectType
  state: TrafficLiveState
  sampled_at: string
  window_seconds: number
  host: { rate: RateTriple; avg_60s: RateTriple }
  unattributed: { rate: RateTriple; avg_60s: RateTriple }
  items: TrafficLiveItem[]
}

export interface TrafficWindowParams {
  range: TrafficRange
  start?: string
  end?: string
}

export interface TrafficRankingParams extends TrafficWindowParams {
  subject: TrafficSubjectType
  sort?: TrafficRankingSort
  order?: TrafficSortOrder
  page?: number
  page_size?: number
  q?: string
  user_id?: number
  compare?: boolean
}

export interface TrafficSubjectParams extends TrafficWindowParams {
  compare?: boolean
}

export interface TrafficLiveParams {
  subject: TrafficSubjectType
  limit?: number
  sort?: TrafficLiveSort
}

interface RequestOptions {
  signal?: AbortSignal
}

// 只发送有值的参数：空搜索词、未开启的环比都不带，避免触发后端的未知参数校验。
function windowQuery(params: TrafficWindowParams): Record<string, string | number> {
  const query: Record<string, string | number> = { range: params.range }
  if (params.range === 'custom') {
    if (params.start) query.start = params.start
    if (params.end) query.end = params.end
  }
  return query
}

export async function getTrafficRanking(
  params: TrafficRankingParams,
  options: RequestOptions = {}
): Promise<TrafficRankingData> {
  const query = windowQuery(params)
  query.subject = params.subject
  if (params.sort) query.sort = params.sort
  if (params.order) query.order = params.order
  if (params.page) query.page = params.page
  if (params.page_size) query.page_size = params.page_size
  const q = params.q?.trim()
  if (q) query.q = q
  if (params.user_id && params.subject === 'api_key') query.user_id = params.user_id
  if (params.compare) query.compare = 'previous'
  const { data } = await apiClient.get<TrafficRankingData>('/admin/ops/traffic-ranking', {
    params: query,
    signal: options.signal
  })
  return data
}

export async function getTrafficSubject(
  type: TrafficSubjectType,
  id: number,
  params: TrafficSubjectParams,
  options: RequestOptions = {}
): Promise<TrafficSubjectDetailData> {
  const query = windowQuery(params)
  if (params.compare) query.compare = 'previous'
  const { data } = await apiClient.get<TrafficSubjectDetailData>(`/admin/ops/traffic-subjects/${type}/${id}`, {
    params: query,
    signal: options.signal
  })
  return data
}

export async function getTrafficLive(
  params: TrafficLiveParams,
  options: RequestOptions = {}
): Promise<TrafficLiveData> {
  const query: Record<string, string | number> = { subject: params.subject }
  if (params.limit) query.limit = params.limit
  if (params.sort) query.sort = params.sort
  const { data } = await apiClient.get<TrafficLiveData>('/admin/ops/traffic-live', {
    params: query,
    signal: options.signal
  })
  return data
}

export const trafficSubjectsAPI = { getTrafficRanking, getTrafficSubject, getTrafficLive }
