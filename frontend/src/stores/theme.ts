import { defineStore } from 'pinia'
import { computed, ref } from 'vue'

/**
 * 主题（浅色 / 深色 / 跟随系统）。
 *
 * 「跟随系统」只看 `prefers-color-scheme`：桌面外壳不接管系统主题，也不再有宿主
 * 主题桥。系统切换时要实时响应，所以挂一条监听。
 *
 * 深色不是换一套色号就完事：Tailwind 的 dark 变体按 html.dark 生效，
 * Element Plus 的暗色变量也认这个类，两边靠同一个类对齐。
 */
export type ThemeMode = 'light' | 'dark' | 'system'

const STORAGE_KEY = 'fpkstudio.theme'

/** 系统浅深色偏好。和「显示方式」一样记在本地，不进后端状态文件。 */
const darkQuery = window.matchMedia('(prefers-color-scheme: dark)')

function readMode(): ThemeMode {
  const saved = window.localStorage.getItem(STORAGE_KEY)
  return saved === 'light' || saved === 'dark' ? saved : 'system'
}

export const useThemeStore = defineStore('theme', () => {
  const mode = ref<ThemeMode>(readMode())
  const systemDark = ref(darkQuery.matches)

  const resolved = computed<'light' | 'dark'>(() => {
    if (mode.value !== 'system') return mode.value
    return systemDark.value ? 'dark' : 'light'
  })

  function apply() {
    const root = document.documentElement
    root.classList.toggle('dark', resolved.value === 'dark')
    root.dataset.theme = resolved.value
    // 让原生控件（滚动条、日期选择、下拉）也跟着切，不然深色页面里会冒出白条。
    root.style.colorScheme = resolved.value
  }

  function setMode(next: ThemeMode) {
    mode.value = next
    window.localStorage.setItem(STORAGE_KEY, next)
    apply()
  }

  /** 挂上监听并应用一次。只该在应用启动时调一次。 */
  function init() {
    darkQuery.addEventListener('change', (event) => {
      systemDark.value = event.matches
      apply()
    })
    apply()
  }

  return { mode, resolved, systemDark, setMode, init, apply }
})
