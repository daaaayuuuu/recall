<script setup lang="ts">
defineProps<{
  number: number
  title: string
}>()

defineEmits<{ complete: [] }>()
</script>

<template>
  <section
    class="journey-scene pending-scene"
    :aria-labelledby="`pending-stage-${number}`"
  >
    <header class="journey-scene__header">
      <p>场景 {{ number }}</p>
      <h1 :id="`pending-stage-${number}`">
        {{ title }}
      </h1>
    </header>

    <div class="pending-scene__body">
      <span
        class="pending-scene__mark"
        aria-hidden="true"
      >···</span>
      <p>这一幕正在制作中</p>
      <span>完整玩法接入后会保留在当前顺序中</span>
    </div>

    <button
      class="journey-primary-button"
      type="button"
      @click="$emit('complete')"
    >
      暂时跳过这一幕
    </button>
  </section>
</template>

<style scoped>
.pending-scene__body {
  display: grid;
  flex: 1;
  place-content: center;
  justify-items: center;
  gap: 10px;
  padding: 40px 12px;
  text-align: center;
}

.pending-scene__body p,
.pending-scene__body span {
  margin: 0;
}

.pending-scene__body p {
  font-size: clamp(24px, 7vw, 32px);
  font-weight: 800;
}

.pending-scene__body > span:last-child {
  max-width: 18em;
  color: #444;
  font-size: 13px;
  line-height: 1.6;
}

.pending-scene__mark {
  font-size: 40px;
  font-weight: 900;
  letter-spacing: 0.2em;
}
</style>
