<template>
  <div class="panel p-3">
    <div class="mb-2 flex items-center gap-2">
      <span class="text-sm font-medium text-slate-700 dark:text-slate-200">{{ t('home.import.title') }}</span>
      <span class="flex-1" />
      <el-button size="default" text @click="emit('cancel')">{{ t('common.cancel') }}</el-button>
    </div>

    <!--
      拖放不挂在这个小方框上：面板开着的时候整窗都是拖放区（见 script 里的窗口级
      监听）。框只有几十像素高，掉偏一点用户看到的就是「拖了没反应」；挂在框和
      window 上还会因为冒泡导致一次拖放导入两次。
    -->
    <div
      class="flex cursor-pointer flex-col items-center justify-center gap-2 rounded-lg border-2 border-dashed px-4 py-6 transition-colors"
      :class="dragging ? 'border-sky-400 bg-sky-50 dark:bg-sky-950/40' : 'border-slate-300 dark:border-slate-600 bg-slate-50 dark:bg-slate-900'"
      @click="pick"
    >
      <el-icon class="text-2xl text-slate-400 dark:text-slate-500"><UploadFilled /></el-icon>
      <p class="text-sm text-slate-600 dark:text-slate-300">
        {{ dragging ? t('home.import.drop') : t('home.import.hint') }}
      </p>
      <div class="flex items-center gap-2">
        <el-button
          size="default"
          :disabled="loading"
          @click.stop="pick"
        >
          {{ t('home.import.pick') }}
        </el-button>
      </div>
      <input ref="input" type="file" accept=".fpk,.tar,.tgz,application/gzip" class="hidden" @change="onPick" />
    </div>

    <p class="mt-2 text-sm leading-relaxed text-slate-500 dark:text-slate-400">
      {{ t('home.import.limitHint') }}
    </p>

    <div v-if="loading" class="mt-3 flex items-center gap-2">
      <el-progress
        class="min-w-0 flex-1"
        :percentage="percent"
        :stroke-width="6"
        :format="() => t('home.import.uploading', { percent })"
      />
      <!-- 上传是请求体，客户端断开时后端会把写了一半的文件删掉 -->
      <el-button v-if="cancelable" size="default" @click="cancelUpload">
        {{ t('common.cancel') }}
      </el-button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { UploadFilled } from '@element-plus/icons-vue'
import { useProjectsStore } from '@/stores/projects'
import type { ProjectInfo } from '@/api/types'

const emit = defineEmits<{
  done: [project: ProjectInfo | undefined]
  cancel: []
}>()

const { t } = useI18n()
const projects = useProjectsStore()

const input = ref<HTMLInputElement | null>(null)
const dragging = ref(false)
const loading = ref(false)
const percent = ref(0)
/** 当前这次上传的取消手柄（只有浏览器上传那一路有）。 */
let controller: AbortController | null = null
/** 正在上传、且这一路支持取消。 */
const cancelable = computed(() => loading.value && controller !== null)

function pick() {
  if (!loading.value) input.value?.click()
}

async function onPick(event: Event) {
  const files = (event.target as HTMLInputElement).files
  if (files?.length) await upload(files[0]!)
  if (input.value) input.value.value = ''
}

/** 这个拖放事件里是不是文件（拖选中的文字、拖动内部元素时 types 里没有 Files）。 */
function hasFiles(event: DragEvent): boolean {
  return Array.from(event.dataTransfer?.types ?? []).includes('Files')
}

// 面板开着时整窗都算拖放区。
//
// 桌面端（WebView2）的文件拖放是由页面自己处理的（见 src-tauri 的
// disable_drag_drop_handler），所以窗口级监听能收到事件；浏览器开发模式下同样成立。
function onWindowDragOver(event: DragEvent) {
  if (!hasFiles(event)) return
  event.preventDefault()
  if (event.dataTransfer) event.dataTransfer.dropEffect = 'copy'
  dragging.value = true
}

function onWindowDragLeave(event: DragEvent) {
  // 在窗口内的元素之间移动也会触发 dragleave，只有真的离开窗口（relatedTarget 为
  // null）才收起高亮。
  if (event.relatedTarget === null) dragging.value = false
}

async function onWindowDrop(event: DragEvent) {
  if (!hasFiles(event)) return
  event.preventDefault()
  dragging.value = false
  if (loading.value) return
  const file = event.dataTransfer?.files?.[0]
  if (file) await upload(file)
}

onMounted(() => {
  window.addEventListener('dragover', onWindowDragOver)
  window.addEventListener('dragleave', onWindowDragLeave)
  window.addEventListener('drop', onWindowDrop)
})

onBeforeUnmount(() => {
  window.removeEventListener('dragover', onWindowDragOver)
  window.removeEventListener('dragleave', onWindowDragLeave)
  window.removeEventListener('drop', onWindowDrop)
})

async function upload(file: File) {
  const ctl = new AbortController()
  controller = ctl
  loading.value = true
  percent.value = 0
  try {
    const info = await projects.importFpk(
      file,
      (p) => {
        percent.value = p
      },
      ctl.signal,
    )
    emit('done', info)
  } catch (e) {
    // 用户自己点的取消不是错误
    if (ctl.signal.aborted) ElMessage.info(t('home.import.canceled'))
    else ElMessage.error(e instanceof Error ? e.message : String(e))
  } finally {
    if (controller === ctl) controller = null
    loading.value = false
    percent.value = 0
  }
}

/** 取消正在进行的导入（后端收到 ctx 取消后会清掉暂存目录）。 */
function cancelUpload() {
  controller?.abort()
}
</script>
