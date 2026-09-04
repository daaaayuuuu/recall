package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type inputs struct {
	Task          string          `json:"task"`
	FrontendURL   string          `json:"frontendUrl"`
	LoginID       string          `json:"loginId"`
	CreatorPass   string          `json:"creatorPass"`
	ShareURL      string          `json:"shareUrl"`
	AdminUsername string          `json:"adminUsername"`
	AdminPass     string          `json:"adminPass"`
	Forbidden     json.RawMessage `json:"forbidden"`
}

func main() {
	if len(os.Args) != 2 || (os.Args[1] != "flow" && os.Args[1] != "cleanup") {
		fatal("browser harness mode must be flow or cleanup")
	}
	values := inputs{
		Task: os.Getenv("ANALYTICS_E2E_BROWSER_TASK"), FrontendURL: os.Getenv("ANALYTICS_E2E_FRONTEND_URL"),
		LoginID: os.Getenv("ANALYTICS_E2E_LOGIN_ID"), CreatorPass: os.Getenv("ANALYTICS_E2E_CREATOR_PASSWORD"),
		ShareURL:      os.Getenv("ANALYTICS_E2E_SHARE_URL"),
		AdminUsername: os.Getenv("ANALYTICS_E2E_ADMIN_USERNAME"), AdminPass: os.Getenv("ANALYTICS_E2E_ADMIN_PASSWORD"),
		Forbidden: json.RawMessage(os.Getenv("ANALYTICS_E2E_BROWSER_FORBIDDEN")),
	}
	if strings.TrimSpace(values.Task) == "" {
		fatal("browser harness task is missing")
	}
	if os.Args[1] == "cleanup" {
		runEgo(fmt.Sprintf(`
const result = await completeTaskSpace(%s, { keep: false })
if (!result.done) throw new Error('browser task-space cleanup was skipped')
cliLog('BROWSER_TASK_CLEANUP_PASS')
`, quote(values.Task)))
		return
	}
	if values.FrontendURL == "" || values.LoginID == "" || values.CreatorPass == "" ||
		values.ShareURL == "" || values.AdminUsername == "" || values.AdminPass == "" || !json.Valid(values.Forbidden) {
		fatal("browser harness private environment is incomplete")
	}
	encoded, err := json.Marshal(values)
	if err != nil {
		fatal("browser harness could not encode private input")
	}
	runEgo(fmt.Sprintf(`
const input = %s
await useOrCreateTaskSpace(input.task)
const completeClosingPuzzle = async (expectedPieces, stageLabel, nextStageSelector) => {
  await waitForElement('.puzzle-piece--loose', { timeout: 10 })
  const pieceCount = await js("Number(document.querySelector('.puzzle-board')?.dataset.pieceCount)")
  if (pieceCount !== expectedPieces) {
    throw new Error(stageLabel + ' puzzle expected ' + expectedPieces + ' pieces, got ' + pieceCount)
  }
  for (let piece = 0; piece < expectedPieces; piece += 1) {
    await waitForElement('.puzzle-piece--loose', { timeout: 10 })
    const looseBefore = await js("document.querySelectorAll('.puzzle-piece--loose').length")
    const keyDispatched = await js("(() => { const piece = document.querySelector('.puzzle-piece--loose'); if (!piece) return false; piece.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true })); return true })()")
    if (!keyDispatched) throw new Error('could not activate ' + stageLabel + ' puzzle piece')
    await wait(0.05)
    const looseAfter = await js("document.querySelectorAll('.puzzle-piece--loose').length")
    if (looseAfter !== looseBefore - 1) {
      throw new Error(stageLabel + ' puzzle piece did not move: before=' + looseBefore + ' after=' + looseAfter)
    }
  }
  const nextStageReady = await waitForElement(nextStageSelector, { timeout: 10 })
  if (!nextStageReady) throw new Error(stageLabel + ' puzzle did not auto-advance')
}
await openOrReuseTab(input.frontendUrl + '/auth/login', { wait: true, timeout: 20 })
await waitForElement('input[autocomplete="username"]', { timeout: 10 })
await fillInput('input[autocomplete="username"]', input.loginId)
await fillInput('input[autocomplete="current-password"]', input.creatorPass)
await click('button[type="submit"]', { label: 'login creator account' })
await waitForElement('a[href="/app/games"]', { timeout: 10 })

await gotoAndWait(input.frontendUrl + '/app/create', { timeout: 20, settle: 0.2 })
await gotoAndWait(input.frontendUrl + '/app/games', { timeout: 20, settle: 0.2 })
await waitForNetworkIdle({ timeout: 10 })

await gotoAndWait(input.shareUrl, { timeout: 20, settle: 0.2 })
await waitForElement('xpath=//button[contains(., "开始游戏")]', { timeout: 10 })
await click('xpath=//button[contains(., "开始游戏")]', { label: 'start shared game' })

for (let round = 0; round < 3; round += 1) {
  await waitForElement('button[aria-label^="选择 "]:not(:disabled)', { timeout: 10 })
  await click('button[aria-label^="选择 "]:not(:disabled)', { label: 'choose first-meeting emoji' })
  await waitForElement('xpath=//button[normalize-space(.)="发送" and not(@disabled)]', { timeout: 10 })
  await click('xpath=//button[normalize-space(.)="发送" and not(@disabled)]', { label: 'send first-meeting emoji' })
  await wait(0.6)
}
await completeClosingPuzzle(5, 'first-meeting', '.dining-scene')

for (let food = 0; food < 6; food += 1) {
  await waitForElement('button[aria-label^="食物 "]:not(:disabled)', { timeout: 10 })
  await click('button[aria-label^="食物 "]:not(:disabled)', { label: 'finish dining food' })
}
await completeClosingPuzzle(4, 'dining', '.movie-scene')

for (let approach = 0; approach < 3; approach += 1) {
  await waitForElement('.movie-hands__drag-target[aria-disabled="false"]', { timeout: 10 })
  const approached = await js("(() => { const target = document.querySelector('.movie-hands__drag-target[aria-disabled=\"false\"]'); if (!target) return false; target.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowRight', bubbles: true, cancelable: true })); return true })()")
  if (!approached) throw new Error('could not advance movie relationship')
  await wait(0.6)
}
await completeClosingPuzzle(3, 'movie', '.travel-scene')

for (const item of ['camera', 'hat', 'ticket', 'charger']) {
  await waitForElement('button[data-travel-item="' + item + '"]:not(:disabled)', { timeout: 10 })
  const packed = await js("(() => { const button = document.querySelector('button[data-travel-item=\"" + item + "\"]:not(:disabled)'); if (!button) return false; button.click(); return true })()")
  if (!packed) throw new Error('could not pack travel item ' + item)
  await wait(0.1)
}
await waitForElement('.travel-scene[data-phase="ready-to-close"] .travel-suitcase:not(:disabled)', { timeout: 10 })
const suitcaseClosed = await js("(() => { const button = document.querySelector('.travel-scene[data-phase=\"ready-to-close\"] .travel-suitcase:not(:disabled)'); if (!button) return false; button.click(); return true })()")
if (!suitcaseClosed) throw new Error('could not close travel suitcase')
await completeClosingPuzzle(2, 'travel', '.password-scene')

for (const digit of ['2', '5', '8', '0']) {
  await waitForElement('button[aria-label="数字 ' + digit + '"]:not(:disabled)', { timeout: 10 })
  await click('button[aria-label="数字 ' + digit + '"]:not(:disabled)', { label: 'enter journey password digit' })
}
await waitForElement('.envelope-scene', { timeout: 10 })
for (let reveal = 0; reveal < 10; reveal += 1) {
  const readyToComplete = await js("Boolean(document.querySelector('[data-testid=journey-complete]'))")
  if (readyToComplete) break
  await waitForElement('.envelope-pull-card:not(:disabled)', { timeout: 10 })
  await click('.envelope-pull-card:not(:disabled)', { label: 'reveal envelope item' })
  await wait(0.6)
}
await waitForElement('[data-testid="journey-complete"]', { timeout: 10 })
const completedJourney = await js("(() => { const button = document.querySelector('[data-testid=journey-complete]'); if (!button) return false; button.click(); return true })()")
if (!completedJourney) throw new Error('could not complete shared game journey')
await waitForElement('xpath=//button[contains(., "再玩一次")]', { timeout: 10 })
await click('xpath=//button[contains(., "再玩一次")]', { label: 'replay shared game' })

await gotoAndWait(input.frontendUrl + '/admin/login', { timeout: 20, settle: 0.2 })
await waitForElement('input[autocomplete="username"]', { timeout: 10 })
await fillInput('input[autocomplete="username"]', input.adminUsername)
await fillInput('input[autocomplete="current-password"]', input.adminPass)
await click('button[type="submit"]', { label: 'login admin account' })
await waitForElement('a[href="/admin/behavior-events"]', { timeout: 10 })
await gotoAndWait(input.frontendUrl + '/admin/invitation-codes', { timeout: 20, settle: 0.2 })
await waitForElement('xpath=//button[contains(., "生成邀请码")]', { timeout: 10 })
await click('xpath=//button[contains(., "生成邀请码")]', { label: 'generate registration invitation' })
await waitForElement('[data-testid="generated-invitation-code"]', { timeout: 10 })
const generatedInvitation = await js(String.raw`+"`"+`document.querySelector('[data-testid="generated-invitation-code"]')?.textContent?.trim()`+"`"+`)
if (!/^[2-9A-HJ-NP-Z]{4}-[2-9A-HJ-NP-Z]{4}$/.test(generatedInvitation || '')) {
  throw new Error('administrator UI generated an invalid invitation format')
}
await gotoAndWait(input.frontendUrl + '/admin/behavior-events', { timeout: 20, settle: 0.2 })
await waitForElement('.behavior-event-card', { timeout: 10 })
await waitForNetworkIdle({ timeout: 10 })

const applicationCookieNames = new Set(['creator_session', 'admin_session', 'play_session'])
const browserCookies = ((await cdp('Network.getAllCookies')).cookies || [])
  .filter((cookie) => applicationCookieNames.has(cookie.name) &&
    (cookie.domain === '127.0.0.1' || cookie.domain === 'localhost'))
  .map((cookie) => cookie.value)
const forbiddenValues = [...input.forbidden, generatedInvitation, ...browserCookies]
const wanted = [
  'creator.registered', 'creator.logged_in', 'creator.page_viewed', 'game.created',
  'game.version_created', 'asset.uploaded', 'generation.submitted', 'generation.succeeded',
  'generation.failed', 'share.created', 'share.opened', 'play.started', 'play.completed', 'play.replayed',
]
const browserExpression = String.raw`+"`"+`(() => {
  const cards = [...document.querySelectorAll('.behavior-event-card')]
  const cardText = cards.map((card) => card.innerText).join('\n')
  const cardHTML = cards.map((card) => card.innerHTML).join('\n')
  const text = document.body.innerText
  const html = document.documentElement.innerHTML
  const wanted = ${JSON.stringify(wanted)}
  const forbidden = ${JSON.stringify(forbiddenValues)}
  const loginId = ${JSON.stringify(input.loginId)}
  return {
    path: location.pathname,
    cardCount: cards.length,
    missing: wanted.filter((name) => !cardText.includes(name)),
    hasLoginId: cardText.includes(loginId),
    clientEventIdVisible: cardText.includes('clientEventId') || cardHTML.includes('clientEventId'),
    forbiddenIndex: forbidden.findIndex((value) => value && (text.includes(value) || html.includes(value))),
  }
})()`+"`"+`
const result = await js(browserExpression)
if (result.missing.length !== 0) throw new Error(
  'admin DOM is missing required event types at ' + result.path + ' cards=' + result.cardCount + ': ' + result.missing.join(','),
)
if (!result.hasLoginId) throw new Error('admin DOM did not render the joined top-level loginId')
if (result.clientEventIdVisible) throw new Error('admin DOM exposed clientEventId')
if (result.forbiddenIndex >= 0) throw new Error(
  'admin DOM exposed privacy canary index=' + result.forbiddenIndex +
  ' browserCookie=' + (result.forbiddenIndex >= input.forbidden.length),
)
cliLog('BROWSER_E2E_PASS')
`, encoded))
}

func quote(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func runEgo(script string) {
	command := exec.Command("ego-browser", "nodejs")
	command.Stdin = strings.NewReader(script)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		os.Exit(1)
	}
}

func fatal(message string) {
	_, _ = fmt.Fprintln(os.Stderr, message)
	os.Exit(2)
}
