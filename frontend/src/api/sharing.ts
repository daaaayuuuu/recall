import { apiRequest } from './client'
import type { PlayableAsset, PlayableGameConfig } from './gameplay'

export type ShareStatus = 'active' | 'expired' | 'revoked'

export interface ShareLink {
  id: string
  gameId: string
  gameVersionId: string
  publicId: string
  url: string
  status: ShareStatus
  expiresAt: string
  revokedAt: string | null
  createdAt: string
}

export interface PublicShare {
  creator: { displayName: string }
  share: { expiresAt: string }
  game: { title: string; ready: boolean }
}

export interface PlaySession {
  expiresAt: string
  game: { title: string; templateId: string; templateVersion: string }
}

export type PublicAsset = PlayableAsset

export interface GameConfig extends PlayableGameConfig {
  playSessionExpiresAt: string
}

export function createShareLink(gameId: string, expiresAt: string, csrfToken: string) {
  return apiRequest<ShareLink>(`/games/${gameId}/share-links`, {
    method: 'POST',
    headers: { 'X-CSRF-Token': csrfToken },
    body: JSON.stringify({ expiresAt }),
  })
}

export function resolvePublicShare(publicId: string, secret: string) {
  return apiRequest<PublicShare>(`/public/shares/${publicId}/resolve`, {
    method: 'POST',
    body: JSON.stringify({ secret }),
  })
}

export function createPlaySession(publicId: string, secret: string) {
  return apiRequest<PlaySession>(`/public/shares/${publicId}/play-sessions`, {
    method: 'POST',
    body: JSON.stringify({ secret }),
  })
}

export function getGameConfig() {
  return apiRequest<GameConfig>('/public/play-sessions/current/game-config')
}

export function refreshPublicAssets() {
  return apiRequest<{ assets: PublicAsset[] }>('/public/play-sessions/current/refresh-assets', {
    method: 'POST',
  })
}
