import { apiClient } from '../client'

export interface BandwidthLimit {
  mbps: number | null
  version: string
}

export interface BandwidthLimits {
  host: BandwidthLimit
  nodes: Record<string, BandwidthLimit>
}

export async function getBandwidthLimits(signal?: AbortSignal): Promise<BandwidthLimits> {
  const { data } = await apiClient.get<BandwidthLimits>('/admin/ops/bandwidth-limits', { signal })
  return data
}

export async function saveBandwidthLimit(
  nodeId: number | null,
  value: BandwidthLimit,
  signal?: AbortSignal
): Promise<BandwidthLimit> {
  const target = nodeId === null ? 'host' : `nodes/${nodeId}`
  const { data } = await apiClient.put<BandwidthLimit>(`/admin/ops/bandwidth-limits/${target}`, value, { signal })
  return data
}
