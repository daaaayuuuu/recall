import { apiRequest } from './client'

export type AICapability = 'text' | 'image_moderation' | 'image_to_image'

export interface TextAISettings {
  enabled: boolean
  provider: string
  baseUrl: string
  model: string
  timeout: string
  maxOutputTokens: number
}

export interface ImageModerationAISettings {
  enabled: boolean
  provider: string
  baseUrl: string
  model: string
  timeout: string
  maxOutputTokens: number
}

export interface ImageToImageAISettings {
  enabled: boolean
  provider: string
  baseUrl: string
  model: string
  quality: 'auto' | 'low' | 'medium' | 'high'
  timeout: string
  maxInputBytes: number
  maxOutputBytes: number
}

export interface AISettingsSnapshot {
  text: TextAISettings
  imageModeration: ImageModerationAISettings
  imageToImage: ImageToImageAISettings
}

export interface APIKeyStatus {
  configured: boolean
  hint?: string
  source: 'environment' | 'admin' | 'none' | string
}

export interface AdminAISettings {
  dynamicEnabled: boolean
  version: number
  source: 'environment' | 'admin'
  settings: AISettingsSnapshot
  apiKeys: {
    text: APIKeyStatus
    imageModeration: APIKeyStatus
    imageToImage: APIKeyStatus
  }
  updatedBy?: string
  updatedAt?: string
}

export interface APIKeyMutation {
  value: string
  clear: boolean
}

export interface APIKeyMutations {
  text: APIKeyMutation
  imageModeration: APIKeyMutation
  imageToImage: APIKeyMutation
}

export function getAdminAISettings() {
  return apiRequest<AdminAISettings>('/admin/ai-settings')
}

export function updateAdminAISettings(
  expectedVersion: number,
  settings: AISettingsSnapshot,
  apiKeys: APIKeyMutations,
  csrfToken: string,
) {
  return apiRequest<AdminAISettings>('/admin/ai-settings', {
    method: 'PUT',
    headers: { 'X-CSRF-Token': csrfToken },
    body: JSON.stringify({ expectedVersion, settings, apiKeys }),
  })
}

export function testAdminAIConnection(
  capability: AICapability,
  settings: AISettingsSnapshot,
  apiKeys: APIKeyMutations,
  csrfToken: string,
) {
  return apiRequest<{ capability: AICapability, latencyMs: number }>('/admin/ai-settings/test', {
    method: 'POST',
    headers: { 'X-CSRF-Token': csrfToken },
    body: JSON.stringify({ capability, settings, apiKeys }),
  })
}
