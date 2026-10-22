/**
 * 运行时配置。
 *
 * Tauri 外壳在页面脚本执行之前注入 window.__FSFW__（见 docs/porting-contract.md）：
 * 后端是随机端口的本地回环 HTTP API，端口和令牌只有外壳知道。直接用 `npm run dev`
 * 在普通浏览器里开发时没有这层注入，回落到 VITE_FSFW_API_BASE / VITE_FSFW_TOKEN。
 */
interface Injected {
  apiBase?: string
  token?: string
  version?: string
  dataDir?: string
}

const injected: Injected =
  typeof window === 'undefined' ? {} : (window.__FSFW__ ?? {})

/** Vite 环境变量。Node 里跑检查脚本时 import.meta.env 不存在，所以不能直接读属性。 */
const env = (import.meta.env ?? {}) as ImportMetaEnv

/** 去掉尾斜杠；空串表示同源（浏览器开发时由 Vite 代理 / 同源反代接手）。 */
function normalizeApiBase(input: string | undefined): string {
  const raw = (input ?? '').trim()
  return raw.replace(/\/+$/, '')
}

/** 后端 API 根地址，形如 `http://127.0.0.1:51234`。 */
export const apiBase: string = normalizeApiBase(injected.apiBase || env.VITE_FSFW_API_BASE)

/** 兼容旧名字：这里不再是「挂载前缀」，就是 API 根地址。 */
export const baseURL: string = apiBase

/** 后端 API 令牌；为空表示后端没开鉴权（普通浏览器开发）。 */
export const apiToken: string = injected.token || env.VITE_FSFW_TOKEN || ''

/** 外壳注入的版本号，取不到时回落到 dev。 */
export const appVersion: string = injected.version || 'dev'

/** 数据根目录（`Documents\fpkstudio`）；普通浏览器里取不到。 */
export const dataDir: string = injected.dataDir || ''

/** 把接口路径拼成完整 URL。 */
export function apiURL(path: string): string {
  const p = path.startsWith('/') ? path : `/${path}`
  return `${apiBase}${p}`
}
