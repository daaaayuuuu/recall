import { describe, expect, it } from 'vitest'

import {
  clearPasswordError,
  createPasswordState,
  deletePasswordDigit,
  enterPasswordDigit,
  verifyPassword,
} from './passwordState'

describe('password game', () => {
  it('accepts four digits and completes only for the configured password', () => {
    let state = createPasswordState()
    for (const digit of '0820') state = enterPasswordDigit(state, digit)
    expect(verifyPassword(state, '0820').status).toBe('completed')

    state = verifyPassword({ input: '1234', status: 'playing' }, '0820')
    expect(state.status).toBe('error')
    expect(clearPasswordError(state)).toEqual(createPasswordState())
  })

  it('stops accepting input after four digits and rejects six digit configs', () => {
    let state = createPasswordState()
    for (const digit of '258036') state = enterPasswordDigit(state, digit)
    expect(state.input).toBe('2580')
    expect(verifyPassword(state, '258036').status).toBe('error')
  })

  it('supports deleting the most recent digit', () => {
    const state = deletePasswordDigit(enterPasswordDigit(createPasswordState(), '2'))
    expect(state.input).toBe('')
  })
})
