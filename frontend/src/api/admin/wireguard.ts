import { apiClient } from '../client'
export interface ExitEvidence { ip?: string; source: string; family: string; observed_at: string; expires_at: string; route: string; node_id?: number; version?: number; runtime_id?: string; state: string }
export interface WGNode { id: number; name: string; config_version: number; active: boolean; paused: boolean; pause_reason?: string; expires_at?: string; tx_bytes: string; rx_bytes: string; saved_at?: string; incomplete: boolean; metric_basis: string }
export interface WGDetails { node: WGNode; rate: { tx_bytes_per_second: number; rx_bytes_per_second: number; state: string; sampled_at: string }; history: { start: string; tx_bytes: string; rx_bytes: string }[]; observations: ExitEvidence[]; dropped_attempt_records: number }
export interface EgressAttempt { attempt_id: string; account_id: number; started_at: string; finished_at: string; outcome: string; snapshot: { route: string; node_id?: number; config_version?: number; runtime_id?: string; family: string; connection_reused: boolean; evidence: ExitEvidence } }
export interface WGInput { name: string; config: string; config_version?: number; expires_at: string | null }
export async function capabilities() { return (await apiClient.get<{ key_configured: boolean }>('/admin/proxies/wireguard/capabilities')).data }
export async function details(id: number, interval: string, signal?: AbortSignal) { return (await apiClient.get<WGDetails>(`/admin/proxies/${id}/wireguard`, { params: { interval }, signal })).data }
export async function save(id: number | null, input: WGInput) { return (id ? await apiClient.put<WGNode>(`/admin/proxies/${id}/wireguard`, input) : await apiClient.post<WGNode>('/admin/proxies/wireguard', input)).data }
export async function setPaused(id: number, version: number, paused: boolean) { await apiClient.post(`/admin/proxies/${id}/wireguard/state`, { config_version: version, paused }) }
export async function probe(id: number, family: string) { return (await apiClient.post<ExitEvidence>(`/admin/proxies/${id}/wireguard/probe`, null, { params: { family }, timeout: 30000 })).data }
export async function attempts(requestID: string) { return (await apiClient.get<EgressAttempt[]>('/admin/proxies/egress-attempts', { params: { request_id: requestID } })).data }
