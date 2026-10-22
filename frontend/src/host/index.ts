/**
 * 桌面宿主能力（替代原来的微应用宿主层）。
 *
 * 两类能力分得很清：
 *  - 「在资源管理器里打开」走后端 `POST /api/system/reveal`，普通浏览器开发时也能用；
 *  - 原生文件选择器只有 Tauri 外壳里有（`@tauri-apps/plugin-dialog`），普通浏览器
 *    里没有这层桥，调用会抛 `HostUnavailableError`，由调用方给一句本地化提示。
 */
import { i18n } from '@/i18n'
import { revealPath } from '@/api/endpoints'

/** 调用了只有桌面外壳才有的能力（普通浏览器开发时）。 */
export class HostUnavailableError extends Error {
  constructor() {
    super('the fsfw desktop host is not available')
    this.name = 'HostUnavailableError'
  }
}

/** 是否跑在 Tauri 外壳里（外壳注入 `__TAURI_INTERNALS__`，普通浏览器没有）。 */
export function isDesktopHost(): boolean {
  return typeof window !== 'undefined' && '__TAURI_INTERNALS__' in window
}

/** 契约里的别名：宿主可用 == 原生能力可用。 */
export const isHostAvailable = isDesktopHost

/**
 * 在资源管理器里打开某个绝对路径。
 *
 * 走本地 API，所以普通浏览器里也照常工作；接口错误保持 `Error` 原样抛出去
 * （调用方用 `hostErrorMessage` 取文案）。
 */
export async function openInFileManager(path: string): Promise<void> {
  await revealPath(path)
}

interface PickOptions {
  directory: boolean
  title: string
  /** 文件扩展名白名单（带不带前导点都行），目录选择时不用。 */
  accept?: string[]
}

/**
 * 拉起系统的文件 / 目录选择器。
 *
 * 插件返回 `string | string[] | null`，这里统一成数组：**空数组表示用户取消**，
 * 不是错误。`multiple: false` 保证最多一个，但类型上仍然可能是数组。
 */
async function pick(options: PickOptions): Promise<string[]> {
  if (!isDesktopHost()) throw new HostUnavailableError()

  // 动态 import：普通浏览器里不会加载 Tauri 的插件代码。
  const { open } = await import('@tauri-apps/plugin-dialog')
  const picked = await open({
    directory: options.directory,
    multiple: false,
    title: options.title,
    filters: options.accept?.length
      ? [
          {
            name: i18n.global.t('host.fileFilter'),
            extensions: options.accept.map((ext) => ext.replace(/^\./, '')),
          },
        ]
      : undefined,
  })

  if (!picked) return []
  return Array.isArray(picked) ? picked : [picked]
}

/** 选一个目录；返回选中的路径（取消时为空数组）。 */
export function pickUserDirectory(title: string): Promise<string[]> {
  return pick({ directory: true, title })
}

/** 选一个文件；`accept` 限定扩展名（如 `['.fpk', '.tar', '.tgz']`）。 */
export function pickUserFiles(title: string, accept: string[]): Promise<string[]> {
  return pick({ directory: false, title, accept })
}

/** 把宿主 / 接口错误翻成给用户看的一句话。 */
export function hostErrorMessage(err: unknown): string {
  if (err instanceof HostUnavailableError) return i18n.global.t('host.unavailable')
  if (err instanceof Error) return err.message
  return i18n.global.t('host.error', { error: String(err) })
}
