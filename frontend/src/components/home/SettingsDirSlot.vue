<template>
  <div class="rounded-lg border border-slate-200 p-2.5 dark:border-slate-700">
    <div class="flex flex-wrap items-center gap-2">
      <span class="text-sm font-medium text-slate-700 dark:text-slate-200">
        {{ t(`settings.dirs.${slot}.title`) }}
      </span>
      <el-tag class="text-sm" :type="fromUser ? 'success' : 'info'" effect="plain">
        {{ fromUser ? t('settings.dirs.fromUser') : t('settings.dirs.default') }}
      </el-tag>

      <span class="flex-1" />

      <!--
        按钮是一组：宽度不够时整组下移一行、左对齐，而不是从中间拆开
        （shrink-0 让这一组不会被挤扁，flex 换行才会把它整体放到下一行）。
      -->
      <div class="flex shrink-0 items-center gap-2">
        <el-button
          text
          size="default"
          :icon="FolderOpened"
          :disabled="!canOpenFolder"
          :title="t('settings.dirs.openFolder')"
          @click="emit('openFolder')"
        />
        <el-button size="default" :disabled="pickDisabled" :title="pickTitle" @click="emit('pick')">
          {{ fromUser ? t('settings.dirs.replace') : t('settings.dirs.add') }}
        </el-button>
        <!-- 恢复默认是「回到默认」这种带副作用的动作，给它一个红框（但不填充）区别于旁边的更换按钮 -->
        <el-button
          v-if="fromUser"
          size="default"
          type="danger"
          plain
          class="border-red-400 bg-transparent hover:bg-red-50 disabled:opacity-50 dark:border-red-500/70 dark:hover:bg-red-950/40"
          :disabled="busy"
          @click="emit('clear')"
        >
          {{ t('settings.dirs.clear') }}
        </el-button>
      </div>
    </div>

    <!--
      直接显示绝对路径：Windows 上没有「语义路径」可转换，`C:\Users\...` 本身就是
      用户看得懂的东西。长路径截断，鼠标悬停看编辑提示；点一下打开输入框，可以
      手动粘贴 / 修改（原生选择器不可用时这是唯一的改法）。
      select-text 让路径本身仍然可以拖选复制。
    -->
    <p
      class="mt-1.5 cursor-pointer truncate text-sm text-slate-700 select-text hover:text-sky-600 hover:underline dark:text-slate-200 dark:hover:text-sky-400"
      :title="path"
      :aria-label="t('settings.dirs.editPath')"
      @click="edit"
    >
      {{ path }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { FolderOpened } from '@element-plus/icons-vue'

const props = defineProps<{
  slot: 'projectRoot' | 'outputDir'
  /** 当前生效的绝对路径（配置里存着的那个）。 */
  path: string
  /** 是否来自用户设置（否则就是默认目录）。 */
  fromUser: boolean
  busy: boolean
  /** 选目录按钮是否禁用（普通浏览器开发时没有原生选择器）。 */
  pickDisabled: boolean
  /** 禁用原因，鼠标悬停时显示。 */
  pickTitle: string
  /** 能不能在资源管理器里打开（有路径就行）。 */
  canOpenFolder: boolean
}>()

const emit = defineEmits<{ pick: []; edit: []; clear: []; openFolder: [] }>()

const { t } = useI18n()

/** 空路径（设置还没加载出来）时不弹输入框。 */
function edit() {
  if (props.path) emit('edit')
}
</script>
