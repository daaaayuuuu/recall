import { createPinia } from 'pinia'
import { createApp } from 'vue'
import {
  ElAlert,
  ElButton,
  ElDescriptions,
  ElDescriptionsItem,
  ElDialog,
  ElEmpty,
  ElForm,
  ElFormItem,
  ElInput,
  ElResult,
  ElOption,
  ElProgress,
  ElSelect,
  ElTag,
} from 'element-plus'

import 'element-plus/dist/index.css'
import './styles/main.css'

import App from './App.vue'
import router from './router'

const app = createApp(App).use(createPinia()).use(router)

for (const component of [
  ElAlert,
  ElButton,
  ElDescriptions,
  ElDescriptionsItem,
  ElDialog,
  ElEmpty,
  ElForm,
  ElFormItem,
  ElInput,
  ElResult,
  ElOption,
  ElProgress,
  ElSelect,
  ElTag,
]) {
  app.component(component.name!, component)
}

app.mount('#app')
