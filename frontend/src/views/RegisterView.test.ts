/* eslint-disable vue/component-definition-name-casing, vue/one-component-per-file */
import { createApp, defineComponent, h, nextTick } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { register } from '@/api/auth'
import { APIError } from '@/api/client'
import {
  validateInvitationCode,
  validateNickname,
  validatePassword,
  validatePasswordConfirmation,
} from '@/utils/registration'
import { validateUserId } from '@/utils/userId'

import RegisterView from './RegisterView.vue'

vi.mock('@/api/auth', () => ({ register: vi.fn() }))
vi.mock('element-plus', () => ({ ElMessage: { error: vi.fn() } }))

const FormStub = defineComponent({
  setup(_props, { attrs, slots }) {
    return () => h('form', attrs, slots.default?.())
  },
})

const FormItemStub = defineComponent({
  props: {
    label: { type: String, default: '' },
    error: { type: String, default: '' },
  },
  setup(props, { slots }) {
    return () => h('div', [
      h('label', props.label),
      slots.default?.(),
      props.error ? h('span', { class: 'field-error' }, props.error) : null,
    ])
  },
})

const InputStub = defineComponent({
  inheritAttrs: false,
  props: {
    modelValue: { type: String, default: '' },
    type: { type: String, default: 'text' },
  },
  emits: ['update:modelValue', 'input'],
  setup(props, { attrs, emit }) {
    return () => h('input', {
      ...attrs,
      type: props.type ?? 'text',
      value: props.modelValue,
      onInput: (event: Event) => {
        const value = (event.target as HTMLInputElement).value
        emit('update:modelValue', value)
        emit('input', value)
      },
    })
  },
})

const ButtonStub = defineComponent({
  props: {
    nativeType: { type: String, default: 'button' },
    loading: Boolean,
  },
  setup(props, { slots }) {
    return () => h('button', { type: props.nativeType ?? 'button', disabled: props.loading }, slots.default?.())
  },
})

const RouterLinkStub = defineComponent({
  setup(_props, { slots }) {
    return () => h('a', { href: '#' }, slots.default?.())
  },
})

function mountRegister() {
  const host = document.createElement('div')
  document.body.append(host)
  const app = createApp(RegisterView)
  app.component('el-form', FormStub)
  app.component('el-form-item', FormItemStub)
  app.component('el-input', InputStub)
  app.component('el-button', ButtonStub)
  app.component('el-result', defineComponent({ setup: () => () => null }))
  app.component('RouterLink', RouterLinkStub)
  app.mount(host)
  return { app, host }
}

function fill(input: HTMLInputElement, value: string) {
  input.value = value
  input.dispatchEvent(new Event('input', { bubbles: true }))
}

async function submit(host: HTMLElement) {
  host.querySelector('form')?.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
  await Promise.resolve()
  await nextTick()
}

afterEach(() => {
  vi.clearAllMocks()
})

describe('registration validation', () => {
  it('explains the supported rules for every registration field', () => {
    expect(validateInvitationCode('')).toBe('请输入邀请码')
    expect(validateInvitationCode('ABCD')).toBe('')
    expect(validateInvitationCode('7KDM-N4PX')).toBe('')
    expect(validateInvitationCode('future-format:v2')).toBe('')
    expect(validateNickname('昵'.repeat(65))).toBe('昵称不能超过 64 个字符')
    expect(validateNickname('昵称')).toBe('')
    expect(validateUserId('')).toBe('请输入用户ID')
    expect(validateUserId('ab')).toBe('用户ID长度应为 3–32 位')
    expect(validateUserId('13135570592')).toBe('用户ID必须以小写字母开头')
    expect(validateUserId('creator@example.com')).toBe('用户ID只能包含小写字母、数字、下划线（_）或连字符（-）')
    expect(validateUserId('creator_01')).toBe('')
    expect(validatePassword('')).toBe('请输入密码')
    expect(validatePassword('短密码')).toBe('密码长度应为 8–128 个字符')
    expect(validatePassword('八个字符密码足够')).toBe('')
    expect(validatePasswordConfirmation('password-123', '')).toBe('请再次输入密码')
    expect(validatePasswordConfirmation('password-123', 'password-456')).toBe('两次输入的密码不一致')
  })

  it('keeps guidance hidden until the user enters an invalid user ID or password', async () => {
    const { app, host } = mountRegister()
    const inputs = host.querySelectorAll<HTMLInputElement>('input')

    expect(host.textContent).not.toContain('3–32 位，以小写字母开头')
    expect(host.textContent).not.toContain('格式为 XXXX-XXXX')
    expect(host.textContent).not.toContain('密码长度应为 8–128 个字符')

    fill(inputs[0], 'abcd1234')
    await nextTick()
    expect(inputs[0].value).toBe('ABCD1234')

    fill(inputs[2], '13135570592')
    fill(inputs[3], '短密码')
    await nextTick()

    expect(host.textContent).toContain('用户ID必须以小写字母开头')
    expect(host.textContent).toContain('密码长度应为 8–128 个字符')

    fill(inputs[2], 'creator_01')
    fill(inputs[3], 'password-123')
    await nextTick()

    expect(host.textContent).not.toContain('用户ID必须以小写字母开头')
    expect(host.textContent).not.toContain('密码长度应为 8–128 个字符')
    app.unmount()
  })

  it('shows all client-side errors together on submit and blocks the request', async () => {
    const { app, host } = mountRegister()
    const inputs = host.querySelectorAll<HTMLInputElement>('input')

    fill(inputs[2], '13135570592')
    await submit(host)

    expect(host.textContent).toContain('请输入邀请码')
    expect(host.textContent).toContain('用户ID必须以小写字母开头')
    expect(host.textContent).toContain('请输入密码')
    expect(host.textContent).toContain('请再次输入密码')
    expect(register).not.toHaveBeenCalled()
    app.unmount()
  })

  it('renders backend field errors beside their corresponding inputs', async () => {
    vi.mocked(register).mockRejectedValueOnce(new APIError(
      422,
      'VALIDATION_ERROR',
      '请检查输入内容',
      {
        invitationCode: '邀请码无效或已被使用',
        nickname: '昵称不符合服务端规则',
        userId: '该用户ID不符合服务端规则',
        password: '密码不符合服务端规则',
      },
    ))
    const { app, host } = mountRegister()
    const inputs = host.querySelectorAll<HTMLInputElement>('input')
    fill(inputs[0], '7KDM-N4PX')
    fill(inputs[2], 'creator_01')
    fill(inputs[3], 'password-123')
    fill(inputs[4], 'password-123')

    await submit(host)

    expect(register).toHaveBeenCalledOnce()
    expect(host.textContent).toContain('邀请码无效或已被使用')
    expect(host.textContent).toContain('昵称不符合服务端规则')
    expect(host.textContent).toContain('该用户ID不符合服务端规则')
    expect(host.textContent).toContain('密码不符合服务端规则')
    app.unmount()
  })
})
