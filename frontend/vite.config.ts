import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'

// 产物落到 frontend/dist，由 Tauri 外壳通过自定义协议（tauri://localhost）加载。
// base 固定为 './'：自定义协议下没有可用的站点根，资源必须走相对路径。
export default defineConfig({
  plugins: [vue(), tailwindcss()],
  base: './',
  // Tauri 的 dev 输出自己会打印一遍，别让 Vite 把终端清掉。
  clearScreen: false,
  resolve: {
    // Vite 会把以 / 开头的别名按「项目根目录」解析，写成 /src 即可，
    // 不需要引入 node:url，也就少一个 @types/node 依赖。
    alias: { '@': '/src' },
  },
  server: {
    port: 5273,
    // 端口被占用时直接失败：Tauri 的 devUrl 写死了 5273，静默换端口会白屏。
    strictPort: true,
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    chunkSizeWarningLimit: 1500,
    rollupOptions: {
      output: {
        // Vite 8 底层换成 rolldown，manualChunks 只接受函数形式。
        manualChunks(id: string) {
          if (!id.includes('node_modules')) return undefined
          if (id.includes('element-plus')) return 'element'
          if (id.includes('codemirror') || id.includes('@lezer')) return 'codemirror'
          if (id.includes('xterm')) return 'xterm'
          if (id.includes('vue') || id.includes('pinia')) return 'vendor'
          return undefined
        },
      },
    },
  },
})
