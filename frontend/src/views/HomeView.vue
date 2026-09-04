<script setup lang="ts">
import { ElMessage } from 'element-plus'
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'

import {
  createGame,
  createVersion,
  deleteAsset,
  getGame,
  listAssets,
  listTemplates,
  listVersions,
  polishLoveLetter,
  updateGame,
  uploadAsset,
  type GameAsset,
  type GameTemplate,
  type TemplateAssetSlot,
  type TemplateTextInput,
} from '@/api/games'
import { APIError } from '@/api/client'
import { submitGeneration } from '@/api/generation'
import GenerationProgressScreen from '@/components/GenerationProgressScreen.vue'
import { useAuthStore } from '@/stores/auth'

type CreationStep = 1 | 2 | 3

const props = defineProps<{ gameId?: string }>()
const authStore = useAuthStore()
const router = useRouter()
const loading = ref(false)
const polishing = ref(false)
const templateLoading = ref(true)
const currentStep = ref<CreationStep>(1)
const templates = ref<GameTemplate[]>([])
const selectedTemplateKey = ref('')
const form = reactive({ title: '', description: '', sceneInputs: {} as Record<string, string> })
const selectedFiles = reactive<Record<string, Array<globalThis.File | undefined>>>({})
const selectedFilePreviews = reactive<Record<string, Array<string | undefined>>>({})
const originalAssets = reactive<Record<string, Array<GameAsset | undefined>>>({})
const existingAssets = reactive<Record<string, Array<GameAsset | undefined>>>({})
const generationPreparing = ref(false)
const editing = computed(() => Boolean(props.gameId))

const flowSteps = [
  {
    number: 1 as const,
    eyebrow: '做一份只属于你们的礼物',
    title: '先从你们两个开始',
    description: '填写一次，留刻会把这些素材放进完整故事里。',
  },
  {
    number: 2 as const,
    eyebrow: '把回忆放进信里',
    title: '选择照片，写下想说的话',
    description: '挑选共同经历的照片，再写下只想让对方读到的心意。',
  },
  {
    number: 3 as const,
    eyebrow: '给心意留一把钥匙',
    title: '设置拆信密码',
    description: '密码不必太难，最好是只有你们知道的数字。',
  },
]

const selectedTemplate = computed(() => {
  return templates.value.find((item) => `${item.id}@${item.version}` === selectedTemplateKey.value) ?? null
})
const textInputs = computed<TemplateTextInput[]>(() => {
  return selectedTemplate.value?.scenes.flatMap((scene) => scene.textInputs) ?? []
})
const coverSlot = computed<TemplateAssetSlot | null>(() => selectedTemplate.value?.cover ?? null)
const memorySlots = computed<TemplateAssetSlot[]>(() => {
  return selectedTemplate.value?.scenes.flatMap((scene) => scene.assetSlots) ?? []
})
const assetSlots = computed<TemplateAssetSlot[]>(() => {
  return coverSlot.value ? [coverSlot.value, ...memorySlots.value] : memorySlots.value
})
const letterInputs = computed<TemplateTextInput[]>(() => {
  return textInputs.value.filter((input) => input.inputType !== 'password' && input.key !== 'passwordHint')
})
const passwordInput = computed<TemplateTextInput | null>(() => {
  return textInputs.value.find((input) => input.key === 'letterPassword')
    ?? textInputs.value.find((input) => input.inputType === 'password')
    ?? null
})
const passwordHintInput = computed<TemplateTextInput | null>(() => {
  return textInputs.value.find((input) => input.key === 'passwordHint') ?? null
})
const otherSecurityInputs = computed<TemplateTextInput[]>(() => {
  return textInputs.value.filter((input) => (
    input !== passwordInput.value
    && input !== passwordHintInput.value
    && !letterInputs.value.includes(input)
  ))
})
const activeStep = computed(() => flowSteps[currentStep.value - 1])
const selfieCount = computed(() => coverSlot.value ? selectedFileCount(coverSlot.value.key) : 0)
const memoryPhotoCount = computed(() => memorySlots.value.reduce((total, slot) => {
  return total + selectedFileCount(slot.key)
}, 0))
const hasLoveLetter = computed(() => Boolean(form.sceneInputs.loveLetter?.trim()))

onMounted(initializeFlow)

async function initializeFlow() {
  try {
    templates.value = (await listTemplates()).items
    if (props.gameId) {
      const [game, versions] = await Promise.all([
        getGame(props.gameId),
        listVersions(props.gameId),
      ])
      const version = versions.items.find((item) => item.id === game.currentVersionId)
        ?? versions.items[0]
      if (!version) throw new Error('游戏没有可修改的版本')

      selectedTemplateKey.value = `${version.templateId}@${version.templateVersion}`
      if (!selectedTemplate.value) throw new Error('当前游戏模板已经不可用')
      form.title = game.title
      form.description = game.description ?? ''
      form.sceneInputs = { ...version.sceneInputs }
      hydrateExistingAssets((await listAssets(props.gameId, version.id)).items)
    } else {
      const first = templates.value[0]
      if (first) selectedTemplateKey.value = `${first.id}@${first.version}`
    }
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : (editing.value ? '游戏内容加载失败' : '游戏模板加载失败'))
  } finally {
    templateLoading.value = false
  }
}

function hydrateExistingAssets(assets: GameAsset[]) {
  for (const asset of assets) {
    const originals = [...(originalAssets[asset.slotKey] ?? [])]
    originals[asset.sortOrder] = asset
    originalAssets[asset.slotKey] = originals

    const existing = [...(existingAssets[asset.slotKey] ?? [])]
    existing[asset.sortOrder] = asset
    existingAssets[asset.slotKey] = existing

    const previews = [...(selectedFilePreviews[asset.slotKey] ?? [])]
    previews[asset.sortOrder] = asset.previewUrl
    selectedFilePreviews[asset.slotKey] = previews
  }
}

onBeforeUnmount(() => {
  for (const previews of Object.values(selectedFilePreviews)) {
    previews.forEach(revokePreview)
  }
})

function createPreview(file: globalThis.File) {
  if (typeof globalThis.URL.createObjectURL !== 'function') return undefined
  return globalThis.URL.createObjectURL(file)
}

function revokePreview(preview?: string) {
  if (preview?.startsWith('blob:') && typeof globalThis.URL.revokeObjectURL === 'function') {
    globalThis.URL.revokeObjectURL(preview)
  }
}

function selectFile(event: globalThis.Event, slot: TemplateAssetSlot, index: number) {
  const input = event.target as globalThis.HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return

  const files = [...(selectedFiles[slot.key] ?? [])]
  files[index] = file
  selectedFiles[slot.key] = files

  const previews = [...(selectedFilePreviews[slot.key] ?? [])]
  revokePreview(previews[index])
  previews[index] = createPreview(file)
  selectedFilePreviews[slot.key] = previews
}

function removeSelectedFile(slotKey: string, index: number) {
  const files = [...(selectedFiles[slotKey] ?? [])]
  files[index] = undefined
  selectedFiles[slotKey] = files

  const previews = [...(selectedFilePreviews[slotKey] ?? [])]
  revokePreview(previews[index])
  previews[index] = undefined
  selectedFilePreviews[slotKey] = previews

  const existing = [...(existingAssets[slotKey] ?? [])]
  existing[index] = undefined
  existingAssets[slotKey] = existing
}

function selectedFileCount(slotKey: string) {
  const files = selectedFiles[slotKey] ?? []
  const existing = existingAssets[slotKey] ?? []
  const length = Math.max(files.length, existing.length)
  let count = 0
  for (let index = 0; index < length; index += 1) {
    if (files[index] || existing[index]) count += 1
  }
  return count
}

function hasSelectedAsset(slotKey: string, index: number) {
  return Boolean(selectedFiles[slotKey]?.[index] || existingAssets[slotKey]?.[index])
}

function normalizePassword(value: string | globalThis.Event) {
  if (!passwordInput.value) return
  const rawValue = typeof value === 'string'
    ? value
    : (value.target as globalThis.HTMLInputElement | null)?.value ?? ''
  form.sceneInputs[passwordInput.value.key] = rawValue.replace(/\D/g, '').slice(0, 4)
}

function validateAssets(slots: TemplateAssetSlot[]) {
  for (const slot of slots) {
    if (slot.required && selectedFileCount(slot.key) < slot.minItems) {
      ElMessage.warning(`请至少选择 ${slot.minItems} 张“${slot.label}”`)
      return false
    }
  }
  return true
}

function validateInputs(inputs: TemplateTextInput[]) {
  for (const input of inputs) {
    const value = form.sceneInputs[input.key]?.trim() ?? ''
    if (input.required && !value) {
      ElMessage.warning(`请填写“${input.label}”`)
      return false
    }
    if (input.format === 'four-digit-code' && !/^\d{4}$/.test(value)) {
      ElMessage.warning('拆信密码必须是 4 位数字')
      return false
    }
  }
  return true
}

function usesCurrentPasswordContract(input: TemplateTextInput) {
  return input.format === 'four-digit-code' && input.minLength === 4 && input.maxLength === 4
}

function validateStep(step: CreationStep) {
  if (step === 1) {
    if (!form.title.trim()) {
      ElMessage.warning('请填写作品名称')
      return false
    }
    return coverSlot.value ? validateAssets([coverSlot.value]) : true
  }
  if (step === 2) return validateAssets(memorySlots.value) && validateInputs(letterInputs.value)
  if (passwordInput.value && !usesCurrentPasswordContract(passwordInput.value)) {
    ElMessage.error('创建服务的密码规则尚未同步，请刷新页面后重试')
    return false
  }
  return validateInputs([
    ...(passwordInput.value ? [passwordInput.value] : []),
    ...(passwordHintInput.value ? [passwordHintInput.value] : []),
    ...otherSecurityInputs.value,
  ])
}

function scrollToFlowTop() {
  if (typeof globalThis.window.scrollTo !== 'function') return
  globalThis.window.scrollTo({ top: 0, behavior: 'smooth' })
}

function goBack() {
  if (currentStep.value === 1) return
  currentStep.value = (currentStep.value - 1) as CreationStep
  scrollToFlowTop()
}

function goToVisitedStep(step: CreationStep) {
  if (step >= currentStep.value) return
  currentStep.value = step
  scrollToFlowTop()
}

function continueFlow() {
  if (!validateStep(currentStep.value) || currentStep.value === 3) return
  currentStep.value = (currentStep.value + 1) as CreationStep
  scrollToFlowTop()
}

async function polishLetter() {
  const text = form.sceneInputs.loveLetter?.trim()
  if (!text) {
    ElMessage.warning('请先填写情书内容')
    return
  }
  polishing.value = true
  try {
    const csrfToken = await authStore.ensureCSRF()
    const result = await polishLoveLetter(text, csrfToken)
    form.sceneInputs.loveLetter = result.polishedText
    if (result.skipped) {
      ElMessage.info('未配置 AI 润色，已保留原文，可继续创建')
    } else {
      ElMessage.success('情书已润色并回填，请确认后再创建')
    }
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : 'AI 润色失败，请稍后再试')
  } finally {
    polishing.value = false
  }
}

async function syncVersionAssets(gameId: string, versionId: string, csrfToken: string) {
  const failures: string[] = []
  for (const slot of assetSlots.value) {
    let slotFailed = false
    if (editing.value) {
      const originals = originalAssets[slot.key] ?? []
      const existing = existingAssets[slot.key] ?? []
      const files = selectedFiles[slot.key] ?? []
      for (const [index, original] of originals.entries()) {
        if (!original || (!files[index] && existing[index]?.id === original.id)) continue
        try {
          await deleteAsset(gameId, versionId, original.id, csrfToken)
        } catch (error) {
          const message = error instanceof Error ? error.message : '原图片更新失败'
          failures.push(`${slot.label}：${message}`)
          slotFailed = true
          break
        }
      }
    }
    if (slotFailed) continue

    const files = selectedFiles[slot.key] ?? []
    for (const [index, file] of files.entries()) {
      if (!file) continue
      try {
        await uploadAsset(gameId, versionId, file, slot.key, index, csrfToken)
      } catch (error) {
        const message = error instanceof Error ? error.message : '图片上传失败'
        failures.push(`${slot.label}：${message}`)
        break
      }
    }
  }
  return [...new Set(failures)]
}

async function submit() {
  const template = selectedTemplate.value
  if (!template) {
    ElMessage.warning('请先选择游戏模板')
    return
  }
  for (const step of [1, 2, 3] as const) {
    if (!validateStep(step)) {
      currentStep.value = step
      scrollToFlowTop()
      return
    }
  }

  loading.value = true
  generationPreparing.value = true
  let targetGameId = props.gameId ?? ''
  let targetVersionId = ''
  try {
    const csrfToken = await authStore.ensureCSRF()
    if (editing.value && props.gameId) {
      await updateGame(props.gameId, {
        title: form.title,
        description: form.description,
      }, csrfToken)
      const version = await createVersion(props.gameId, form.sceneInputs, csrfToken)
      targetVersionId = version.id
    } else {
      const result = await createGame({
        title: form.title,
        description: form.description,
        templateId: template.id,
        templateVersion: template.version,
        sceneInputs: form.sceneInputs,
      }, csrfToken)
      targetGameId = result.game.id
      targetVersionId = result.version.id
    }

    const assetFailures = await syncVersionAssets(targetGameId, targetVersionId, csrfToken)
    if (assetFailures.length > 0) {
      ElMessage.warning(`${editing.value ? '修改草稿已保存' : '草稿已创建'}；${assetFailures.join('；')}。请在修改页面重新上传后继续生成`)
      await router.replace({ name: 'game-edit', params: { gameId: targetGameId } })
      return
    }
    const run = await submitGeneration(
      targetGameId,
      targetVersionId,
      globalThis.crypto.randomUUID(),
      csrfToken,
    )
    await router.replace({
      name: 'generation-progress',
      params: { gameId: targetGameId, runId: run.id },
    })
  } catch (error) {
    let errorMessage: string
    if (error instanceof APIError) {
      const fieldMessage = error.fields['sceneInputs.letterPassword']
        ?? Object.values(error.fields)[0]
      errorMessage = fieldMessage ?? error.message
    } else {
      errorMessage = error instanceof Error ? error.message : (editing.value ? '保存修改失败' : '创建草稿失败')
    }
    ElMessage.error(targetVersionId ? `素材已保存，但生成任务提交失败：${errorMessage}` : errorMessage)
    if (targetVersionId) {
      await router.replace({ name: 'game-edit', params: { gameId: targetGameId } })
    } else {
      generationPreparing.value = false
    }
  } finally {
    loading.value = false
  }
}

function handleSubmit() {
  if (currentStep.value < 3) {
    continueFlow()
    return
  }
  void submit()
}
</script>

<template>
  <GenerationProgressScreen
    v-if="generationPreparing"
    preparing
  />
  <section
    v-else
    class="creation-flow"
  >
    <header class="creation-flow__status">
      <div
        class="creation-progress"
        :aria-label="editing ? '修改内容进度' : '创建内容进度'"
      >
        <button
          v-for="step in flowSteps"
          :key="step.number"
          type="button"
          class="creation-progress__step"
          :class="{
            'creation-progress__step--active': step.number <= currentStep,
            'creation-progress__step--current': step.number === currentStep,
          }"
          :aria-label="`第 ${step.number} 步${step.number === currentStep ? '，当前步骤' : ''}`"
          :aria-current="step.number === currentStep ? 'step' : undefined"
          :disabled="step.number >= currentStep"
          @click="goToVisitedStep(step.number)"
        />
        <strong>{{ editing ? '修改内容' : '创建内容' }}</strong>
      </div>
    </header>

    <el-skeleton
      v-if="templateLoading"
      class="creation-flow__skeleton"
      :rows="8"
      animated
    />

    <template v-else-if="selectedTemplate">
      <div class="creation-flow__intro">
        <button
          v-if="currentStep > 1"
          type="button"
          class="creation-flow__back"
          @click="goBack"
        >
          ← 返回上一步
        </button>
        <p class="eyebrow">
          {{ activeStep.eyebrow }}
        </p>
        <h1>{{ activeStep.title }}</h1>
        <p>{{ activeStep.description }}</p>
      </div>

      <el-form
        class="creation-flow__form"
        label-position="top"
        @submit.prevent="handleSubmit"
      >
        <template v-if="currentStep === 1">
          <section class="creation-flow-card creation-flow-card--title">
            <span class="creation-flow-card__number">01</span>
            <el-form-item
              class="creation-title-field"
              label="作品名称"
              required
            >
              <template #label>
                <span class="creation-title-field__label">
                  作品名称
                  <small>必填</small>
                </span>
              </template>
              <el-input
                v-model="form.title"
                maxlength="120"
                placeholder="例如：我们的第 1095 天"
              />
            </el-form-item>
            <p class="creation-flow-card__note">
              将显示在邀请封面和分享页
            </p>
          </section>

          <section
            v-if="coverSlot"
            class="creation-flow-card creation-flow-card--selfie"
          >
            <span class="creation-flow-card__number">02</span>
            <div class="creation-flow-card__heading">
              <div>
                <h2>双人自拍合照 <small>{{ coverSlot.required ? '必填' : '可选' }}</small></h2>
                <p>{{ coverSlot.helpText }}</p>
              </div>
              <span>上传 {{ coverSlot.maxItems }} 张</span>
            </div>
            <div class="creation-selfie-upload">
              <img
                v-if="selectedFilePreviews[coverSlot.key]?.[0]"
                :src="selectedFilePreviews[coverSlot.key]?.[0]"
                alt="已选择的双人自拍预览"
              >
              <div
                v-else
                class="creation-selfie-upload__placeholder"
              >
                <span
                  class="creation-selfie-upload__faces"
                  aria-hidden="true"
                >
                  <i />
                  <i />
                  <b>+</b>
                </span>
                <strong>上传一张清晰的双人自拍</strong>
                <small>用于识别两个人的脸型、发型和显著特征</small>
              </div>
              <label class="creation-selfie-upload__button">
                {{ hasSelectedAsset(coverSlot.key, 0) ? '重新选择' : '选择照片' }}
                <input
                  type="file"
                  accept="image/jpeg,image/png,image/webp"
                  @change="selectFile($event, coverSlot, 0)"
                >
              </label>
              <button
                v-if="hasSelectedAsset(coverSlot.key, 0)"
                type="button"
                class="creation-upload-remove"
                @click="removeSelectedFile(coverSlot.key, 0)"
              >
                移除照片
              </button>
            </div>
            <p class="creation-flow-card__privacy">
              <span aria-hidden="true">✓</span>
              将用于生成和展示你们的专属故事，仅在这份游戏中使用
            </p>
          </section>
        </template>

        <template v-else-if="currentStep === 2">
          <section
            v-for="slot in memorySlots"
            :key="slot.key"
            class="creation-flow-card creation-flow-card--memories"
          >
            <span class="creation-flow-card__number">03</span>
            <div class="creation-flow-card__heading">
              <div>
                <h2>回忆照片 <small>{{ slot.required ? '必填' : '可选' }}</small></h2>
                <p>{{ slot.helpText }}</p>
              </div>
              <span>0–{{ slot.maxItems }} 张</span>
            </div>
            <div class="creation-memory-grid">
              <div
                v-for="position in slot.maxItems"
                :key="position"
                class="creation-memory-position"
              >
                <label class="creation-memory-tile">
                  <img
                    v-if="selectedFilePreviews[slot.key]?.[position - 1]"
                    :src="selectedFilePreviews[slot.key]?.[position - 1]"
                    :alt="`已选择的回忆照片 ${position}`"
                  >
                  <span v-else>
                    <b aria-hidden="true">＋</b>
                    添加照片 {{ position }}
                  </span>
                  <input
                    type="file"
                    accept="image/jpeg,image/png,image/webp"
                    @change="selectFile($event, slot, position - 1)"
                  >
                </label>
                <span
                  v-if="hasSelectedAsset(slot.key, position - 1)"
                  class="creation-memory-position__index"
                >{{ position }}</span>
                <button
                  v-if="hasSelectedAsset(slot.key, position - 1)"
                  type="button"
                  class="creation-upload-remove"
                  @click="removeSelectedFile(slot.key, position - 1)"
                >
                  移除
                </button>
              </div>
              <div
                v-for="position in Math.max(0, 3 - slot.maxItems)"
                :key="`empty-${position}`"
                class="creation-memory-tile creation-memory-tile--empty"
                aria-hidden="true"
              />
            </div>
            <p class="creation-flow-card__note">
              对方打开情书后，照片会像回忆长卷一样缓缓滚动
            </p>
          </section>

          <template
            v-for="input in letterInputs"
            :key="input.key"
          >
            <section
              v-if="input.key === 'loveLetter'"
              class="creation-flow-card creation-flow-card--letter"
            >
              <span class="creation-flow-card__number">04</span>
              <el-form-item
                :label="input.label"
                :required="input.required"
              >
                <div class="love-letter-field">
                  <el-input
                    v-model="form.sceneInputs[input.key]"
                    type="textarea"
                    :rows="8"
                    :maxlength="input.maxLength"
                    :placeholder="input.placeholder"
                    show-word-limit
                  />
                  <div class="love-letter-field__actions">
                    <span>最多 {{ input.maxLength }} 字，润色后仍可继续修改</span>
                    <el-button
                      type="primary"
                      plain
                      :loading="polishing"
                      :disabled="!form.sceneInputs.loveLetter?.trim()"
                      @click="polishLetter"
                    >
                      ✦ AI 润色
                    </el-button>
                  </div>
                </div>
              </el-form-item>
            </section>
            <section
              v-else
              class="creation-flow-card"
            >
              <el-form-item
                :label="input.label"
                :required="input.required"
              >
                <el-input
                  v-model="form.sceneInputs[input.key]"
                  :maxlength="input.maxLength"
                  :placeholder="input.placeholder"
                />
              </el-form-item>
            </section>
          </template>
        </template>

        <template v-else>
          <section class="creation-flow-card creation-flow-card--password">
            <span class="creation-flow-card__number">05</span>
            <el-form-item
              v-if="passwordInput"
              class="creation-password-field"
              :label="passwordInput.label"
              :required="passwordInput.required"
            >
              <template #label>
                <span class="creation-title-field__label">
                  拆情书的密码
                  <small>必填</small>
                </span>
              </template>
              <div class="creation-password-code">
                <span
                  v-for="index in 4"
                  :key="index"
                  aria-hidden="true"
                >{{ form.sceneInputs[passwordInput.key]?.[index - 1] ?? '' }}</span>
                <input
                  v-model="form.sceneInputs[passwordInput.key]"
                  class="creation-password-code__input"
                  type="text"
                  inputmode="numeric"
                  autocomplete="new-password"
                  aria-label="4 位拆信密码"
                  @input="normalizePassword"
                >
              </div>
              <p class="creation-flow-card__note">
                设置 4 位数字，建议选择一段只有你们熟悉的日期
              </p>
            </el-form-item>
            <el-form-item
              v-if="passwordHintInput"
              :label="passwordHintInput.label"
              :required="passwordHintInput.required"
            >
              <el-input
                v-model="form.sceneInputs[passwordHintInput.key]"
                :maxlength="passwordHintInput.maxLength"
                :placeholder="passwordHintInput.placeholder"
              />
              <p class="creation-flow-card__note">
                给对方一点提示，但不要直接写出密码
              </p>
            </el-form-item>
            <el-form-item
              v-for="input in otherSecurityInputs"
              :key="input.key"
              :label="input.label"
              :required="input.required"
            >
              <el-input
                v-model="form.sceneInputs[input.key]"
                :maxlength="input.maxLength"
                :placeholder="input.placeholder"
              />
            </el-form-item>
          </section>

          <section class="creation-ready-card">
            <span
              class="creation-ready-card__check"
              aria-hidden="true"
            >✓</span>
            <div>
              <strong>素材准备好了</strong>
              <p>生成前还可以返回修改</p>
            </div>
            <dl>
              <div><dt>{{ selfieCount }}</dt><dd>张双人自拍</dd></div>
              <div><dt>{{ memoryPhotoCount }}</dt><dd>张回忆照片</dd></div>
              <div><dt>{{ hasLoveLetter ? 1 : 0 }}</dt><dd>封情书</dd></div>
            </dl>
          </section>
        </template>

        <div
          class="creation-flow__actions"
          :class="{ 'creation-flow__actions--continue': currentStep < 3 }"
        >
          <el-button
            v-if="currentStep < 3"
            class="creation-flow__continue"
            native-type="submit"
            type="primary"
            size="large"
          >
            {{ currentStep === 1 ? '继续向下填写' : '最后一步' }}
            <span
              class="creation-flow__down-icon"
              aria-hidden="true"
            />
          </el-button>
          <el-button
            v-else
            native-type="submit"
            type="primary"
            size="large"
            :loading="loading"
          >
            {{ loading ? (editing ? '正在保存修改…' : '正在提交资料…') : (editing ? '重新生成这份回忆' : '生成我们的回忆') }}
            <span
              v-if="!loading"
              aria-hidden="true"
            >→</span>
          </el-button>
        </div>
        <p
          v-if="currentStep === 3"
          class="creation-flow__submit-note"
        >
          生成预计需要 2–5 分钟，提交后可在游戏页查看进度
        </p>
      </el-form>
    </template>

    <el-empty
      v-else
      description="当前没有可用的游戏模板"
    />
  </section>
</template>
