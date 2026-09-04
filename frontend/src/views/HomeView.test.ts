/* eslint-disable vue/component-definition-name-casing, vue/one-component-per-file */
import { createApp, defineComponent, h, nextTick } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import {
  createGame,
  createVersion,
  deleteAsset,
  getGame,
  listAssets,
  listTemplates,
  listVersions,
  polishLoveLetter,
  updateGame,
  uploadAsset,
  type GameTemplate,
} from '@/api/games'
import { APIError } from '@/api/client'
import { submitGeneration } from '@/api/generation'

import HomeView from './HomeView.vue'

const routerMocks = vi.hoisted(() => ({
  push: vi.fn(),
  replace: vi.fn(),
}))

const authMocks = vi.hoisted(() => ({
  ensureCSRF: vi.fn().mockResolvedValue('csrf-token'),
}))

const messageMocks = vi.hoisted(() => ({
  error: vi.fn(),
  info: vi.fn(),
  success: vi.fn(),
  warning: vi.fn(),
}))

vi.mock('vue-router', () => ({
  useRouter: () => routerMocks,
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => authMocks,
}))

vi.mock('@/api/games', () => ({
  createGame: vi.fn(),
  createVersion: vi.fn(),
  deleteAsset: vi.fn(),
  getGame: vi.fn(),
  listAssets: vi.fn(),
  listTemplates: vi.fn(),
  listVersions: vi.fn(),
  polishLoveLetter: vi.fn(),
  updateGame: vi.fn(),
  uploadAsset: vi.fn(),
}))

vi.mock('@/api/generation', () => ({
  submitGeneration: vi.fn(),
}))

vi.mock('element-plus', () => ({
  ElMessage: messageMocks,
}))

const template = {
  id: 'love-journey',
  version: '1.1.0',
  name: '爱的旅程',
  description: '测试模板',
  inputSchemaVersion: 3,
  generationEnabled: true,
  cover: {
    key: 'cover',
    label: '双人自拍正脸合照（可选）',
    helpText: '最多上传 1 张。',
    required: false,
    minItems: 0,
    maxItems: 1,
    sortable: false,
  },
  scenes: [{
    key: 'materials',
    name: '礼物资料',
    summary: '统一资料',
    textInputs: [
      {
        key: 'loveLetter',
        label: '写给对方的情书',
        placeholder: '写下想说的话',
        inputType: 'textarea' as const,
        required: true,
        maxLength: 1000,
      },
      {
        key: 'passwordHint',
        label: '密码提示（可选）',
        placeholder: '例如：纪念日',
        inputType: 'text' as const,
        required: false,
        maxLength: 100,
      },
      {
        key: 'letterPassword',
        label: '拆信密码',
        placeholder: '请输入 4 位数字',
        inputType: 'password' as const,
        required: true,
        minLength: 4,
        maxLength: 4,
        format: 'four-digit-code' as const,
      },
    ],
    assetSlots: [{
      key: 'travelPhotos',
      label: '旅行照片（可选）',
      helpText: '最多上传 2 张。',
      required: false,
      minItems: 0,
      maxItems: 2,
      sortable: true,
    }],
  }],
}

const FormStub = defineComponent({
  setup(_props, { attrs, slots }) {
    return () => h('form', attrs, slots.default?.())
  },
})

const FormItemStub = defineComponent({
  props: {
    label: { type: String, default: '' },
  },
  setup(props, { slots }) {
    return () => h('div', [slots.label?.() ?? h('label', props.label), slots.default?.()])
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
    return () => h(props.type === 'textarea' ? 'textarea' : 'input', {
      ...attrs,
      type: props.type === 'textarea' ? undefined : props.type,
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
  inheritAttrs: false,
  props: {
    nativeType: { type: String, default: 'button' },
    disabled: Boolean,
    loading: Boolean,
  },
  setup(props, { attrs, slots }) {
    return () => h('button', {
      ...attrs,
      type: props.nativeType,
      disabled: props.disabled || props.loading,
    }, slots.default?.())
  },
})

function mountHome(gameId?: string) {
  const host = document.createElement('div')
  document.body.append(host)
  const app = createApp(HomeView, gameId ? { gameId } : {})
  app.component('el-form', FormStub)
  app.component('el-form-item', FormItemStub)
  app.component('el-input', InputStub)
  app.component('el-button', ButtonStub)
  app.component('el-skeleton', defineComponent({ setup: () => () => null }))
  app.component('el-empty', defineComponent({ setup: () => () => null }))
  app.component('RouterLink', defineComponent({
    setup(_props, { slots }) {
      return () => h('a', slots.default?.())
    },
  }))
  app.mount(host)
  return { app, host }
}

async function flushView() {
  await Promise.resolve()
  await Promise.resolve()
  await nextTick()
}

function fill(input: HTMLInputElement | HTMLTextAreaElement, value: string) {
  input.value = value
  input.dispatchEvent(new Event('input', { bubbles: true }))
}

function buttonWithText(host: HTMLElement, text: string) {
  return [...host.querySelectorAll<HTMLButtonElement>('button')]
    .find((button) => button.textContent?.includes(text))
}

beforeEach(() => {
  vi.mocked(listTemplates).mockResolvedValue({ items: [template], maxSourceImageBytes: 25 * 1024 * 1024 })
  vi.mocked(createGame).mockResolvedValue({
    game: { id: 'game-1' },
    version: { id: 'version-1' },
  } as Awaited<ReturnType<typeof createGame>>)
  vi.mocked(createVersion).mockResolvedValue({
    id: 'version-edit',
    gameId: 'game-edit',
    versionNumber: 2,
    status: 'draft',
    memoryText: '',
    sceneInputs: {},
    inputSchemaVersion: 3,
    templateId: 'love-journey',
    templateVersion: '1.1.0',
    assetCount: 2,
    createdAt: '2026-08-23T00:02:00Z',
    updatedAt: '2026-08-23T00:02:00Z',
  })
  vi.mocked(getGame).mockResolvedValue({
    id: 'game-edit',
    title: '原来的故事',
    description: '原来的描述',
    coverAssetId: 'cover-asset',
    coverPreviewUrl: '/cover.png',
    status: 'ready',
    currentVersionId: 'version-current',
    assetCount: 2,
    createdAt: '2026-08-23T00:00:00Z',
    updatedAt: '2026-08-23T00:01:00Z',
  })
  vi.mocked(listVersions).mockResolvedValue({
    items: [{
      id: 'version-current',
      gameId: 'game-edit',
      versionNumber: 1,
      status: 'ready',
      memoryText: '',
      sceneInputs: {
        loveLetter: '原来的情书',
        letterPassword: '0820',
        passwordHint: '原来的提示',
      },
      inputSchemaVersion: 3,
      templateId: 'love-journey',
      templateVersion: '1.1.0',
      assetCount: 2,
      createdAt: '2026-08-23T00:00:00Z',
      updatedAt: '2026-08-23T00:01:00Z',
    }],
  })
  vi.mocked(listAssets).mockResolvedValue({
    items: [
      {
        id: 'cover-asset',
        role: 'cover',
        slotKey: 'cover',
        mimeType: 'image/png',
        sizeBytes: 123,
        width: 640,
        height: 640,
        sortOrder: 0,
        previewUrl: '/cover.png',
        createdAt: '2026-08-23T00:00:00Z',
      },
      {
        id: 'travel-asset',
        role: 'source',
        slotKey: 'travelPhotos',
        mimeType: 'image/png',
        sizeBytes: 456,
        width: 640,
        height: 640,
        sortOrder: 0,
        previewUrl: '/travel.png',
        createdAt: '2026-08-23T00:00:00Z',
      },
    ],
  })
  vi.mocked(updateGame).mockResolvedValue({} as Awaited<ReturnType<typeof updateGame>>)
  vi.mocked(deleteAsset).mockResolvedValue(undefined)
  vi.mocked(uploadAsset).mockResolvedValue({} as Awaited<ReturnType<typeof uploadAsset>>)
  vi.mocked(submitGeneration).mockResolvedValue({
    id: 'run-1',
    gameId: 'game-1',
    gameVersionId: 'version-1',
    status: 'queued',
    stage: 'queued',
    progress: 0,
  } as Awaited<ReturnType<typeof submitGeneration>>)
  globalThis.window.scrollTo = vi.fn()
})

afterEach(() => {
  document.body.innerHTML = ''
  vi.clearAllMocks()
})

describe('creation flow', () => {
  it('shows one group at a time and validates before moving forward', async () => {
    const { app, host } = mountHome()
    await flushView()

    expect(host.textContent).toContain('先从你们两个开始')
    expect(host.textContent).toContain('作品名称')
    expect(host.textContent).toContain('双人自拍合照')
    expect(host.textContent).not.toContain('写给对方的情书')

    buttonWithText(host, '继续向下填写')?.click()
    await nextTick()
    expect(messageMocks.warning).toHaveBeenCalledWith('请填写作品名称')
    expect(host.textContent).toContain('先从你们两个开始')

    fill(host.querySelector<HTMLInputElement>('input:not([type="file"])')!, '我们的第 1095 天')
    buttonWithText(host, '继续向下填写')?.click()
    await nextTick()

    expect(host.textContent).toContain('选择照片，写下想说的话')
    expect(host.textContent).toContain('回忆照片')
    expect(host.textContent).toContain('✦ AI 润色')
    expect(host.textContent).not.toContain('密码提示（可选）')

    buttonWithText(host, '最后一步')?.click()
    await nextTick()
    expect(messageMocks.warning).toHaveBeenCalledWith('请填写“写给对方的情书”')
    expect(host.textContent).toContain('选择照片，写下想说的话')

    app.unmount()
  })

  it('keeps AI polish available on the letter step', async () => {
    vi.mocked(polishLoveLetter).mockResolvedValue({ polishedText: '润色后的情书', skipped: false })
    const { app, host } = mountHome()
    await flushView()

    fill(host.querySelector<HTMLInputElement>('input:not([type="file"])')!, '我们的故事')
    buttonWithText(host, '继续向下填写')?.click()
    await nextTick()

    const letter = host.querySelector<HTMLTextAreaElement>('textarea')!
    fill(letter, '原始情书')
    await nextTick()
    buttonWithText(host, 'AI 润色')?.click()
    await flushView()

    expect(polishLoveLetter).toHaveBeenCalledWith('原始情书', 'csrf-token')
    expect(letter.value).toBe('润色后的情书')
    expect(messageMocks.success).toHaveBeenCalledWith('情书已润色并回填，请确认后再创建')

    app.unmount()
  })

  it('normalizes the password and submits all three steps together', async () => {
    const { app, host } = mountHome()
    await flushView()

    fill(host.querySelector<HTMLInputElement>('input:not([type="file"])')!, '我们的故事')
    buttonWithText(host, '继续向下填写')?.click()
    await nextTick()
    fill(host.querySelector<HTMLTextAreaElement>('textarea')!, '写给你的信')
    buttonWithText(host, '最后一步')?.click()
    await nextTick()

    expect(host.textContent).toContain('设置拆信密码')
    expect(host.textContent).toContain('密码提示（可选）')
    expect(host.textContent).toContain('素材准备好了')

    const securityInputs = host.querySelectorAll<HTMLInputElement>('input:not([type="file"])')
    fill(securityInputs[0], '08a20-25')
    fill(securityInputs[1], '第一次旅行的日期')
    await nextTick()
    expect(securityInputs[0].value).toBe('0820')

    buttonWithText(host, '生成我们的回忆')?.click()
    await flushView()

    expect(createGame).toHaveBeenCalledWith(expect.objectContaining({
      title: '我们的故事',
      sceneInputs: expect.objectContaining({
        loveLetter: '写给你的信',
        letterPassword: '0820',
        passwordHint: '第一次旅行的日期',
      }),
    }), 'csrf-token')
    expect(submitGeneration).toHaveBeenCalledWith(
      'game-1',
      'version-1',
      expect.any(String),
      'csrf-token',
    )
    expect(routerMocks.replace).toHaveBeenCalledWith({
      name: 'generation-progress',
      params: { gameId: 'game-1', runId: 'run-1' },
    })

    app.unmount()
  })

  it('shows the generation page immediately while the draft request is still pending', async () => {
    let resolveCreate!: (value: Awaited<ReturnType<typeof createGame>>) => void
    vi.mocked(createGame).mockReturnValueOnce(new Promise((resolve) => {
      resolveCreate = resolve
    }))
    const { app, host } = mountHome()
    await flushView()

    fill(host.querySelector<HTMLInputElement>('input:not([type="file"])')!, '正在生成的故事')
    buttonWithText(host, '继续向下填写')?.click()
    await nextTick()
    fill(host.querySelector<HTMLTextAreaElement>('textarea')!, '写给你的信')
    buttonWithText(host, '最后一步')?.click()
    await nextTick()
    const securityInputs = host.querySelectorAll<HTMLInputElement>('input:not([type="file"])')
    fill(securityInputs[0], '0820')

    buttonWithText(host, '生成我们的回忆')?.click()
    await flushView()

    expect(host.textContent).toContain('让回忆慢慢成形')
    expect(routerMocks.replace).not.toHaveBeenCalled()

    resolveCreate({
      game: { id: 'game-1' },
      version: { id: 'version-1' },
    } as Awaited<ReturnType<typeof createGame>>)
    await flushView()
    await flushView()
    app.unmount()
  })

  it('blocks submission when the backend still advertises the old password contract', async () => {
    const staleTemplate = structuredClone(template) as unknown as GameTemplate
    const stalePassword = staleTemplate.scenes[0]?.textInputs.find((input) => input.key === 'letterPassword')
    Object.assign(stalePassword ?? {}, { minLength: 6, maxLength: 6, format: 'six-digit-code' })
    vi.mocked(listTemplates).mockResolvedValueOnce({
      items: [staleTemplate],
      maxSourceImageBytes: 25 * 1024 * 1024,
    })
    const { app, host } = mountHome()
    await flushView()

    fill(host.querySelector<HTMLInputElement>('input:not([type="file"])')!, '我们的故事')
    buttonWithText(host, '继续向下填写')?.click()
    await nextTick()
    fill(host.querySelector<HTMLTextAreaElement>('textarea')!, '写给你的信')
    buttonWithText(host, '最后一步')?.click()
    await nextTick()
    const securityInputs = host.querySelectorAll<HTMLInputElement>('input:not([type="file"])')
    fill(securityInputs[0], '0820')

    buttonWithText(host, '生成我们的回忆')?.click()
    await flushView()

    expect(createGame).not.toHaveBeenCalled()
    expect(messageMocks.error).toHaveBeenCalledWith('创建服务的密码规则尚未同步，请刷新页面后重试')
    app.unmount()
  })

  it('shows the backend field error instead of the generic validation message', async () => {
    vi.mocked(createGame).mockRejectedValueOnce(new APIError(
      422,
      'VALIDATION_ERROR',
      '请检查输入内容',
      { 'sceneInputs.letterPassword': '拆信密码必须是 4 位数字' },
    ))
    const { app, host } = mountHome()
    await flushView()

    fill(host.querySelector<HTMLInputElement>('input:not([type="file"])')!, '我们的故事')
    buttonWithText(host, '继续向下填写')?.click()
    await nextTick()
    fill(host.querySelector<HTMLTextAreaElement>('textarea')!, '写给你的信')
    buttonWithText(host, '最后一步')?.click()
    await nextTick()
    const securityInputs = host.querySelectorAll<HTMLInputElement>('input:not([type="file"])')
    fill(securityInputs[0], '0820')

    buttonWithText(host, '生成我们的回忆')?.click()
    await flushView()

    expect(messageMocks.error).toHaveBeenCalledWith('拆信密码必须是 4 位数字')
    app.unmount()
  })

  it('prefills the creation flow and generates a new version when editing', async () => {
    const { app, host } = mountHome('game-edit')
    await flushView()
    await flushView()

    expect(host.textContent).toContain('修改内容')
    const title = host.querySelector<HTMLInputElement>('input:not([type="file"])')!
    expect(title.value).toBe('原来的故事')
    expect(host.querySelector<HTMLImageElement>('[alt="已选择的双人自拍预览"]')?.src).toContain('/cover.png')
    fill(title, '修改后的故事')

    buttonWithText(host, '继续向下填写')?.click()
    await nextTick()
    const letter = host.querySelector<HTMLTextAreaElement>('textarea')!
    expect(letter.value).toBe('原来的情书')
    expect(host.querySelector<HTMLImageElement>('[alt="已选择的回忆照片 1"]')?.src).toContain('/travel.png')
    fill(letter, '修改后的情书')

    buttonWithText(host, '最后一步')?.click()
    await nextTick()
    const securityInputs = host.querySelectorAll<HTMLInputElement>('input:not([type="file"])')
    expect(securityInputs[0]?.value).toBe('0820')
    expect(securityInputs[1]?.value).toBe('原来的提示')

    buttonWithText(host, '重新生成这份回忆')?.click()
    await flushView()
    await flushView()

    expect(createGame).not.toHaveBeenCalled()
    expect(updateGame).toHaveBeenCalledWith('game-edit', {
      title: '修改后的故事',
      description: '原来的描述',
    }, 'csrf-token')
    expect(createVersion).toHaveBeenCalledWith('game-edit', expect.objectContaining({
      loveLetter: '修改后的情书',
      letterPassword: '0820',
      passwordHint: '原来的提示',
    }), 'csrf-token')
    expect(deleteAsset).not.toHaveBeenCalled()
    expect(uploadAsset).not.toHaveBeenCalled()
    expect(submitGeneration).toHaveBeenCalledWith(
      'game-edit',
      'version-edit',
      expect.any(String),
      'csrf-token',
    )
    expect(routerMocks.replace).toHaveBeenCalledWith({
      name: 'generation-progress',
      params: { gameId: 'game-edit', runId: 'run-1' },
    })

    app.unmount()
  })

  it('removes an inherited image only from the new edited version', async () => {
    const { app, host } = mountHome('game-edit')
    await flushView()
    await flushView()

    buttonWithText(host, '移除照片')?.click()
    await nextTick()
    expect(host.querySelector('[alt="已选择的双人自拍预览"]')).toBeNull()

    buttonWithText(host, '继续向下填写')?.click()
    await nextTick()
    buttonWithText(host, '最后一步')?.click()
    await nextTick()
    buttonWithText(host, '重新生成这份回忆')?.click()
    await flushView()
    await flushView()

    expect(deleteAsset).toHaveBeenCalledWith(
      'game-edit',
      'version-edit',
      'cover-asset',
      'csrf-token',
    )
    expect(submitGeneration).toHaveBeenCalled()

    app.unmount()
  })
})
