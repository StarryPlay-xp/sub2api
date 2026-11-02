import { afterEach, describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import HelpTooltip from '@/components/common/HelpTooltip.vue'

function getTooltipElement(): HTMLDivElement {
  const tooltip = document.body.querySelector('[role="tooltip"]')
  if (!(tooltip instanceof HTMLDivElement)) {
    throw new Error('tooltip element not found')
  }
  return tooltip
}

describe('HelpTooltip', () => {
  afterEach(() => {
    document.body.innerHTML = ''
  })

  it('keeps the existing hover interaction by default', async () => {
    const wrapper = mount(HelpTooltip, {
      attachTo: document.body,
      props: {
        content: 'hover details',
      },
    })

    const trigger = wrapper.get('.group')
    const tooltip = getTooltipElement()

    expect(tooltip.style.display).toBe('none')

    await trigger.trigger('mouseenter')
    await nextTick()
    expect(tooltip.style.display).not.toBe('none')

    await trigger.trigger('mouseleave')
    await nextTick()
    expect(tooltip.style.display).toBe('none')

    wrapper.unmount()
  })

  it('keeps a hover tooltip open while the pointer moves between the trigger and the tooltip', async () => {
    const wrapper = mount(HelpTooltip, {
      attachTo: document.body,
      props: {
        content: 'copyable details',
      },
    })

    const trigger = wrapper.get('.group')
    const tooltip = getTooltipElement()

    await trigger.trigger('mouseenter')
    await nextTick()
    expect(tooltip.style.display).not.toBe('none')

    await trigger.trigger('mouseleave', { relatedTarget: tooltip })
    await nextTick()
    expect(tooltip.style.display).not.toBe('none')

    tooltip.dispatchEvent(new MouseEvent('mouseleave', { relatedTarget: trigger.element }))
    await nextTick()
    expect(tooltip.style.display).not.toBe('none')

    tooltip.dispatchEvent(new MouseEvent('mouseleave', { relatedTarget: null }))
    await nextTick()
    expect(tooltip.style.display).toBe('none')

    wrapper.unmount()
  })

  it('supports click-to-toggle details and closes on outside click', async () => {
    const wrapper = mount(HelpTooltip, {
      attachTo: document.body,
      props: {
        content: 'click details',
        trigger: 'click',
      },
    })

    const trigger = wrapper.get('.group')
    const tooltip = getTooltipElement()

    expect(tooltip.style.display).toBe('none')

    await trigger.trigger('click')
    await nextTick()
    expect(tooltip.style.display).not.toBe('none')
    expect(tooltip.textContent).toContain('click details')

    const closeButton = tooltip.querySelector('button[aria-label="Close"]')
    if (!(closeButton instanceof HTMLButtonElement)) {
      throw new Error('close button not found')
    }
    closeButton.click()
    await nextTick()
    expect(tooltip.style.display).toBe('none')

    await trigger.trigger('click')
    await nextTick()
    expect(tooltip.style.display).not.toBe('none')

    document.body.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await nextTick()
    expect(tooltip.style.display).toBe('none')

    wrapper.unmount()
  })

  it('positions against the viewport even when the page is scrolled', async () => {
    const wrapper = mount(HelpTooltip, {
      attachTo: document.body,
      props: { content: 'scrolled details' },
    })
    const trigger = wrapper.get('.group')
    trigger.element.getBoundingClientRect = () =>
      ({ top: 300, bottom: 316, left: 100, right: 116, width: 16, height: 16, x: 100, y: 300, toJSON: () => ({}) }) as DOMRect
    Object.defineProperty(window, 'scrollY', { configurable: true, value: 2000 })

    await trigger.trigger('mouseenter')
    await nextTick()
    const tooltip = getTooltipElement()
    // fixed 定位：顶部为图标顶部减去间距，不叠加页面滚动距离。
    expect(tooltip.style.top).toBe('292px')
    expect(tooltip.style.left).toBe('108px')
    expect(tooltip.dataset.placement).toBe('top')

    Object.defineProperty(window, 'scrollY', { configurable: true, value: 0 })
    wrapper.unmount()
  })

  it('flips below the trigger when there is no room above', async () => {
    const wrapper = mount(HelpTooltip, {
      attachTo: document.body,
      props: { content: 'top edge details' },
    })
    const trigger = wrapper.get('.group')
    trigger.element.getBoundingClientRect = () =>
      ({ top: 4, bottom: 20, left: 100, right: 116, width: 16, height: 16, x: 100, y: 4, toJSON: () => ({}) }) as DOMRect
    const tooltip = getTooltipElement()
    Object.defineProperty(tooltip, 'offsetHeight', { configurable: true, value: 60 })

    await trigger.trigger('mouseenter')
    await nextTick()
    expect(tooltip.dataset.placement).toBe('bottom')
    expect(tooltip.style.top).toBe('28px')
    wrapper.unmount()
  })
})
