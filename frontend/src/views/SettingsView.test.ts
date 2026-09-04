/* eslint-disable vue/component-definition-name-casing, vue/one-component-per-file */
import { createApp, defineComponent, h, nextTick } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import SettingsView from './SettingsView.vue'

const { authMocks, messageBoxMocks, messageMocks, routerMocks } = vi.hoisted(() => ({
  authMocks: {
    user: {
      id: 'creator-1',
      userId: 'recall-user',
      nickname: '旧昵称',
      avatarAssetId: null as string | null,
      createdAt: '2026-08-23T00:00:00Z',
      updatedAt: '2026-08-23T00:00:00Z',
    },
    changePassword: vi.fn(),
    deleteAvatar: vi.fn(),
    logout: vi.fn(),
    updateNickname: vi.fn(),
    uploadAvatar: vi.fn(),
  },
  messageBoxMocks: { confirm: vi.fn() },
  messageMocks: { error: vi.fn(), success: vi.fn(), warning: vi.fn() },
  routerMocks: { push: vi.fn() },
}))

vi.mock('@/stores/auth', () => ({ useAuthStore: () => authMocks }))
vi.mock('vue-router', () => ({ useRouter: () => routerMocks }))
vi.mock('element-plus', () => ({ ElMessage: messageMocks, ElMessageBox: messageBoxMocks }))

const ButtonStub = defineComponent({
  inheritAttrs: false,
  props: {
    disabled: Boolean,
    loading: Boolean,
    nativeType: { type: String, default: 'button' },
  },
  setup(props, { attrs, slots }) {
    return () => h('button', {
      ...attrs,
      disabled: props.disabled || props.loading,
      type: props.nativeType,
    }, slots.default?.())
  },
})

const DialogStub = defineComponent({
  props: {
    modelValue: Boolean,
    title: { type: String, default: '' },
  },
  emits: ['update:modelValue'],
  setup(props, { slots }) {
    return () => props.modelValue
      ? h('section', { role: 'dialog', 'aria-label': props.title }, [
          h('h2', props.title),
          slots.default?.(),
        ])
      : null
  },
})

const FormStub = defineComponent({
  setup(_props, { attrs, slots }) {
    return () => h('form', attrs, slots.default?.())
  },
})

const FormItemStub = defineComponent({
  props: { label: { type: String, default: '' } },
  setup(props, { slots }) {
    return () => h('label', [h('span', props.label), slots.default?.()])
  },
})

const InputStub = defineComponent({
  inheritAttrs: false,
  props: {
    modelValue: { type: String, default: '' },
    type: { type: String, default: 'text' },
  },
  emits: ['update:modelValue'],
  setup(props, { attrs, emit }) {
    return () => h('input', {
      ...attrs,
      type: props.type,
      value: props.modelValue,
      onInput: (event: Event) => emit(
        'update:modelValue',
        (event.target as HTMLInputElement).value,
      ),
    })
  },
})

function mountSettings() {
  const host = document.createElement('div')
  document.body.append(host)
  const app = createApp(SettingsView)
  app.component('el-button', ButtonStub)
  app.component('el-dialog', DialogStub)
  app.component('el-form', FormStub)
  app.component('el-form-item', FormItemStub)
  app.component('el-input', InputStub)
  app.mount(host)
  return { app, host }
}

async function flushView() {
  await Promise.resolve()
  await nextTick()
}

function clickButton(host: HTMLElement, ariaLabel: string) {
  host.querySelector<HTMLButtonElement>(`button[aria-label="${ariaLabel}"]`)?.click()
}

beforeEach(() => {
  authMocks.user.nickname = '旧昵称'
  authMocks.user.avatarAssetId = null
  authMocks.updateNickname.mockImplementation(async (nickname: string) => {
    authMocks.user.nickname = nickname
  })
  authMocks.changePassword.mockResolvedValue({ message: '密码已修改' })
})

afterEach(() => {
  document.body.innerHTML = ''
  vi.clearAllMocks()
})

describe('settings on-demand editors', () => {
  it('shows concise account summaries without rendering any form by default', () => {
    const { app, host } = mountSettings()

    expect(host.textContent).toContain('旧昵称')
    expect(host.textContent).toContain('recall-user')
    expect(host.textContent).not.toContain('用户ID是你的登录凭据')
    expect(host.querySelector('form')).toBeNull()
    expect(host.querySelector('input')).toBeNull()
    expect(host.querySelector('[role="dialog"]')).toBeNull()
    expect(host.querySelector('[aria-label="修改头像"]')).not.toBeNull()
    expect(host.querySelector('[aria-label="修改昵称"]')).not.toBeNull()
    expect(host.querySelector('[aria-label="修改密码"]')).not.toBeNull()

    app.unmount()
  })

  it('opens only the editor selected by the user', async () => {
    const { app, host } = mountSettings()

    clickButton(host, '修改头像')
    await flushView()
    expect(host.querySelector('[role="dialog"]')?.getAttribute('aria-label')).toBe('修改头像')
    expect(host.querySelector('input[type="file"]')).not.toBeNull()
    expect(host.querySelector('input[type="password"]')).toBeNull()

    app.unmount()
    const passwordMount = mountSettings()
    clickButton(passwordMount.host, '修改密码')
    await flushView()
    expect(passwordMount.host.querySelector('[role="dialog"]')?.getAttribute('aria-label')).toBe('修改密码')
    expect(passwordMount.host.querySelectorAll('input[type="password"]')).toHaveLength(3)
    expect(passwordMount.host.querySelector('input[type="file"]')).toBeNull()

    passwordMount.app.unmount()
  })

  it('prefills the nickname and closes its dialog after a successful save', async () => {
    const { app, host } = mountSettings()

    clickButton(host, '修改昵称')
    await flushView()
    const input = host.querySelector<HTMLInputElement>('input[autocomplete="nickname"]')!
    expect(input.value).toBe('旧昵称')

    input.value = '新昵称'
    input.dispatchEvent(new Event('input', { bubbles: true }))
    host.querySelector('form')?.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await flushView()

    expect(authMocks.updateNickname).toHaveBeenCalledWith('新昵称')
    expect(messageMocks.success).toHaveBeenCalledWith('个人资料已保存')
    expect(host.querySelector('[role="dialog"]')).toBeNull()

    app.unmount()
  })
})
