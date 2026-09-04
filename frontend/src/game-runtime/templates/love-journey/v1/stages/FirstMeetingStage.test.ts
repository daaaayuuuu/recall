import { createApp, nextTick } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import FirstMeetingStage from './FirstMeetingStage.vue'
import { firstMeetingRoundSequences, type FirstMeetingEmoji } from './firstMeetingState'

const emojiLabels: Record<FirstMeetingEmoji, string> = {
  wink: '眨眼笑脸',
  heart: '爱心',
  laugh: '大笑脸',
  blush: '害羞笑脸',
}

function sequenceText(sequence: readonly FirstMeetingEmoji[]) {
  return sequence.map((emoji) => emojiLabels[emoji]).join('、')
}

describe('FirstMeetingStage', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
    document.body.innerHTML = ''
  })

  it('guides all three fixed rounds and completes once after each sent pair is displayed', async () => {
    const host = document.createElement('div')
    document.body.append(host)
    const onComplete = vi.fn()
    const app = createApp(FirstMeetingStage, { active: true, onComplete })
    app.mount(host)

    expect(host.querySelector<HTMLImageElement>('.first-meeting-artboard__background')?.src)
      .toContain('/assets/love-journey/first-meeting/background.png')
    expect(host.querySelector<HTMLImageElement>('.first-meeting-character--left')?.src)
      .toContain('/assets/love-journey/first-meeting/character-left.png')
    expect(host.querySelector<HTMLImageElement>('.first-meeting-character--right')?.src)
      .toContain('/assets/love-journey/first-meeting/character-right.png')
    const characterLayer = host.querySelector('.first-meeting-character-layer')
    expect(characterLayer?.querySelectorAll('.first-meeting-character')).toHaveLength(2)

    const choices = host.querySelectorAll<HTMLButtonElement>('[data-emoji]')
    const send = host.querySelector<HTMLButtonElement>('[data-testid="first-meeting-send"]')!
    const controlRow = host.querySelector('[data-testid="first-meeting-control-row"]')
    expect(choices).toHaveLength(4)
    expect(controlRow?.querySelectorAll('button')).toHaveLength(5)
    expect([...controlRow!.querySelectorAll('button')].map((control) => (
      control.getAttribute('data-emoji') ?? control.getAttribute('aria-label')
    ))).toEqual(['wink', 'heart', 'laugh', 'blush', '发送'])
    expect(send.disabled).toBe(true)
    expect(host.querySelector('[data-testid="first-meeting-player-bubble"]')).toBeNull()
    expect(host.querySelector('[data-testid="first-meeting-guide"]')?.getAttribute('data-target'))
      .toBe('wink')

    host.querySelector<HTMLButtonElement>('[data-emoji="heart"]')!.click()
    await nextTick()
    expect(host.querySelector('[data-testid="first-meeting-guide"]')?.getAttribute('data-target'))
      .toBe('wink')

    for (const [roundIndex, sequence] of firstMeetingRoundSequences.entries()) {
      const readableSequence = sequenceText(sequence)
      const partnerBubble = host.querySelector<HTMLElement>(
        '[data-testid="first-meeting-partner-bubble"]',
      )!
      expect(partnerBubble.getAttribute('aria-label')).toBe(`对方发来：${readableSequence}`)
      expect(host.querySelector<HTMLElement>('.first-meeting-scene')?.dataset.round)
        .toBe(String(roundIndex + 1))

      for (const [emojiIndex, emoji] of sequence.entries()) {
        const choice = host.querySelector<HTMLButtonElement>(`[data-emoji="${emoji}"]`)!
        choice.click()
        await nextTick()

        const expectedTarget = sequence[emojiIndex + 1] ?? 'send'
        expect(host.querySelector('[data-testid="first-meeting-guide"]')?.getAttribute('data-target'))
          .toBe(expectedTarget)
      }

      expect(send.disabled).toBe(false)
      send.click()
      await nextTick()

      expect(send.disabled).toBe(true)
      expect(host.querySelector('[data-testid="first-meeting-guide"]')?.getAttribute('data-target'))
        .toBeNull()
      expect(host.querySelector<HTMLElement>(
        '[data-testid="first-meeting-player-bubble"]',
      )?.getAttribute('aria-label')).toBe(`玩家发出：${readableSequence}`)
      expect(onComplete).not.toHaveBeenCalled()

      vi.advanceTimersByTime(899)
      await nextTick()
      expect(host.querySelector<HTMLElement>('.first-meeting-scene')?.dataset.round)
        .toBe(String(roundIndex + 1))

      vi.advanceTimersByTime(1)
      await nextTick()
    }

    expect(host.querySelector<HTMLElement>('.first-meeting-scene')?.dataset.phase)
      .toBe('completed')
    expect(onComplete).toHaveBeenCalledOnce()
    expect(onComplete).toHaveBeenCalledWith(expect.objectContaining({
      stageId: 'first-meeting',
      actionCount: 3,
      metadata: {
        roundSequences: firstMeetingRoundSequences.map((sequence) => [...sequence]),
      },
    }))

    vi.advanceTimersByTime(5000)
    await nextTick()
    expect(onComplete).toHaveBeenCalledOnce()

    app.unmount()
  })
})
