<template>
  <el-dialog
    v-model="open"
    :title="t('settings.title')"
    class="app-dialog app-dialog-auto"
    modal-class="app-modal"
    width="640px"
    align-center
    lock-scroll
    append-to-body
    @open="onOpen"
  >
    <div class="flex flex-col gap-5">
      <!--
        主题 / 语言：两项并排一行，各自「标题一行、一个按钮一行」。
        每项只有一个循环按钮——点一下切到下一个值，按钮上写当前值。
      -->
      <div class="grid grid-cols-2 gap-3">
        <section class="min-w-0">
          <p class="mb-2 truncate text-sm font-medium text-slate-500 dark:text-slate-400">
            {{ t('settings.theme.title') }}
          </p>
          <el-button size="default" class="w-full" @click="cycleTheme">
            {{ t(`settings.theme.${theme.mode}`) }}
          </el-button>
        </section>

        <section class="min-w-0">
          <p class="mb-2 truncate text-sm font-medium text-slate-500 dark:text-slate-400">
            {{ t('settings.locale.title') }}
          </p>
          <el-button size="default" class="w-full" @click="cycleLocale">
            {{ t(localeLabelKey) }}
          </el-button>
        </section>
      </div>

      <!-- 目录设置 -->
      <section>
        <div class="mb-2 flex items-center gap-2">
          <p class="text-sm font-medium text-slate-500 dark:text-slate-400">{{ t('settings.dirs.title') }}</p>
        </div>

        <div class="flex flex-col gap-3">
          <DirSlot
            slot="projectRoot"
            :path="info?.dirs.projectRoot ?? ''"
            :from-user="info?.dirs.projectRootFromUser ?? false"
            :busy="busy"
            :pick-disabled="busy || !hostAvailable"
            :pick-title="hostAvailable ? '' : t('host.unavailable')"
            :can-open-folder="canOpenFolder(info?.dirs.projectRoot)"
            @pick="pick('projectRoot')"
            @edit="editPath('projectRoot')"
            @clear="clear('projectRoot')"
            @open-folder="openFolder(info?.dirs.projectRoot)"
          />
          <DirSlot
            slot="outputDir"
            :path="info?.dirs.outputDir ?? ''"
            :from-user="info?.dirs.outputDirFromUser ?? false"
            :busy="busy"
            :pick-disabled="busy || !hostAvailable"
            :pick-title="hostAvailable ? '' : t('host.unavailable')"
            :can-open-folder="canOpenFolder(info?.dirs.outputDir)"
            @pick="pick('outputDir')"
            @edit="editPath('outputDir')"
            @clear="clear('outputDir')"
            @open-folder="openFolder(info?.dirs.outputDir)"
          />
        </div>
      </section>

      <!--
        关于：仓库地址是写死的常量，不做成配置——它就是「这个程序是哪个」这个事实，
        没有第二个取值。
      -->
      <section class="border-t border-slate-200 pt-3 dark:border-slate-700">
        <div class="flex items-center gap-2">
          <span class="text-lg font-medium text-slate-700 dark:text-slate-200">{{ t('settings.about.title') }}</span>
          <span
            class="rounded-md bg-slate-100 px-2 py-0.5 text-xs font-medium tabular-nums text-slate-500 dark:bg-slate-800 dark:text-slate-400"
          >
            {{ versionLabel }}
          </span>
          <a
            :href="REPO_URL"
            target="_blank"
            rel="noopener noreferrer"
            class="text-slate-400 transition-colors hover:text-slate-700 dark:text-slate-500 dark:hover:text-slate-200"
            :title="t('settings.about.repo')"
            :aria-label="t('settings.about.repo')"
          >
            <!-- Element Plus 没有 GitHub 图标，这里直接内联官方 mark（16×16） -->
            <svg viewBox="0 0 16 16" width="16" height="16" fill="currentColor" aria-hidden="true">
              <path
                d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82a7.42 7.42 0 0 1 2-.27c.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.01 8.01 0 0 0 16 8c0-4.42-3.58-8-8-8Z"
              />
            </svg>
          </a>
        </div>
        <p class="mt-1.5 text-sm leading-relaxed text-slate-500 dark:text-slate-400">
          {{ t('settings.about.desc') }}
        </p>
        <p class="mt-1.5 text-sm leading-relaxed text-slate-500 dark:text-slate-400">
          {{ t('settings.about.thanks') }}
        </p>
      </section>
    </div>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import DirSlot from './SettingsDirSlot.vue'
import { getAppSettings, saveAppSettings } from '@/api/endpoints'
import type { SettingsInfo } from '@/api/types'
import {
  hostErrorMessage,
  isDesktopHost,
  openInFileManager,
  pickUserDirectory,
} from '@/host'
import { useThemeStore, type ThemeMode } from '@/stores/theme'
import { useLocaleStore } from '@/stores/locale'
import type { LocaleMode } from '@/i18n'
import { appVersion } from '@/runtime'

type Slot = 'projectRoot' | 'outputDir'

/** 主题的循环顺序，文案走 i18n 的 settings.theme.<mode>。 */
const THEME_MODES: ThemeMode[] = ['light', 'dark', 'system']

/** 语言的三档；「跟随系统」是默认值，也是没存过偏好时的落点。 */
const LOCALE_MODES: { value: LocaleMode; labelKey: string }[] = [
  { value: 'zh-CN', labelKey: 'settings.locale.zh' },
  { value: 'en-US', labelKey: 'settings.locale.en' },
  { value: 'system', labelKey: 'settings.locale.system' },
]

/** 仓库地址：关于一节里的图标链到这里。 */
const REPO_URL = 'https://github.com/shuangji66/fsfw'

/** 开发模式（普通浏览器 / tauri dev 未注入版本）显示 dev，打包后显示 v1.0.1。 */
const versionLabel = computed(() => (appVersion === 'dev' ? 'dev' : `v${appVersion}`))

/**
 * 绝对路径：盘符 + 分隔符（`C:\...`），或 UNC（`\\server\share`）。
 *
 * 也放行以 `/` 开头的路径：目标平台是 Windows，但浏览器开发模式跑在 Linux / macOS
 * 上，默认目录本来就是 `/tmp/...`；前端只做格式提示，真正的校验在后端 SetRoots。
 */
const ABSOLUTE_PATH_RE = /^(?:[A-Za-z]:[\\/]|\\|\/)/

const open = defineModel<boolean>({ required: true })

/** 目录设置真的变了才通知外面刷新项目列表（主题变了不用）。 */
const emit = defineEmits<{ saved: [] }>()

const { t } = useI18n()
const theme = useThemeStore()
const language = useLocaleStore()

const info = ref<SettingsInfo | null>(null)
const busy = ref(false)

/** 原生选择器只有桌面外壳里有；普通浏览器里用「点路径手动输入」兜底。 */
const hostAvailable = computed(() => isDesktopHost())

/** 目录能不能在资源管理器里打开：有路径就行（走本地 API）。 */
const canOpenFolder = (path: string | undefined): boolean => Boolean(path)

const currentOf = (slot: Slot): string =>
  slot === 'projectRoot' ? (info.value?.dirs.projectRoot ?? '') : (info.value?.dirs.outputDir ?? '')

const fromUserOf = (slot: Slot): boolean =>
  slot === 'projectRoot'
    ? (info.value?.dirs.projectRootFromUser ?? false)
    : (info.value?.dirs.outputDirFromUser ?? false)

async function openFolder(path: string | undefined) {
  if (!path) return
  try {
    await openInFileManager(path)
  } catch (e) {
    ElMessage.error(hostErrorMessage(e))
  }
}

/** 三档循环：找不到当前值时从头开始（本地偏好被手工改坏也不会卡住）。 */
function nextOf<T>(modes: readonly T[], current: T): T {
  const index = modes.indexOf(current)
  return modes[(index + 1) % modes.length]!
}

function cycleTheme() {
  theme.setMode(nextOf(THEME_MODES, theme.mode))
}

/** 语言按钮上写的是当前那一档的文案（「自动」也要能显示出来）。 */
const localeLabelKey = computed(
  () => LOCALE_MODES.find((item) => item.value === language.mode)?.labelKey ?? 'settings.locale.system',
)

function cycleLocale() {
  const index = LOCALE_MODES.findIndex((item) => item.value === language.mode)
  const next = LOCALE_MODES[(index + 1) % LOCALE_MODES.length]!
  language.setMode(next.value)
}

async function onOpen() {
  await reload()
}

/** 拉一次设置。 */
async function reload() {
  try {
    info.value = await getAppSettings()
  } catch (e) {
    ElMessage.error(hostErrorMessage(e))
  }
}

/**
 * 用原生选择器换目录。
 *
 * 换目录之前先确认（恢复默认 / 换到别处都会让旧目录不再被使用）；用户取消
 * 选择器时给一句「已取消」，不按错误处理。
 */
async function pick(slot: Slot) {
  if (!hostAvailable.value) {
    ElMessage.warning(t('host.unavailable'))
    return
  }

  const current = currentOf(slot)
  if (fromUserOf(slot) && current && !(await confirmReplace(current))) return

  busy.value = true
  try {
    const picked = await pickUserDirectory(t('settings.dirs.pickerTitle'))
    const target = picked[0]
    if (!target) {
      ElMessage.info(t('host.canceled'))
      return
    }
    if (target === current) {
      ElMessage.info(t('settings.dirs.unchanged'))
      return
    }
    await save(slot, target)
  } catch (e) {
    ElMessage.error(hostErrorMessage(e))
  } finally {
    busy.value = false
  }
}

/**
 * 手动改路径。
 *
 * 原生选择器在普通浏览器开发里不可用，而且有时用户就是知道要填哪条路径，
 * 所以点路径本身就能编辑：留空 = 恢复默认目录。
 */
async function editPath(slot: Slot) {
  const current = currentOf(slot)
  let value = ''
  try {
    const res = await ElMessageBox.prompt(
      t('settings.dirs.editPathHint'),
      t('settings.dirs.editPathTitle'),
      {
        inputValue: current,
        confirmButtonText: t('common.save'),
        cancelButtonText: t('common.cancel'),
        inputValidator: (input: string) => {
          const next = input.trim()
          if (!next || ABSOLUTE_PATH_RE.test(next)) return true
          return t('settings.dirs.editPathInvalid')
        },
      },
    )
    value = String(res.value ?? '').trim()
  } catch {
    // 取消 / 关闭弹窗：什么都不做
    return
  }

  try {
    // 清空等于「恢复默认」：和「恢复默认」按钮一样先确认一次。
    if (!value && fromUserOf(slot) && current) await confirmClear(current)
    await save(slot, value)
  } catch (e) {
    if (e === 'cancel' || e === 'close') return
    ElMessage.error(hostErrorMessage(e))
  }
}

/** 恢复默认目录（用户设置过才需要确认）。 */
async function clear(slot: Slot) {
  const current = currentOf(slot)
  const fromUser = fromUserOf(slot)

  try {
    if (fromUser && current) await confirmClear(current)
    await save(slot, '')
  } catch (e) {
    if (e === 'cancel' || e === 'close') return
    ElMessage.error(hostErrorMessage(e))
  }
}

async function save(slot: Slot, path: string) {
  const patch = slot === 'projectRoot' ? { projectRoot: path } : { outputDir: path }
  busy.value = true
  try {
    info.value = await saveAppSettings(patch)
    emit('saved')
    ElMessage.success(t('settings.dirs.saved'))
  } finally {
    busy.value = false
  }
}

async function confirmReplace(path: string): Promise<boolean> {
  try {
    await ElMessageBox.confirm(
      t('settings.dirs.replaceConfirm', { path }),
      t('settings.dirs.replaceTitle'),
      { type: 'warning', confirmButtonText: t('common.confirm'), cancelButtonText: t('common.cancel') },
    )
    return true
  } catch {
    return false
  }
}

async function confirmClear(path: string): Promise<void> {
  await ElMessageBox.confirm(
    t('settings.dirs.clearConfirm', { path }),
    t('settings.dirs.clearTitle'),
    { type: 'warning', confirmButtonText: t('common.confirm'), cancelButtonText: t('common.cancel') },
  )
}
</script>
