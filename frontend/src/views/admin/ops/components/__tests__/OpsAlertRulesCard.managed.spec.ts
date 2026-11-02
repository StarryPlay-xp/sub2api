import { afterEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { ref } from 'vue'

vi.mock('vue-i18n', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-i18n')>()),
  useI18n: () => ({ t: (key: string) => key })
}))
enableAutoUnmount(afterEach)

import OpsAlertRulesCard from '../OpsAlertRulesCard.vue'

const { listAlertRules } = vi.hoisted(() => ({ listAlertRules: vi.fn() }))
vi.mock('@vueuse/core', () => ({ useMediaQuery: () => ref(true) }))
vi.mock('@/api', () => ({ adminAPI: { groups: { getAll: vi.fn().mockResolvedValue([]) } } }))
vi.mock('@/api/admin/ops', () => ({ opsAPI: { listAlertRules } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError: vi.fn(), showSuccess: vi.fn() }) }))

const base = {
  enabled: true,
  operator: '>=' as const,
  window_minutes: 1,
  sustained_minutes: 1,
  severity: 'P1',
  cooldown_minutes: 60,
  notify_email: true
}

describe('managed alert rules', () => {
  it('shows VPS plan rules read-only with a link to the plan instead of edit and delete', async () => {
    listAlertRules.mockResolvedValue([
      { ...base, id: 2, name: 'host_traffic:usage_warning', description: 'Managed by the local host traffic plan', metric_type: 'host_traffic_quota_used_percent', threshold: 80, managed_by: 'host_traffic:usage_warning' },
      { ...base, id: 1, name: 'Error rate', metric_type: 'error_rate', threshold: 5 }
    ])
    const wrapper = mount(OpsAlertRulesCard, {
      global: {
        stubs: {
          BaseDialog: true,
          ConfirmDialog: true,
          Select: true,
          RouterLink: { props: ['to'], template: '<a :href="to" v-bind="$attrs"><slot /></a>' }
        }
      }
    })
    await flushPromises()

    const rows = wrapper.findAll('tbody tr')
    expect(rows[0].text()).toContain('admin.ops.alertRules.managed.hostTraffic.usage_warning')
    expect(rows[0].text()).toContain('admin.ops.alertRules.managed.description')
    expect(rows[0].text()).not.toContain('common.edit')
    expect(rows[0].get('[data-testid="managed-rule-link"]').attributes('href')).toBe('/admin/traffic')

    expect(rows[1].text()).toContain('Error rate')
    expect(rows[1].text()).toContain('common.edit')
    expect(rows[1].text()).toContain('common.delete')
  })
})
