<template>
  <el-dialog
    v-model="visible"
    :title="t('iconTab.title')"
    class="app-dialog app-dialog-auto"
    modal-class="app-modal"
    :width="layout.isMobile ? '95%' : '30rem'"
    align-center
    lock-scroll
    append-to-body
  >
    <!--
      首页没有项目，这个弹窗只做「取景 → 导出 PNG」：图标制作本身和项目里的
      「图标」页共用 IconMaker，两边的取景换算才不会各写一套。

      `pt-1` 是必须的：`.el-dialog__body` 现在是 `overflow-y: auto` 且没有上内边距，
      取景框紧贴 body 上沿时，那圈 1px 的虚线（画在元素边框之外）会被 body 裁掉，
      上边整整一条看不见。项目图标页的容器自带 `py-4`，所以不用补。
    -->
    <IconMaker ref="maker" class="pt-1" @change="onChange" />

    <!--
      底部只留「导出图片」：关窗有标题栏那个 × 和遮罩 / Esc，不用再摆一个「关闭」——
      它和 × 做的是同一件事，还占掉一行视觉重量。
    -->
    <template #footer>
      <el-button
        size="default"
        type="primary"
        :icon="Download"
        :disabled="!hasImage"
        @click="exportImage"
      >
        {{ t('iconTab.exportPng') }}
      </el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { Download } from '@element-plus/icons-vue'
import IconMaker from '@/components/project/IconMaker.vue'
import { saveFileBytes } from '@/api/endpoints'
import { hostErrorMessage, isDesktopHost, pickSavePath } from '@/host'
import { useLayoutStore } from '@/stores/layout'

/** 首页工具条上的「图标」弹窗（和设置一样用 v-model 控制显隐）。 */
const visible = defineModel<boolean>({ required: true })

const { t } = useI18n()
const layout = useLayoutStore()

const maker = ref<InstanceType<typeof IconMaker> | null>(null)
/** 没选图时导出按钮置灰（IconMaker 选到图 / 换图时通知）。 */
const hasImage = ref(false)

function onChange(state: { hasImage: boolean }) {
  hasImage.value = state.hasImage
}

/**
 * 导出到本地。
 *
 * 桌面外壳里弹系统的「另存为」，再把 PNG 字节交给后端写到选定路径（图标是 canvas
 * 生成的，没有可复制的后端路径）。普通浏览器开发没有原生对话框，回落到 object URL
 * + `<a download>`。
 */
async function exportImage() {
  const file = await maker.value?.exportPng()
  if (!file) return

  if (!isDesktopHost()) {
    downloadViaAnchor(file)
    return
  }

  let target: string | null
  try {
    target = await pickSavePath({
      defaultPath: file.name,
      title: t('iconTab.exportPng'),
      accept: ['png'],
    })
  } catch (e) {
    ElMessage.error(hostErrorMessage(e))
    return
  }
  if (!target) return

  try {
    await saveFileBytes(target, await toBase64(file))
    ElMessage.success(t('iconTab.exportSaved', { path: target }))
  } catch (e) {
    ElMessage.error(hostErrorMessage(e))
  }
}

/** 浏览器开发时的回落：object URL + `<a download>`。 */
function downloadViaAnchor(file: File) {
  const url = URL.createObjectURL(file)
  try {
    const a = document.createElement('a')
    a.href = url
    a.download = file.name
    a.rel = 'noopener'
    document.body.appendChild(a)
    a.click()
    a.remove()
  } finally {
    // 点完立刻 revoke 会让部分浏览器来不及取内容，放到下一轮再说。
    window.setTimeout(() => URL.revokeObjectURL(url), 10_000)
  }
}

/**
 * `File` → base64。图标 PNG 很小（几百 KB 以内），分块拼字符串只是为了避免
 * `String.fromCharCode(...bytes)` 的参数个数上限。
 */
async function toBase64(file: File): Promise<string> {
  const bytes = new Uint8Array(await file.arrayBuffer())
  let binary = ''
  const chunk = 0x8000
  for (let i = 0; i < bytes.length; i += chunk) {
    binary += String.fromCharCode(...bytes.subarray(i, i + chunk))
  }
  return btoa(binary)
}
</script>
