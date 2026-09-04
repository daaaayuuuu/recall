import type { Component } from 'vue'

import type { PlayableGameConfig } from '@/api/gameplay'

export type GameRuntimeMode = 'public' | 'creator-preview'

export interface GameTemplateProps {
  gameConfig: PlayableGameConfig
  mode: GameRuntimeMode
  previewSkipRequest?: number
}

export interface GameTemplateDefinition {
  id: string
  version: string
  displayName: string
  load: () => Promise<{ default: Component }>
}

export interface GameStageResult {
  stageId: string
  completedAt: number
  actionCount: number
  metadata?: Record<string, unknown>
}
