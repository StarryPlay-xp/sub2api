import { onMounted, onUnmounted, ref, watch, type Ref } from 'vue'
import { useDocumentVisibility } from '@vueuse/core'
import { useI18n } from 'vue-i18n'
import {
  getTrafficRanking,
  type TrafficRankingData,
  type TrafficRankingSort,
  type TrafficSortOrder,
  type TrafficSubjectType
} from '@/api/admin/trafficSubjects'
import { isBlockingError, subjectErrorMessage } from './subjectErrors'
import type { PresetRange } from './usePresetRange'

// 小时汇总约 5 秒保存一次，排行按 30 秒刷新已足够；页面隐藏时不刷新。
export const RANKING_POLL_MS = 30_000
export const RANKING_PAGE_SIZE = 20

export interface SubjectRankingOptions {
  range: Ref<PresetRange>
  subject?: TrafficSubjectType
  sort: TrafficRankingSort
  // 只看某个用户名下的 Key（用户详情内使用）。
  userId?: Ref<number | null>
  pageSize?: number
  poll?: boolean
}

export function useSubjectRanking(options: SubjectRankingOptions) {
  const { t } = useI18n()
  const defaultSort = options.sort
  const pageSize = options.pageSize ?? RANKING_PAGE_SIZE

  const subject = ref<TrafficSubjectType>(options.userId ? 'api_key' : (options.subject ?? 'user'))
  const sort = ref<TrafficRankingSort>(defaultSort)
  const order = ref<TrafficSortOrder>('desc')
  const page = ref(1)
  const query = ref('')
  const compare = ref(false)

  const data = ref<TrafficRankingData | null>(null)
  const loading = ref(false)
  const error = ref('')
  const blocked = ref(false)
  const visibility = useDocumentVisibility()

  let seq = 0
  let controller: AbortController | null = null
  let timer: ReturnType<typeof setTimeout> | undefined
  let disposed = false
  // 条件变化把页码重置为 1 时由条件监听统一加载，页码监听跳过这一次。
  let pageReset = false

  function schedule() {
    if (timer) clearTimeout(timer)
    timer = undefined
    if (disposed || blocked.value || options.poll === false) return
    timer = setTimeout(() => {
      if (visibility.value === 'hidden') schedule()
      else void load(false)
    }, RANKING_POLL_MS)
  }

  async function load(reset: boolean) {
    if (disposed) return
    const userId = options.userId?.value ?? null
    if (options.userId && !userId) return
    const current = ++seq
    controller?.abort()
    const request = new AbortController()
    controller = request
    // 条件变化时先清空，避免沿用上一组条件的排名。
    if (reset) {
      data.value = null
      error.value = ''
    }
    loading.value = true
    try {
      const result = await getTrafficRanking(
        {
          range: options.range.value,
          subject: subject.value,
          sort: sort.value,
          order: order.value,
          page: page.value,
          page_size: pageSize,
          q: query.value,
          user_id: userId ?? undefined,
          compare: compare.value
        },
        { signal: request.signal }
      )
      if (current !== seq || disposed) return
      data.value = result
      error.value = ''
      blocked.value = false
    } catch (e) {
      if (current !== seq || disposed || request.signal.aborted) return
      data.value = null
      error.value = subjectErrorMessage(e, t)
      blocked.value = isBlockingError(e)
    } finally {
      if (current === seq && !disposed) {
        loading.value = false
        schedule()
      }
    }
  }

  function reload() {
    blocked.value = false
    void load(true)
  }

  /** 点击同一列切换升降序；换列时从高到低排。按环比排序需要先开启环比。 */
  function setSort(key: TrafficRankingSort) {
    if (sort.value === key) {
      order.value = order.value === 'desc' ? 'asc' : 'desc'
    } else {
      sort.value = key
      order.value = 'desc'
    }
    if (key === 'growth') compare.value = true
  }

  function refresh() {
    blocked.value = false
    void load(false)
  }

  watch([options.range, subject, sort, order, query, compare, ...(options.userId ? [options.userId] : [])], () => {
    // 关闭环比后不能继续按环比排序，回到默认排序。
    if (!compare.value && sort.value === 'growth') {
      sort.value = defaultSort
      order.value = 'desc'
      return
    }
    pageReset = page.value !== 1
    page.value = 1
    reload()
  })

  watch(page, () => {
    if (pageReset) {
      pageReset = false
      return
    }
    void load(false)
  })

  watch(visibility, (value) => {
    if (value === 'visible' && !blocked.value && !loading.value && data.value) void load(false)
  })

  onMounted(() => {
    void load(true)
  })

  onUnmounted(() => {
    disposed = true
    if (timer) clearTimeout(timer)
    controller?.abort()
  })

  return { subject, sort, order, page, pageSize, query, compare, data, loading, error, blocked, setSort, refresh }
}
