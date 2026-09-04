import { apiRequest } from './client'
import type { PlayableGameConfig } from './gameplay'

export type GameStatus = 'draft' | 'queued' | 'generating' | 'ready' | 'failed'

export interface Game {
  id: string
  title: string
  description: string | null
  coverAssetId: string | null
  coverPreviewUrl: string | null
  status: GameStatus
  currentVersionId: string | null
  assetCount: number
  createdAt: string
  updatedAt: string
}

export interface GameVersion {
  id: string
  gameId: string
  versionNumber: number
  status: GameStatus
  memoryText: string
	inputSchemaVersion: number
	sceneInputs: Record<string, string>
  templateId: string
  templateVersion: string
  assetCount: number
  createdAt: string
  updatedAt: string
}

export interface GameAsset {
  id: string
  role: 'source' | 'cover'
	slotKey: string
  mimeType: string
  sizeBytes: number
  width: number
  height: number
  sortOrder: number
  previewUrl: string
  createdAt: string
}

export interface TemplateTextInput {
	key: string
	label: string
	placeholder?: string
	helpText?: string
	inputType?: 'textarea' | 'text' | 'password'
	required: boolean
	minLength?: number
	maxLength: number
	format?: 'four-digit-code'
}

export interface TemplateAssetSlot {
	key: string
	label: string
	helpText?: string
	required: boolean
	minItems: number
	maxItems: number
	sortable: boolean
}

export interface TemplateScene {
	key: string
	name: string
	summary: string
	textInputs: TemplateTextInput[]
	assetSlots: TemplateAssetSlot[]
}

export interface GameTemplate {
	id: string
	version: string
	name: string
	description: string
	inputSchemaVersion: number
	generationEnabled: boolean
	cover: TemplateAssetSlot
	scenes: TemplateScene[]
}

export interface CreatorPreview extends PlayableGameConfig {
  game: { id: string; title: string }
  version: { id: string; versionNumber: number }
}

export function createGame(input: {
	title: string
	description: string
	templateId: string
	templateVersion: string
	sceneInputs: Record<string, string>
}, csrfToken: string) {
  return apiRequest<{ game: Game; version: GameVersion }>('/games', {
    method: 'POST',
    headers: { 'X-CSRF-Token': csrfToken },
    body: JSON.stringify(input),
  })
}

export function listTemplates() {
	return apiRequest<{ items: GameTemplate[]; maxSourceImageBytes: number }>('/templates')
}

export function polishLoveLetter(text: string, csrfToken: string) {
	return apiRequest<{ polishedText: string; skipped: boolean }>('/ai/love-letter/polish', {
		method: 'POST',
		headers: { 'X-CSRF-Token': csrfToken },
		body: JSON.stringify({ text }),
	})
}

export function listGames() {
  return apiRequest<{ items: Game[] }>('/games')
}

export function getGame(gameId: string) {
  return apiRequest<Game>(`/games/${gameId}`)
}

export function updateGame(gameId: string, input: { title: string; description: string }, csrfToken: string) {
  return apiRequest<Game>(`/games/${gameId}`, {
    method: 'PATCH',
    headers: { 'X-CSRF-Token': csrfToken },
    body: JSON.stringify(input),
  })
}

export function deleteGame(gameId: string, csrfToken: string) {
  return apiRequest<{ deletionJobId: string; message: string }>(`/games/${gameId}`, {
    method: 'DELETE',
    headers: { 'X-CSRF-Token': csrfToken },
  })
}

export function createVersion(gameId: string, sceneInputs: Record<string, string>, csrfToken: string) {
  return apiRequest<GameVersion>(`/games/${gameId}/versions`, {
    method: 'POST',
    headers: { 'X-CSRF-Token': csrfToken },
		body: JSON.stringify({ sceneInputs }),
  })
}

export function listVersions(gameId: string) {
  return apiRequest<{ items: GameVersion[] }>(`/games/${gameId}/versions`)
}

export function listAssets(gameId: string, versionId: string) {
  return apiRequest<{ items: GameAsset[] }>(`/games/${gameId}/versions/${versionId}/assets`)
}

export function getGamePreview(gameId: string, versionId: string) {
  return apiRequest<CreatorPreview>(`/games/${gameId}/versions/${versionId}/preview`)
}

export function uploadAsset(
  gameId: string,
  versionId: string,
  file: File,
	slotKey: string,
  sortOrder: number,
  csrfToken: string,
) {
  const body = new FormData()
  body.append('file', file)
	body.append('slotKey', slotKey)
  body.append('sortOrder', String(sortOrder))
  return apiRequest<GameAsset>(`/games/${gameId}/versions/${versionId}/assets`, {
    method: 'POST',
    headers: { 'X-CSRF-Token': csrfToken },
    body,
  })
}

export function reorderAssets(
	gameId: string,
	versionId: string,
	slotKey: string,
	assetIds: string[],
	csrfToken: string,
) {
	return apiRequest<void>(`/games/${gameId}/versions/${versionId}/assets/order`, {
		method: 'PATCH',
		headers: { 'X-CSRF-Token': csrfToken },
		body: JSON.stringify({ slotKey, assetIds }),
	})
}

export function deleteAsset(gameId: string, versionId: string, assetId: string, csrfToken: string) {
  return apiRequest<void>(`/games/${gameId}/versions/${versionId}/assets/${assetId}`, {
    method: 'DELETE',
    headers: { 'X-CSRF-Token': csrfToken },
  })
}
