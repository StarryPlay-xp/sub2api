import { apiClient } from '../client'
export interface ExitEvidence { ip?: string; source: string; family: string; observed_at: string; expires_at: string; route: string; node_id?: number; version?: number; runtime_id?: string; state: string }
export interface WGNode { id: number; name: string; config_version: number; active: boolean; paused: boolean; pause_reason?: string; expires_at?: string; tx_bytes: string; rx_bytes: string; saved_at?: string; incomplete: boolean; metric_basis: string }
export interface WGDetails { node: WGNode; rate: { tx_bytes_per_second: number; rx_bytes_per_second: number; window_seconds: number; state: string; sampled_at: string }; history: { start: string; tx_bytes: string; rx_bytes: string }[]; observations: ExitEvidence[]; dropped_attempt_records: number }
export type TrafficRange = '24h' | '7d' | '30d' | '90d' | 'custom'
export type TrafficInterval = 'minute' | 'hour'
// 1 秒平均峰值：at 为采样结束时间；缺少某个键表示未知，只有有效样本的 0 才代表观察到零速率。
export interface TrafficPeak { bytes_per_second: number; at: string; window_seconds: number }
export interface TrafficSampleQuality { first_sample_at: string | null; last_sample_at: string | null; valid_samples: number; invalid_samples: number }
export type WGPeakKey = 'tx' | 'rx' | 'total'
// 升级前的 WG 明细为 null，不能当作 0 速率展示。
export interface WGPeakHistory { peaks: Partial<Record<WGPeakKey, TrafficPeak>> | null; quality: TrafficSampleQuality }
export interface TrafficBucket { start: string; tx_bytes: string; rx_bytes: string; peak_history?: WGPeakHistory | null }
export interface TrafficReport { range: TrafficRange; interval: TrafficInterval; start: string; end: string; requested_start: string; requested_end: string; retained_from: string; as_of: string; end_clamped: boolean; tx_bytes: string; rx_bytes: string; total_bytes: string; has_records: boolean; bucket_count: number; first_record_at?: string; last_record_at?: string; saved_at?: string; incomplete: boolean; metric_basis: string; history: TrafficBucket[]; peak_window_seconds?: number; peak_history?: WGPeakHistory | null }
export interface TrafficSelection { range: TrafficRange; start?: string; end?: string }
export interface TrafficParams extends TrafficSelection { interval?: TrafficInterval }
export interface EgressAttempt { attempt_id: string; account_id: number; started_at: string; finished_at: string; outcome: string; snapshot: { route: string; node_id?: number; config_version?: number; runtime_id?: string; family: string; connection_reused: boolean; evidence: ExitEvidence } }
export interface EgressQuery { request_id: string; account_id: number }
export interface EgressSummary extends EgressQuery { ips: string[]; recorded: boolean; unobserved: boolean; expired: boolean }
export async function exitSummaries(requests: EgressQuery[], signal?: AbortSignal) { return (await apiClient.post<EgressSummary[]>('/admin/proxies/egress-summaries', { requests }, { signal })).data }
export interface WGInput { name: string; config: string; config_version?: number; expires_at: string | null }
export async function capabilities() { return (await apiClient.get<{ key_configured: boolean }>('/admin/proxies/wireguard/capabilities')).data }
export async function details(id: number, interval: string, signal?: AbortSignal) { return (await apiClient.get<WGDetails>(`/admin/proxies/${id}/wireguard`, { params: { interval }, signal })).data }
export async function traffic(id: number, params: TrafficParams, signal?: AbortSignal) { return (await apiClient.get<TrafficReport>(`/admin/proxies/${id}/wireguard/traffic`, { params, signal })).data }
export async function save(id: number | null, input: WGInput) { return (id ? await apiClient.put<WGNode>(`/admin/proxies/${id}/wireguard`, input) : await apiClient.post<WGNode>('/admin/proxies/wireguard', input)).data }
export async function setPaused(id: number, version: number, paused: boolean) { await apiClient.post(`/admin/proxies/${id}/wireguard/state`, { config_version: version, paused }) }
export async function probe(id: number, family: string) { return (await apiClient.post<ExitEvidence>(`/admin/proxies/${id}/wireguard/probe`, null, { params: { family }, timeout: 30000 })).data }
export async function attempts(requestID: string) { return (await apiClient.get<EgressAttempt[]>('/admin/proxies/egress-attempts', { params: { request_id: requestID } })).data }
