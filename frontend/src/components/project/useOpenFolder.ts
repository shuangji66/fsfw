import { computed } from 'vue'
import { ElMessage } from 'element-plus'
import { useProjectsStore } from '@/stores/projects'
import { hostErrorMessage, openInFileManager } from '@/host'

/**
 * 「在资源管理器里打开项目目录」。
 *
 * 三个入口共用：首页卡片的文件夹按钮（`ProjectActions`）、左栏文件树顶栏、以及「文件」
 * 标签页顶栏。三处各写一份的话，迟早有一处忘了 catch——接口会 reject，表现是
 * 「点了没反应」加一条 unhandled rejection。
 *
 * 传项目名而不是绝对路径：路径只有后端知道（项目根可以被用户换到别处，
 * `projectsDir + name` 是拼不出来的，见 `/api/projects` 的 `absPath`）。项目列表还没加载
 * 出来时按钮置灰。
 */
export function useOpenFolder(project: () => string) {
  const projects = useProjectsStore()

  const folderPath = computed(() => projects.find(project())?.absPath ?? '')

  /** 打开资源管理器现在走本地 API，普通浏览器里也能用；只要求有路径。 */
  const canOpenFolder = computed(() => Boolean(folderPath.value))

  async function openFolder(): Promise<void> {
    if (!folderPath.value) return
    try {
      await openInFileManager(folderPath.value)
    } catch (e) {
      ElMessage.error(hostErrorMessage(e))
    }
  }

  return { canOpenFolder, openFolder }
}
