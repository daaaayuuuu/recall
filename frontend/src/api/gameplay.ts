export interface PlayableAsset {
  key: string
  type: 'image'
  url: string
  mimeType: string
  expiresAt: string
}

export interface PlayableGameConfig {
  templateId: string
  templateVersion: string
  configVersion: number
  config: {
    openingTitle: string
    rounds: unknown[]
    loveLetter?: string
    letterPassword?: string
    passwordHint?: string
  }
  assets: PlayableAsset[]
}
