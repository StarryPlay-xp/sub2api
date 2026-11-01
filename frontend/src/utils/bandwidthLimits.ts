const MAX_MBPS = 1_000_000

export function normalizeLimit(value: unknown): number | null {
  const number = typeof value === 'number' ? value : Number(value)
  return Number.isFinite(number) && number > 0 && number <= MAX_MBPS ? number : null
}
