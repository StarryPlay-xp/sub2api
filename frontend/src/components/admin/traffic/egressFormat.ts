// 出口与上游账号的展示换算：直连没有名称，已硬删除的代理只剩编号。
import type { EgressAccountStats, EgressRouteIdentity, EgressRouteStats, EgressRouteType } from '@/api/admin/trafficEgress'
import { parseSignedBytes } from './subjectFormat'

type Translate = (key: string, params?: Record<string, unknown>) => string

export function routeKey(route: EgressRouteIdentity): string {
  return `${route.route_type}:${route.route_id ?? 0}`
}

export function routeKindLabel(type: EgressRouteType, t: Translate): string {
  return t(`traffic.egress.kind.${type}`)
}

export function routeTitle(route: EgressRouteIdentity, t: Translate): string {
  if (route.route_type === 'direct') return t('traffic.egress.kind.direct')
  return route.route_name?.trim() || `#${route.route_id ?? '—'}`
}

export function accountTitle(account: EgressAccountStats): string {
  return account.account_name?.trim() || `#${account.account_id}`
}

/**
 * 隧道与服务载荷差值 = 同一小时窗口的 WG 隧道字节 − 经此节点的服务上游字节，保留正负号。
 * 两者口径不同（加密 UDP 载荷 vs 应用正文，后者可能是解压后的字节），差值不是精确的封装开销。
 */
export function tunnelDifference(route: EgressRouteStats | null | undefined): bigint | null {
  const wireguard = route?.wireguard
  if (!wireguard?.has_records) return null
  const tunnel = parseSignedBytes(wireguard.total_bytes)
  const service = parseSignedBytes(route?.total_bytes)
  return tunnel === null || service === null ? null : tunnel - service
}
