export interface PasswordState {
  input: string
  status: 'playing' | 'error' | 'completed'
}

export const PASSWORD_LENGTH = 4

export function createPasswordState(): PasswordState {
  return { input: '', status: 'playing' }
}

export function enterPasswordDigit(state: PasswordState, digit: string): PasswordState {
  if (state.status !== 'playing' || !/^\d$/.test(digit) || state.input.length >= PASSWORD_LENGTH) return state
  return { ...state, input: state.input + digit }
}

export function deletePasswordDigit(state: PasswordState): PasswordState {
  if (state.status !== 'playing' || state.input.length === 0) return state
  return { ...state, input: state.input.slice(0, -1) }
}

export function verifyPassword(state: PasswordState, password: string): PasswordState {
  if (state.status !== 'playing' || state.input.length !== PASSWORD_LENGTH) return state
  return { ...state, status: /^\d{4}$/.test(password) && state.input === password ? 'completed' : 'error' }
}

export function clearPasswordError(state: PasswordState): PasswordState {
  return state.status === 'error' ? createPasswordState() : state
}
