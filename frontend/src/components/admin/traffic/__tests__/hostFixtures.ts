// 按本机流量套餐接口约定构造的模拟数据：单元测试和本地预览共用，后端交付前用于开发套餐卡、表单与详情。
import type {
  HostInterface,
  HostTrafficDetail,
  HostTrafficPeriod,
  HostTrafficPlan,
  HostTrafficStatus,
  HostTrafficUsage,
  HostUsageRange
} from '@/api/admin/hostTraffic'

export const HOST_AS_OF = '2026-11-02T10:00:00Z'
const GB = 1_000_000_000

const bytes = (gb: number) => String(Math.round(gb * GB))
const ago = (now: string, seconds: number) => new Date(Date.parse(now) - seconds * 1000).toISOString()

export function hostPlan(overrides: Partial<HostTrafficPlan> = {}): HostTrafficPlan {
  return {
    version: 'v1',
    quota_value: '1.5',
    quota_unit: 'TB',
    quota_bytes: bytes(1500),
    billing_mode: 'both',
    reset_day: 24,
    reset_time: '00:00',
    timezone: 'UTC',
    unit_base: 1000,
    interfaces: [],
    alerts: { enabled: true, warning_percent: 80, critical_percent: 95, projection: true },
    updated_at: '2026-11-02T02:10:00Z',
    ...overrides
  }
}

export function hostInterfaces(): HostInterface[] {
  return [
    {
      name: 'eth0',
      selected: true,
      physical: true,
      default_route: { ipv4: true, ipv6: true },
      rx_bytes_per_second: 412_000,
      tx_bytes_per_second: 1_380_000
    }
  ]
}

export function hostPeriod(overrides: Partial<HostTrafficPeriod> = {}): HostTrafficPeriod {
  return {
    start: '2026-10-24T00:00:00Z',
    end: '2026-11-24T00:00:00Z',
    observed_from: '2026-10-24T00:00:00Z',
    coverage: 'complete',
    bound: 'exact',
    rx_bytes: bytes(41.2),
    tx_bytes: bytes(61.72),
    measured_billed_bytes: bytes(102.92),
    adjustment_bytes: null,
    adjustment_kind: null,
    billed_bytes: bytes(102.92),
    quota_bytes: bytes(1500),
    remaining_bytes: bytes(1397.08),
    overage_bytes: null,
    used_ratio: 0.0686,
    gaps: { lost_seconds: 0, minor_restarts: 1, unallocated_bytes: '0' },
    projection: { billed_bytes: bytes(342.6), ratio: 0.2284, basis_days: 7 },
    last_reconcile: null,
    ...overrides
  }
}

export function hostStatus(overrides: Partial<HostTrafficStatus> = {}, now = HOST_AS_OF): HostTrafficStatus {
  return {
    as_of: now,
    collection: {
      state: 'ok',
      deployment: 'docker',
      source: 'host_mount',
      started_at: '2026-10-20T08:00:00Z',
      last_sample_at: ago(now, 30),
      host_boot_at: '2026-09-12T03:21:00Z',
      error_reason: null
    },
    interfaces: hostInterfaces(),
    interface_selection: { mode: 'auto', ambiguous: false },
    since_start: { from: '2026-10-20T08:00:00Z', rx_bytes: bytes(52.4), tx_bytes: bytes(80.1), total_bytes: bytes(132.5), bound: 'exact' },
    plan: hostPlan(),
    period: hostPeriod(),
    alerts: { email_enabled: true, firing: [] },
    unit_hint: null,
    ...overrides
  }
}

export type HostScenario =
  | 'complete'
  | 'filled'
  | 'partial'
  | 'warning'
  | 'over'
  | 'unitHint'
  | 'stale'
  | 'noPlan'
  | 'starting'
  | 'ambiguous'
  | 'needsMount'
  | 'unsupported'

export const HOST_SCENARIOS: HostScenario[] = [
  'complete',
  'filled',
  'partial',
  'warning',
  'over',
  'unitHint',
  'stale',
  'noPlan',
  'starting',
  'ambiguous',
  'needsMount',
  'unsupported'
]

/** 各种典型状态；now 用于让 “最近读数” 等相对时间在预览中保持真实。 */
export function hostScenario(name: HostScenario, now = HOST_AS_OF): HostTrafficStatus {
  const base = hostStatus({}, now)
  const joined = '2026-11-02T02:00:00Z'
  switch (name) {
    case 'complete':
      return base
    case 'filled':
      return {
        ...base,
        collection: { ...base.collection, started_at: joined },
        since_start: { from: joined, rx_bytes: bytes(3.1), tx_bytes: bytes(4.72), total_bytes: bytes(7.82), bound: 'exact' },
        period: hostPeriod({
          observed_from: joined,
          coverage: 'partial',
          bound: 'estimate',
          rx_bytes: bytes(3.1),
          tx_bytes: bytes(4.72),
          measured_billed_bytes: bytes(7.82),
          adjustment_bytes: bytes(95.1),
          adjustment_kind: 'fill',
          projection: null,
          gaps: { lost_seconds: 0, minor_restarts: 0, unallocated_bytes: '0' }
        })
      }
    case 'partial':
      return {
        ...base,
        collection: { ...base.collection, started_at: joined },
        since_start: { from: joined, rx_bytes: bytes(3.1), tx_bytes: bytes(4.72), total_bytes: bytes(7.82), bound: 'exact' },
        period: hostPeriod({
          observed_from: joined,
          coverage: 'partial',
          bound: 'lower',
          rx_bytes: bytes(3.1),
          tx_bytes: bytes(4.72),
          measured_billed_bytes: bytes(7.82),
          billed_bytes: bytes(7.82),
          remaining_bytes: bytes(1492.18),
          used_ratio: 0.0052,
          projection: null,
          gaps: { lost_seconds: 0, minor_restarts: 0, unallocated_bytes: '0' }
        })
      }
    case 'warning':
      return {
        ...base,
        period: hostPeriod({
          rx_bytes: bytes(512.4),
          tx_bytes: bytes(748.1),
          measured_billed_bytes: bytes(1260.5),
          billed_bytes: bytes(1260.5),
          remaining_bytes: bytes(239.5),
          used_ratio: 0.8403,
          projection: { billed_bytes: bytes(1622), ratio: 1.0813, basis_days: 7 }
        }),
        alerts: {
          email_enabled: true,
          firing: [
            { level: 'warning', kind: 'usage', since: '2026-11-01T16:20:00Z' },
            { level: 'warning', kind: 'projection', since: '2026-10-31T09:00:00Z' }
          ]
        }
      }
    case 'over':
      return {
        ...base,
        period: hostPeriod({
          rx_bytes: bytes(640.2),
          tx_bytes: bytes(939.8),
          measured_billed_bytes: bytes(1580),
          billed_bytes: bytes(1580),
          remaining_bytes: null,
          overage_bytes: bytes(80),
          used_ratio: 1.0533,
          projection: { billed_bytes: bytes(1960), ratio: 1.3067, basis_days: 7 }
        }),
        alerts: { email_enabled: false, firing: [{ level: 'critical', kind: 'usage', since: '2026-11-02T06:40:00Z' }] }
      }
    case 'unitHint':
      return {
        ...base,
        period: hostPeriod({
          last_reconcile: {
            at: '2026-11-02T09:30:00Z',
            provider_bytes: bytes(95.84),
            measured_bytes: bytes(102.92),
            deviation_ratio: -0.0688
          }
        }),
        unit_hint: { suggested_base: 1024, samples: 3 }
      }
    case 'stale':
      return {
        ...base,
        collection: { ...base.collection, state: 'stale', last_sample_at: ago(now, 25 * 60) },
        interfaces: base.interfaces.map((item) => ({ ...item, rx_bytes_per_second: null, tx_bytes_per_second: null }))
      }
    case 'noPlan':
      return {
        ...base,
        collection: { ...base.collection, started_at: joined },
        since_start: { from: joined, rx_bytes: bytes(3.1), tx_bytes: bytes(4.72), total_bytes: bytes(7.82), bound: 'exact' },
        plan: null,
        period: null
      }
    case 'starting':
      return {
        ...base,
        collection: { ...base.collection, state: 'starting', started_at: null, last_sample_at: null },
        interfaces: base.interfaces.map((item) => ({ ...item, rx_bytes_per_second: null, tx_bytes_per_second: null })),
        since_start: null,
        plan: null,
        period: null
      }
    case 'ambiguous':
      return {
        ...base,
        collection: { ...base.collection, started_at: joined },
        interfaces: [
          { name: 'eth0', selected: true, physical: true, default_route: null, rx_bytes_per_second: 412_000, tx_bytes_per_second: 1_380_000 },
          { name: 'ens5', selected: false, physical: true, default_route: null, rx_bytes_per_second: 2_100, tx_bytes_per_second: 1_900 }
        ],
        interface_selection: { mode: 'auto', ambiguous: true },
        since_start: { from: joined, rx_bytes: bytes(3.1), tx_bytes: bytes(4.72), total_bytes: bytes(7.82), bound: 'exact' },
        plan: null,
        period: null
      }
    case 'needsMount':
      return {
        ...base,
        collection: {
          state: 'needs_host_mount',
          deployment: 'docker',
          source: null,
          started_at: null,
          last_sample_at: null,
          host_boot_at: null,
          error_reason: null
        },
        interfaces: [],
        since_start: null,
        plan: null,
        period: null
      }
    case 'unsupported':
      return {
        ...base,
        collection: {
          state: 'unsupported',
          deployment: 'binary',
          source: null,
          started_at: null,
          last_sample_at: null,
          host_boot_at: null,
          error_reason: 'NOT_LINUX'
        },
        interfaces: [],
        since_start: null,
        plan: null,
        period: null
      }
  }
}

export function hostUsage(range: HostUsageRange = '24h', overrides: Partial<HostTrafficUsage> = {}): HostTrafficUsage {
  const hours = { '24h': 24, '7d': 168, '30d': 720, '90d': 2160 }[range]
  return {
    window: {
      range,
      start: ago(HOST_AS_OF, hours * 3600),
      end: HOST_AS_OF,
      as_of: HOST_AS_OF,
      retained_from: '2025-10-02T00:00:00Z'
    },
    state: 'ok',
    interfaces: ['eth0'],
    has_records: true,
    rx_bytes: bytes(4.6 * (hours / 24)),
    tx_bytes: bytes(6.9 * (hours / 24)),
    total_bytes: bytes(11.5 * (hours / 24)),
    observed_seconds: hours * 3600 - 120,
    expected_seconds: hours * 3600,
    ...overrides
  }
}

export function hostDetail(overrides: Partial<HostTrafficDetail> = {}): HostTrafficDetail {
  const days = Array.from({ length: 10 }, (_, index) => {
    const start = new Date(Date.UTC(2026, 9, 24 + index)).toISOString()
    const end = new Date(Date.UTC(2026, 9, 25 + index)).toISOString()
    const last = index === 9
    const rx = last ? 3.4 : 3.6 + ((index * 7) % 5) * 0.4
    const tx = last ? 5.1 : 5.4 + ((index * 3) % 4) * 0.7
    return {
      date: start.slice(0, 10),
      start,
      end,
      rx_bytes: bytes(rx),
      tx_bytes: bytes(tx),
      billed_bytes: bytes(rx + tx),
      observed_seconds: last ? 36_000 : index === 4 ? 86_280 : 86_400,
      expected_seconds: last ? 36_000 : 86_400
    }
  })
  return {
    days_timezone: 'UTC',
    days,
    periods: [
      {
        start: '2026-09-24T00:00:00Z',
        end: '2026-10-24T00:00:00Z',
        quota_bytes: bytes(1500),
        billing_mode: 'both',
        rx_bytes: bytes(402.1),
        tx_bytes: bytes(611.4),
        billed_bytes: bytes(1013.5),
        adjustment_bytes: null,
        coverage: 'complete',
        bound: 'exact',
        closed_at: '2026-10-24T00:01:00Z'
      },
      {
        start: '2026-08-24T00:00:00Z',
        end: '2026-09-24T00:00:00Z',
        quota_bytes: bytes(1500),
        billing_mode: 'both',
        rx_bytes: bytes(120.3),
        tx_bytes: bytes(180.6),
        billed_bytes: bytes(1120.9),
        adjustment_bytes: bytes(820),
        coverage: 'partial',
        bound: 'estimate',
        closed_at: '2026-09-24T00:01:00Z'
      }
    ],
    calibrations: [
      {
        id: 2,
        at: '2026-10-30T12:00:00Z',
        kind: 'reconcile',
        provider_bytes: bytes(72.4),
        measured_bytes: bytes(71.6),
        deviation_ratio: 0.0112,
        period_start: '2026-10-24T00:00:00Z'
      },
      {
        id: 1,
        at: '2026-09-12T03:30:00Z',
        kind: 'fill',
        provider_bytes: bytes(830),
        measured_bytes: bytes(10),
        deviation_ratio: null,
        period_start: '2026-08-24T00:00:00Z'
      }
    ],
    ...overrides
  }
}
