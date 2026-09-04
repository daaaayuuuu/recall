/* eslint-disable vue/one-component-per-file */
import { createApp, defineComponent, h } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'

import CreatorLayout from './CreatorLayout.vue'

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    user: { userId: 'creator', nickname: '留刻用户', avatarAssetId: null },
  }),
}))

const RouterLinkStub = defineComponent({
  inheritAttrs: false,
  props: { to: { type: [String, Object], required: true } },
  setup(_props, { attrs, slots }) {
    return () => h('a', attrs, slots.default?.())
  },
})

function mountLayout() {
  const host = document.createElement('div')
  document.body.append(host)
  const app = createApp(CreatorLayout)
  app.component('RouterLink', RouterLinkStub)
  app.component('RouterView', defineComponent({ setup: () => () => h('main') }))
  app.mount(host)
  return { app, host }
}

afterEach(() => {
  document.body.innerHTML = ''
})

describe('creator header', () => {
  it('keeps the wordmark header without a filling status', () => {
    const { app, host } = mountLayout()

    expect(host.querySelector('.creator-header__status')).toBeNull()
    expect(host.querySelector('.shell__header--recall-wordmark')).not.toBeNull()

    app.unmount()
  })
})
