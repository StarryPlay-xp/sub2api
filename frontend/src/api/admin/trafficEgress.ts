import { apiClient } from '../client'
import type { TrafficPreviousWindow } from './trafficSubjects'
import type { TrafficInterval, TrafficPeak, TrafficRange } from './wireguard'

// 服务流量上游两方向按出口（直连 / 代理 / WG 节点）与上游账号归因（service_payload_v1）。
// 只统计本站与上游之间的应用载荷；客户端两方向不参与。不返回代理地址、端口、凭据或 WG 配置。
export type EgressRouteType = 'direct' | 'proxy' | 'wireguard'
// out = 本站 → 上游，in = 上游 → 本站；字节为十进制整数字符串。
export interface UpstreamBytes {
  upstream_out: string
  upstream_in: string
}
// 缺少某个键或整体为 null 表示峰值未知，不能当作 0。
export type EgressPeaks = Partial<Record<'out' | 'in' | 'total', TrafficPeak>> | null

export interface EgressRouteIdentity {
  route_type: EgressRouteType
  route_id: number | null
  route_name: string | null
  protocol: string | null
  route_deleted: boolean
}

export interface EgressWireGuardBytes {
  tx_bytes: string
  rx_bytes: string
  total_bytes: string
  has_records: boolean
}

export interface EgressRouteStats extends EgressRouteIdentity {
  bytes: UpstreamBytes
  total_bytes: string
  // 占全部上游流量（含未识别出口）的比例，0–1。
  share: number | null
  // 应用传输或连接租约尝试次数，不等于底层 TCP 连接数或计费请求数。
  attempts: number
  peaks: EgressPeaks
  account_count: number
  // 仅 WG 出口：与服务流量同一小时窗口的隧道字节。
  wireguard: EgressWireGuardBytes | null
  previous: { total_bytes: string } | null
  growth_bytes: string | null
}

export interface EgressAccountStats {
  account_id: number
  account_name: string
  platform: string
  account_type: string
  account_deleted: boolean
  bytes: UpstreamBytes
  total_bytes: string
  // 占该出口流量的比例。
  share: number | null
  attempts: number
  peaks: EgressPeaks
  previous: { total_bytes: string } | null
  growth_bytes: string | null
}

export interface EgressWindow {
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
  egress_started_at: string | null
  has_records: boolean
  incomplete: boolean
  concurrent_runtimes: boolean
  overflow: boolean
}

export interface EgressTotal {
  bytes: UpstreamBytes
  total_bytes: string
}

export interface TrafficEgressData {
  window: EgressWindow
  previous_window: TrafficPreviousWindow | null
  routes: EgressRouteStats[]
  totals: {
    upstream: EgressTotal
    unidentified: EgressTotal
  }
}

export interface EgressHistoryBucket {
  start: string
  interval: 'hour'
  bytes: UpstreamBytes
  attempts: number
  peaks: EgressPeaks
}

export interface TrafficEgressDetailData {
  window: EgressWindow
  previous_window: TrafficPreviousWindow | null
  summary: EgressRouteStats
  history: EgressHistoryBucket[]
  accounts: EgressAccountStats[]
  // 缺失账号或超出容量后的残余，不放进 accounts。
  unattributed_account: EgressTotal & { attempts: number }
}

export interface TrafficEgressParams {
  range: TrafficRange
  start?: string
  end?: string
  compare?: boolean
}

interface RequestOptions {
  signal?: AbortSignal
}

function query(params: TrafficEgressParams): Record<string, string> {
  const result: Record<string, string> = { range: params.range }
  if (params.range === 'custom') {
    if (params.start) result.start = params.start
    if (params.end) result.end = params.end
  }
  if (params.compare) result.compare = 'previous'
  return result
}

export async function getTrafficEgress(
  params: TrafficEgressParams,
  options: RequestOptions = {}
): Promise<TrafficEgressData> {
  const { data } = await apiClient.get<TrafficEgressData>('/admin/ops/traffic-egress', {
    params: query(params),
    signal: options.signal
  })
  return data
}

/** 直连使用 direct/0。 */
export async function getTrafficEgressRoute(
  type: EgressRouteType,
  id: number,
  params: TrafficEgressParams,
  options: RequestOptions = {}
): Promise<TrafficEgressDetailData> {
  const routeId = type === 'direct' ? 0 : id
  const { data } = await apiClient.get<TrafficEgressDetailData>(`/admin/ops/traffic-egress/${type}/${routeId}`, {
    params: query(params),
    signal: options.signal
  })
  return data
}
