/* eslint-disable vue/one-component-per-file -- createApp receives one component in separate test cases. */
import { createApp, defineComponent, h, nextTick, ref } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import PasswordStage from './PasswordStage.vue'

function setWheelPassword(host: HTMLElement, password: string) {
  for (const [index, digit] of [...password].entries()) {
    const increaseButton = host.querySelector<HTMLButtonElement>(
      `button[aria-label="第 ${index + 1} 位增加"]`,
    )
    for (let step = 0; step < Number(digit); step += 1) increaseButton?.click()
  }
  host.querySelector<HTMLButtonElement>('button.password-unlock')?.click()
}

async function openEnvelopeFromLockbox(host: HTMLElement) {
  host.querySelector<HTMLButtonElement>('button[aria-label="打开盒中的信封"]')?.click()
  vi.advanceTimersByTime(360)
  await nextTick()
}

describe('PasswordStage', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
    document.body.innerHTML = ''
  })

  it('shows the creator hint only after 30 seconds', async () => {
    const host = document.createElement('div')
    document.body.append(host)
    const active = ref(false)
    const TestHost = defineComponent({
      setup: () => () => h(PasswordStage, {
        active: active.value,
        password: '0123',
        passwordHint: '我们的纪念日',
        photos: [],
        loveLetter: '测试情书',
      }),
    })
    const app = createApp(TestHost)
    app.mount(host)

    expect(host.textContent).toContain('转动密码，打开礼物')
    expect(host.querySelector<HTMLImageElement>('img[alt="蓝色水彩旅行密码箱"]')?.src)
      .toContain('/assets/love-journey/gift-lockbox-closed.png')
    expect(host.querySelectorAll('[role="spinbutton"]')).toHaveLength(4)
    expect(host.textContent).not.toContain('提示：我们的纪念日')
    vi.advanceTimersByTime(30_000)
    await nextTick()
    expect(host.textContent).not.toContain('提示：我们的纪念日')

    active.value = true
    await nextTick()
    vi.advanceTimersByTime(29_999)
    await nextTick()
    expect(host.textContent).not.toContain('提示：我们的纪念日')

    vi.advanceTimersByTime(1)
    await nextTick()
    expect(host.textContent).toContain('提示：我们的纪念日')

    setWheelPassword(host, '0123')
    vi.advanceTimersByTime(120)
    await nextTick()
    expect(host.textContent).not.toContain('提示：我们的纪念日')
    expect(host.textContent).toContain('密码正确，盒子打开了')
    expect(host.querySelector('.password-lockbox--open')).not.toBeNull()
    expect(host.querySelector('.envelope-scene')).toBeNull()

    await openEnvelopeFromLockbox(host)
    expect(host.querySelector('.password-lockbox')).toBeNull()
    expect(host.textContent).toContain('向上滑动，从信封里取出回忆。')

    app.unmount()
  })

  it('turns only once during one continuous wheel gesture', async () => {
    const host = document.createElement('div')
    document.body.append(host)
    const app = createApp(PasswordStage, {
      active: true,
      password: '9999',
      photos: [],
      loveLetter: '测试情书',
    })
    app.mount(host)

    const firstWheel = host.querySelector<HTMLElement>('[role="spinbutton"]')!
    for (let eventIndex = 0; eventIndex < 30; eventIndex += 1) {
      firstWheel.dispatchEvent(new WheelEvent('wheel', {
        bubbles: true,
        cancelable: true,
        deltaY: 3,
      }))
    }
    await nextTick()
    expect(firstWheel.getAttribute('aria-valuenow')).toBe('1')

    vi.advanceTimersByTime(180)
    for (let eventIndex = 0; eventIndex < 12; eventIndex += 1) {
      firstWheel.dispatchEvent(new WheelEvent('wheel', {
        bubbles: true,
        cancelable: true,
        deltaY: 3,
      }))
    }
    await nextTick()
    expect(firstWheel.getAttribute('aria-valuenow')).toBe('2')

    app.unmount()
  })

  it('reveals all photos and then the letter before completing', async () => {
    const host = document.createElement('div')
    document.body.append(host)
    const onComplete = vi.fn()
    const app = createApp(PasswordStage, {
      active: true,
      password: '0123',
      passwordHint: '不应再出现',
      photos: [
        { key: 'photo-1', type: 'image', url: '/one.png', mimeType: 'image/png', expiresAt: '2099-01-01T00:00:00Z' },
        { key: 'photo-2', type: 'image', url: '/two.png', mimeType: 'image/png', expiresAt: '2099-01-01T00:00:00Z' },
      ],
      loveLetter: '写给你的测试情书',
      onComplete,
    })
    app.mount(host)

    setWheelPassword(host, '0123')
    vi.advanceTimersByTime(120)
    await nextTick()

    expect(onComplete).not.toHaveBeenCalled()
    expect(host.querySelector<HTMLButtonElement>('button[aria-label="打开盒中的信封"]')).not.toBeNull()
    expect(host.querySelector('.envelope-scene')).toBeNull()

    await openEnvelopeFromLockbox(host)
    expect(host.textContent).toContain('照片正在加载，请稍候…')
    expect(host.textContent).toContain('正在准备这张照片，请稍候…')
    expect(host.querySelector<HTMLButtonElement>('button[aria-label="照片 1 正在加载"]')?.disabled).toBe(true)

    host.querySelector<HTMLImageElement>('img[alt="待取出的照片 1"]')
      ?.dispatchEvent(new Event('load'))
    await nextTick()
    expect(host.textContent).toContain('已取出 0 / 2 张照片')

    host.querySelector<HTMLButtonElement>('button[aria-label="向上滑动或点击取出照片 1"]')?.click()
    vi.advanceTimersByTime(380)
    await nextTick()
    expect(host.textContent).toContain('照片 1')
    expect(host.textContent).toContain('照片正在加载，请稍候…')

    host.querySelector<HTMLImageElement>('img[alt="待取出的照片 2"]')
      ?.dispatchEvent(new Event('load'))
    await nextTick()

    host.querySelector<HTMLButtonElement>('button[aria-label="向上滑动或点击取出照片 2"]')?.click()
    vi.advanceTimersByTime(380)
    await nextTick()
    expect(host.textContent).toContain('再上滑一次打开情书')

    host.querySelector<HTMLButtonElement>('button[aria-label="向上滑动或点击取出情书"]')?.click()
    vi.advanceTimersByTime(380)
    await nextTick()
    expect(host.textContent).toContain('写给你的测试情书')
    expect(onComplete).toHaveBeenCalledOnce()

    vi.advanceTimersByTime(30_000)
    await nextTick()
    expect(host.textContent).not.toContain('提示：不应再出现')

    app.unmount()
  })

  it('shows an error for a wrong wheel combination and allows another attempt', async () => {
    const host = document.createElement('div')
    document.body.append(host)
    const app = createApp(PasswordStage, {
      active: true,
      password: '1234',
      photos: [],
      loveLetter: '测试情书',
    })
    app.mount(host)

    host.querySelector<HTMLButtonElement>('button.password-unlock')?.click()
    vi.advanceTimersByTime(120)
    await nextTick()

    expect(host.textContent).toContain('密码不正确，请再试一次')
    expect(host.querySelector('.password-lockbox--error')).not.toBeNull()

    vi.advanceTimersByTime(550)
    await nextTick()
    expect(host.textContent).not.toContain('密码不正确，请再试一次')
    expect(host.querySelector<HTMLButtonElement>('button.password-unlock')?.disabled).toBe(false)

    app.unmount()
  })
})
