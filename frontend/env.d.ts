/// <reference types="vite/client" />

declare module '*.vue' {
  import type { DefineComponent } from 'vue'
  const component: DefineComponent<Record<string, unknown>, Record<string, unknown>, unknown>
  export default component
}

// 由 Tauri 外壳在页面脚本执行之前注入（见 docs/porting-contract.md）。
interface FsfwRuntime {
  apiBase: string
  token: string
  version: string
  dataDir: string
}

interface Window {
  __FSFW__?: Partial<FsfwRuntime>
}

// 普通浏览器开发时的回落值（`npm run dev`，没有 Tauri 注入）。
interface ImportMetaEnv {
  readonly VITE_FSFW_API_BASE?: string
  readonly VITE_FSFW_TOKEN?: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}
