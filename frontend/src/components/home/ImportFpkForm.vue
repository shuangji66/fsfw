<template>
  <div class="panel p-3">
    <div class="mb-2 flex items-center gap-2">
      <span class="text-sm font-medium text-slate-700 dark:text-slate-200">{{ t('home.import.title') }}</span>
      <span class="flex-1" />
      <el-button size="default" text @click="emit('cancel')">{{ t('common.cancel') }}</el-button>
    </div>

    <div
      class="flex cursor-pointer flex-col items-center justify-center gap-2 rounded-lg border-2 border-dashed px-4 py-6 transition-colors"
      :class="dragging ? 'border-sky-400 bg-sky-50 dark:bg-sky-950/40' : 'border-slate-300 dark:border-slate-600 bg-slate-50 dark:bg-slate-900'"
      @click="pick"
      @dragover.prevent="dragging = true"
      @dragleave.prevent="dragging = false"
      @drop.prevent="onDrop"
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
        <!--
          走系统的文件选择器：返回的是本机绝对路径，交给后端按路径导入，
          不经过请求体上传。普通浏览器里没有原生选择器，按钮禁用。
        -->
        <el-button
          size="default"
          type="primary"
          plain
          :icon="FolderOpened"
          :disabled="loading || !hostAvailable"
          :title="hostAvailable ? '' : t('host.unavailable')"
          @click.stop="pickLocalFile"
        >
          {{ t('home.import.pickPath') }}
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
      <!-- 只有「浏览器上传」这一路能取消：原生选择器那条是后端读本机文件，没有请求体可断 -->
      <el-button v-if="cancelable" size="default" @click="cancelUpload">
        {{ t('common.cancel') }}
      </el-button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { FolderOpened, UploadFilled } from '@element-plus/icons-vue'
import { useProjectsStore } from '@/stores/projects'
import { hostErrorMessage, isDesktopHost, pickUserFiles } from '@/host'
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

const hostAvailable = computed(() => isDesktopHost())

function pick() {
  if (!loading.value) input.value?.click()
}

async function onPick(event: Event) {
  const files = (event.target as HTMLInputElement).files
  if (files?.length) await upload(files[0]!)
  if (input.value) input.value.value = ''
}

async function onDrop(event: DragEvent) {
  dragging.value = false
  const file = event.dataTransfer?.files?.[0]
  if (file) await upload(file)
}

/**
 * 「从本机路径导入」：拉起系统文件选择器 → 由后端按绝对路径导入。
 *
 * 选择器返回最多一个路径；空数组表示用户没选就关掉了，提示一句取消，
 * 不按错误处理。普通浏览器里没有原生选择器，按钮本来就是禁用的。
 */
async function pickLocalFile() {
  if (loading.value) return
  loading.value = true
  try {
    const paths = await pickUserFiles(t('home.import.pickerTitle'), ['.fpk', '.tar', '.tgz'])
    const path = paths[0]
    if (!path) {
      ElMessage.info(t('home.import.canceled'))
      return
    }
    const info = await projects.importFpkPath(path)
    emit('done', info)
  } catch (e) {
    ElMessage.error(hostErrorMessage(e))
  } finally {
    loading.value = false
  }
}

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
