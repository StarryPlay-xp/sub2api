// 按出口接口约定构造的模拟数据：单元测试和本地预览共用。
import type {
  EgressAccountStats,
  EgressRouteStats,
  EgressRouteType,
  EgressWindow,
  TrafficEgressData,
  TrafficEgressDetailData,
  UpstreamBytes
} from '@/api/admin/trafficEgress'
import { AS_OF, subjectPeak } from './fixtures'

export function upstreamBytes(out: number, inbound: number): UpstreamBytes {
  return { upstream_out: String(out), upstream_in: String(inbound) }
}

export function egressWindow(overrides: Partial<EgressWindow> = {}): EgressWindow {
  return {
    range: '24h',
    interval: 'hour',
    start: '2026-10-31T03:00:00Z',
    end: AS_OF,
    requested_start: '2026-10-31T02:59:30Z',
    requested_end: AS_OF,
    retained_from: '2026-08-03T03:00:00Z',
    as_of: AS_OF,
    end_clamped: false,
    metric_basis: 'service_payload_v1',
    egress_started_at: '2026-10-20T00:00:00Z',
    has_records: true,
    incomplete: false,
    concurrent_runtimes: false,
    overflow: false,
    ...overrides
  }
}

/** 默认本站→上游 128 MiB、上游→本站 896 MiB，合计 1 GiB。 */
export function egressRoute(
  type: EgressRouteType,
  id: number | null,
  overrides: Partial<EgressRouteStats> = {}
): EgressRouteStats {
  const bytes = overrides.bytes ?? upstreamBytes(134_217_728, 939_524_096)
  return {
    route_type: type,
    route_id: type === 'direct' ? null : id,
    route_name: type === 'direct' ? null : `${type}-${id}`,
    protocol: type === 'direct' ? null : type === 'wireguard' ? 'wireguard' : 'http',
    route_deleted: false,
    bytes,
    total_bytes: String(BigInt(bytes.upstream_out) + BigInt(bytes.upstream_in)),
    share: 0.5,
    attempts: 240,
    peaks: { out: subjectPeak(400_000), in: subjectPeak(1_600_000), total: subjectPeak(1_900_000) },
    account_count: 2,
    wireguard:
      type === 'wireguard'
        ? { tx_bytes: '201326592', rx_bytes: '1006632960', total_bytes: '1207959552', has_records: true }
        : null,
    previous: null,
    growth_bytes: null,
    ...overrides
  }
}

export function egressReport(overrides: Partial<TrafficEgressData> = {}): TrafficEgressData {
  const routes = overrides.routes ?? [
    egressRoute('wireguard', 11, { route_name: 'Tokyo relay', share: 0.5 }),
    egressRoute('direct', null, { share: 0.25, bytes: upstreamBytes(67_108_864, 469_762_048) }),
    egressRoute('proxy', 21, { route_name: 'HK proxy', route_deleted: true, share: 0.125, bytes: upstreamBytes(33_554_432, 234_881_024) })
  ]
  return {
    window: egressWindow(),
    previous_window: null,
    routes,
    totals: {
      upstream: { bytes: upstreamBytes(268_435_456, 1_879_048_192), total_bytes: '2147483648' },
      unidentified: { bytes: upstreamBytes(33_554_432, 234_881_024), total_bytes: '268435456' }
    },
    ...overrides
  }
}

export function egressAccount(id: number, overrides: Partial<EgressAccountStats> = {}): EgressAccountStats {
  const bytes = overrides.bytes ?? upstreamBytes(100_663_296, 704_643_072)
  return {
    account_id: id,
    account_name: `account-${id}`,
    platform: 'anthropic',
    account_type: 'oauth',
    account_deleted: false,
    bytes,
    total_bytes: String(BigInt(bytes.upstream_out) + BigInt(bytes.upstream_in)),
    share: 0.75,
    attempts: 180,
    peaks: { out: subjectPeak(300_000), in: subjectPeak(1_200_000), total: subjectPeak(1_400_000) },
    previous: null,
    growth_bytes: null,
    ...overrides
  }
}

export function egressDetail(
  summary: EgressRouteStats = egressRoute('wireguard', 11, { route_name: 'Tokyo relay' }),
  overrides: Partial<TrafficEgressDetailData> = {}
): TrafficEgressDetailData {
  return {
    window: egressWindow(),
    previous_window: { start: '2026-10-30T03:00:00Z', end: '2026-10-31T03:00:00Z', complete: true },
    summary: { ...summary, previous: { total_bytes: '805306368' }, growth_bytes: '268435456' },
    history: [
      {
        start: '2026-11-01T01:00:00Z',
        interval: 'hour',
        bytes: upstreamBytes(10_000_000, 70_000_000),
        attempts: 20,
        peaks: { out: subjectPeak(200_000), in: subjectPeak(900_000), total: subjectPeak(1_000_000) }
      },
      { start: '2026-11-01T02:00:00Z', interval: 'hour', bytes: upstreamBytes(5_000_000, 30_000_000), attempts: 8, peaks: null }
    ],
    accounts: [
      egressAccount(31),
      egressAccount(32, {
        account_name: 'Legacy account',
        account_deleted: true,
        bytes: upstreamBytes(16_777_216, 117_440_512),
        share: 0.125,
        attempts: 40,
        peaks: null
      })
    ],
    unattributed_account: { bytes: upstreamBytes(16_777_216, 117_440_512), total_bytes: '134217728', attempts: 20 },
    ...overrides
  }
}
