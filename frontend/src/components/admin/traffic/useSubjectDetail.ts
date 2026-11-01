import { onUnmounted, ref, watch, type Ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  getTrafficSubject,
  type TrafficSubjectDetailData,
  type TrafficSubjectIdentity
} from '@/api/admin/trafficSubjects'
import { subjectErrorMessage } from './subjectErrors'
import type { PresetRange } from './usePresetRange'

/** 单个用户或 Key 的期间汇总与小时趋势，默认带上一时段用于环比。 */
export function useSubjectDetail(subject: Ref<TrafficSubjectIdentity | null>, range: Ref<PresetRange>) {
  const { t } = useI18n()
  const data = ref<TrafficSubjectDetailData | null>(null)
  const loading = ref(false)
  const error = ref('')

  let seq = 0
  let controller: AbortController | null = null
  let disposed = false

  async function load() {
    const target = subject.value
    const current = ++seq
    controller?.abort()
    data.value = null
    error.value = ''
    if (!target || disposed) {
      loading.value = false
      return
    }
    const id = target.subject_type === 'user' ? target.user_id : target.api_key_id
    if (!id) {
      error.value = t('traffic.errors.notFound')
      return
    }
    const request = new AbortController()
    controller = request
    loading.value = true
    try {
      const result = await getTrafficSubject(
        target.subject_type,
        id,
        { range: range.value, compare: true },
        { signal: request.signal }
      )
      if (current !== seq || disposed) return
      data.value = result
    } catch (e) {
      if (current !== seq || disposed || request.signal.aborted) return
      error.value = subjectErrorMessage(e, t)
    } finally {
      if (current === seq && !disposed) loading.value = false
    }
  }

  watch([subject, range], () => void load(), { immediate: true })

  onUnmounted(() => {
    disposed = true
    controller?.abort()
  })

  return { data, loading, error, reload: load }
}
