import { defineStore } from 'pinia'
import { computed } from 'vue'

/**
 * 界面形态。
 *
 * 桌面版固定使用桌面布局：文件树 / 子标签编辑器始终按桌面方式排布，不再提供
 * 「桌面 / 移动 / 自动」开关，也不读取旧的 `fpkstudio.layout` 偏好。
 *
 * 保留这个 store 是因为若干组件仍以 `layout.isMobile` 决定弹窗宽度或表单排布；
 * 固定返回 false 让它们统一走桌面分支，调用方不需要各自写死。
 */
export const useLayoutStore = defineStore('layout', () => {
  const isMobile = computed(() => false)
  return { isMobile }
})
