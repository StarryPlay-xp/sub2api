type Translate = (key: string, params?: Record<string, unknown>) => string

interface RequestError {
  status?: number
  code?: string | number
  reason?: string
}

export function errorStatus(error: unknown): number | undefined {
  return (error as RequestError | null)?.status
}

/** 权限不足、监控关闭或后端尚未提供接口都不会自行恢复，应停止轮询直到手动刷新。 */
export function isBlockingError(error: unknown): boolean {
  const status = errorStatus(error)
  return status === 403 || status === 404
}

export function subjectErrorMessage(error: unknown, t: Translate): string {
  const value = (error ?? {}) as RequestError
  switch (value.reason) {
    case 'TRAFFIC_RANGE_INVALID':
      return t('bandwidth.errors.rangeInvalid')
    case 'TRAFFIC_INTERVAL_INVALID':
      return t('bandwidth.errors.intervalInvalid')
    case 'TRAFFIC_DATE_INVALID':
      return t('bandwidth.errors.dateInvalid')
    case 'TRAFFIC_BOUNDARY_UNALIGNED':
      return t('bandwidth.errors.boundaryUnaligned')
    case 'TRAFFIC_RETENTION_EXCEEDED':
      return t('traffic.errors.retentionExceeded')
    case 'TRAFFIC_SUBJECT_NOT_FOUND':
      return t('traffic.errors.notFound')
    case 'TRAFFIC_SUBJECT_INVALID':
    case 'TRAFFIC_SORT_INVALID':
    case 'TRAFFIC_COMPARE_REQUIRED':
    case 'TRAFFIC_PAGE_INVALID':
    case 'TRAFFIC_FILTER_INVALID':
      return t('traffic.errors.invalidQuery')
  }
  if (value.status === 403) return t('traffic.errors.forbidden')
  if (value.status === 404) {
    // 监控关闭有明确的错误码；其余 404 多为后端版本尚未提供分维度接口。
    return value.code === 'OPS_DISABLED' ? t('traffic.errors.disabled') : t('traffic.errors.unsupported')
  }
  if (value.status === 503) return t('traffic.errors.unavailable')
  return t('traffic.errors.load')
}
