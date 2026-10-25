import { createApp } from 'vue'
import { createPinia } from 'pinia'
import ElementPlus from 'element-plus'
import * as ElementPlusIcons from '@element-plus/icons-vue'

import App from './App.vue'
import { router } from './router'
import { currentLocale, i18n, elementOptions } from './i18n'
import { useThemeStore } from './stores/theme'
import { useLocaleStore } from './stores/locale'
import { initViewportAdaptation } from './utils/viewport'
import './style.css'

// index.html 里写死了 lang="zh-CN"，而界面语言是按浏览器选的：不同步的话，英文界面
// 的页面在屏幕阅读器 / 翻译插件看来仍然是中文文档。
document.documentElement.lang = currentLocale

const app = createApp(App)

app.use(createPinia())
app.use(i18n)
app.use(router)
// 这一份是 **reactive** 的（见 i18n/index.ts）：设置里切语言时改它的 locale，
// 组件树和 ElMessage / ElMessageBox 这类命令式 API 会一起换。
app.use(ElementPlus, elementOptions)

// 主题要在挂载之前定下来：挂载后再切会让首屏闪一下浅色。初始化只是读本地偏好 +
// 挂 prefers-color-scheme 监听，不阻塞渲染。
useThemeStore().init()

// 语言偏好要在挂载之前定下来（和主题同理）：读本地存的那门，没存过就跟随系统，
// 顺手把 Element Plus 的语言包和 html lang 对齐。
useLocaleStore()

// 手机上键盘顶起来时把页面缩到键盘以上（写 --kb-height，见 utils/viewport.ts）。
initViewportAdaptation()

// 桌面端 WebView2 把文件拖放交回页面处理（见 src-tauri 的 disable_drag_drop_handler）。
// 代价是「文件掉在拖放区以外」会走浏览器默认动作：WebView2 把那个文件当成导航目标
// （页面被换掉，或者去拉外部程序）。这里统一兜住，文件拖放只由各自的拖放区
// （导入面板、图标编辑器）处理。
function blockDefaultFileDrop(event: DragEvent) {
  if (Array.from(event.dataTransfer?.types ?? []).includes('Files')) event.preventDefault()
}
window.addEventListener('dragover', blockDefaultFileDrop)
window.addEventListener('drop', blockDefaultFileDrop)

// 图标全部注册为全局组件，模板里直接写 <el-icon><Folder /></el-icon>。
for (const [name, component] of Object.entries(ElementPlusIcons)) {
  app.component(name, component)
}

app.mount('#app')
