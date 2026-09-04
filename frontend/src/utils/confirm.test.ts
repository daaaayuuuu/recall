import { ElMessageBox } from 'element-plus'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { confirmDestructiveAction } from './confirm'

vi.mock('element-plus', () => ({
  ElMessageBox: { confirm: vi.fn() },
}))

describe('confirmDestructiveAction', () => {
  beforeEach(() => {
    vi.mocked(ElMessageBox.confirm).mockReset()
  })

  it('applies the shared recall dialog treatment while preserving action copy', () => {
    confirmDestructiveAction('操作后无法恢复。', '确认操作', {
      type: 'error',
      confirmButtonText: '继续删除',
      customClass: 'feature-dialog',
      confirmButtonClass: 'feature-danger-button',
    })

    expect(ElMessageBox.confirm).toHaveBeenCalledWith(
      '操作后无法恢复。',
      '确认操作',
      expect.objectContaining({
        type: 'error',
        confirmButtonText: '继续删除',
        cancelButtonText: '取消',
        autofocus: false,
        closeOnClickModal: false,
        customClass: 'recall-message-box feature-dialog',
        confirmButtonClass: 'recall-message-box__danger feature-danger-button',
      }),
    )
  })
})
