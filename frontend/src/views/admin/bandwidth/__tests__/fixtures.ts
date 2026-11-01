import type {
  ServiceTrafficCurrent,
  ServiceTrafficReport,
  ServiceTrafficResponse
} from '@/api/admin/ops'
import type { TrafficReport, WGDetails } from '@/api/admin/wireguard'

export const PEAK_AT = '2026-11-01T02:03:04Z'

export const peak = (bytesPerSecond: number, at = PEAK_AT) => ({
  bytes_per_second: bytesPerSecond,
  at,
  window_seconds: 1.002
})

export function hostReport(overrides: Partial<ServiceTrafficReport> = {}): ServiceTrafficReport {
  return {
    range: '24h',
    interval: 'minute',
    start: '2026-10-31T03:00:00Z',
    end: '2026-11-01T03:00:00Z',
    requested_start: '2026-10-31T02:59:30Z',
    requested_end: '2026-11-01T03:00:00Z',
    retained_from: '2026-10-25T03:00:00Z',
    as_of: '2026-11-01T03:00:00Z',
    end_clamped: false,
    metric_basis: 'service_payload_v1',
    scope: 'single_instance_history',
    peak_window_seconds: 1,
    bytes: { client_in: '1024', client_out: '536870912', upstream_out: '2048', upstream_in: '1073741824' },
    in_bytes: '1073742848',
    out_bytes: '536872960',
    total_bytes: '1610615808',
    peaks: {
      client_in: peak(512),
      client_out: peak(1048576),
      upstream_out: peak(1024),
      upstream_in: peak(2097152),
      // 入站 2097664 B/s ≈ 16.78 Mbps，出站 1049600 B/s ≈ 8.4 Mbps
      in: peak(2097664),
      out: peak(1049600, '2026-11-01T01:00:00Z'),
      total: peak(3147264)
    },
    quality: {
      first_sample_at: '2026-10-31T03:00:01Z',
      last_sample_at: '2026-11-01T02:59:59Z',
      valid_samples: 86398,
      invalid_samples: 0
    },
    history: [
      {
        start: '2026-11-01T02:03:00Z',
        interval: 'minute',
        bytes: { client_in: '100', client_out: '2000', upstream_out: '110', upstream_in: '2100' },
        peaks: { in: peak(2097664), out: peak(1049600, '2026-11-01T02:03:30Z') },
        quality: { first_sample_at: null, last_sample_at: null, valid_samples: 60, invalid_samples: 0 }
      }
    ],
    has_records: true,
    incomplete: false,
    concurrent_runtimes: false,
    saved_at: '2026-11-01T02:59:58Z',
    ...overrides
  }
}

export function hostCurrent(overrides: Partial<ServiceTrafficCurrent> = {}): ServiceTrafficCurrent {
  return {
    bytes_per_second: { client_in: 100, client_out: 2048, upstream_out: 200, upstream_in: 4096, in: 4196, out: 2248, total: 6444 },
    sampled_at: '2026-11-01T02:59:59Z',
    window_seconds: 1.001,
    state: 'sampled',
    ...overrides
  }
}

export function hostResponse(
  reportOverrides: Partial<ServiceTrafficReport> = {},
  currentOverrides: Partial<ServiceTrafficCurrent> = {},
  extra: Partial<ServiceTrafficResponse> = {}
): ServiceTrafficResponse {
  return {
    report: hostReport(reportOverrides),
    current: hostCurrent(currentOverrides),
    last_persisted_at: '2026-11-01T02:59:58Z',
    persistence_error: false,
    ...extra
  }
}

export function wgReport(overrides: Partial<TrafficReport> = {}): TrafficReport {
  return {
    range: '24h',
    interval: 'minute',
    start: '2026-10-31T03:00:00Z',
    end: '2026-11-01T03:00:00Z',
    requested_start: '2026-10-31T03:00:00Z',
    requested_end: '2026-11-01T03:00:00Z',
    retained_from: '2026-10-25T03:00:00Z',
    as_of: '2026-11-01T03:00:00Z',
    end_clamped: false,
    tx_bytes: '536870912',
    rx_bytes: '1073741824',
    total_bytes: '1610612736',
    has_records: true,
    bucket_count: 1,
    incomplete: false,
    metric_basis: 'wg_udp_payload_v1',
    history: [{ start: '2026-11-01T02:00:00Z', tx_bytes: '1', rx_bytes: '2' }],
    peak_window_seconds: 1,
    peak_history: {
      // 合计 4194304 B/s ≈ 33.55 Mbps
      peaks: { tx: peak(2658525), rx: peak(2308377), total: peak(4194304, '2026-11-01T02:05:00Z') },
      quality: { first_sample_at: '2026-10-31T03:00:01Z', last_sample_at: '2026-11-01T02:59:59Z', valid_samples: 100, invalid_samples: 0 }
    },
    ...overrides
  }
}

export function wgDetails(state = 'sampled', overrides: Partial<WGDetails['node']> = {}): WGDetails {
  return {
    node: {
      id: 11,
      name: 'Tokyo relay',
      config_version: 2,
      active: true,
      paused: false,
      tx_bytes: '0',
      rx_bytes: '0',
      incomplete: false,
      metric_basis: 'wg_udp_payload_v1',
      ...overrides
    },
    rate: { tx_bytes_per_second: 125_000, rx_bytes_per_second: 250_000, window_seconds: 1, state, sampled_at: '2026-11-01T02:59:59Z' },
    history: [],
    observations: [],
    dropped_attempt_records: 0
  }
}

export function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}
