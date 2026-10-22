# FSFW — fpkstudio for Windows

FSFW 是 [fpkstudio](https://github.com/shuangji66/fpkstudio) 的 Windows 桌面版：在
Windows 10 / 11 上浏览、编辑、打包 fnOS 的 fpk 项目，产出标准 `.msi` 安装包。

它保留了原项目的 Go 后端（项目、文件沙箱、fnpack 调用、打包产物）和 Vue 3 界面，
只替换了「怎么跑起来」这一层：

| | 原版（fnOS） | FSFW（Windows） |
| --- | --- | --- |
| 界面 | Go `//go:embed` + nginx 反代 unix socket | Tauri 2 + 系统 WebView2 |
| 后端 | 只监听 unix socket | 只监听 `127.0.0.1` 随机端口，token 鉴权 |
| 目录授权 | 飞牛开放 API `trim.file.*` | 不需要；当前用户本来就能访问自己选的目录 |
| 数据目录 | 应用数据目录 | `%USERPROFILE%\Documents\fpkstudio` |
| 安装 | fpk 应用包 | 当前用户 MSI（`%LOCALAPPDATA%\fpkstudio`） |
| 常驻 | 服务进程 | 托盘图标，关闭窗口只是隐藏 |

## 功能

- 项目管理：新建（docker / 桌面 UI）、导入 `.fpk`、从本机路径导入、删除、最近修改排序。
- 文件工作区：文件树 / 标签页、文本编辑器（CodeMirror）、manifest / resource / ui
  config / wizard / 环境变量等表单编辑、上传下载、解压、图标制作。
- 打包：随包分发官方 `fnpack.exe`（与 `fsfw.exe` 同目录），支持多架构分别出包、
  把产物复制到导出目录、产物列表与删除。
- 设置：项目目录 / 导出目录可改（默认都在 `Documents\fpkstudio` 下），中英文、深浅色、
  桌面 / 移动布局。
- 日志：后端日志实时流式查看。
- 桌面：托盘菜单（显示主窗口 / 打开数据目录 / 退出）、单实例、关闭到托盘。

## 数据目录

```
%USERPROFILE%\Documents\fpkstudio\
├─ projects\          项目根目录（可在设置里改成任意目录）
├─ output\            导出目录（可在设置里改成任意目录）
├─ logs\
│  ├─ fsfw.log        Go 后端日志
│  └─ shell.log       Tauri 外壳日志（后端的 stdout/stderr）
├─ run\
│  └─ server.json     端口与 token（后端退出时删除）
├─ tmp\               fnpack 的临时目录
└─ state.json         项目目录 / 导出目录设置
```

`Documents` 由 Windows 已知文件夹接口解析，OneDrive 重定向过的机器也会落到正确位置。

项目、导出、日志、设置（`state.json`）都在这里。唯一例外是 WebView2 的浏览器配置目录
（`%LOCALAPPDATA%\com.fpkstudio.fsfw`），里面只放界面偏好（主题 / 语言）和缓存；
放在 LocalAppData 是为了避免把频繁写入的缓存同步到 OneDrive。卸载时会一并删除。
安装目录、快捷方式也都在 `%LOCALAPPDATA%` 下，不写注册表 HKLM，不需要管理员权限。

## 构建

### 环境

- Windows 10 1809+ / Windows 11，x64
- [Node.js](https://nodejs.org/) 22+
- [Go](https://go.dev/) 1.27+
- [Rust](https://rustup.rs/) stable（`x86_64-pc-windows-msvc`）
- Visual Studio Build Tools（「使用 C++ 的桌面开发」）
- WebView2 Runtime（Win11 自带；Win10 若没有，安装包会尝试在线安装）
- WiX Toolset v3 由 Tauri CLI 自动下载，不需要手工安装

### 命令

> MSI 只能在 Windows 上生成（需要 WiX + MSVC 链接器）。Linux / macOS 上可以跑后端测试、
> 前端检查和 `cargo check --target x86_64-pc-windows-msvc`，但出不了安装包。
> **仅生成 amd64 平台**：`package.json` 的 `tauri build` 固定 `--target x86_64-pc-windows-msvc`，
> 构建脚本校验两个 PE 都是 `0x8664`，Rust 侧对非 x86_64 目标直接 `compile_error!`。
> 随包的 `fnpack.exe` 也是 amd64；ARM64 需要另行准备对应版本并重新验证。

```powershell
npm install
npm --prefix frontend install

npm run build
```

产物：

```
src-tauri\target\x86_64-pc-windows-msvc\release\bundle\msi\fpkstudio for Windows_1.0.0_x64_zh-CN.msi
```

`npm run build` 依次做三件事：

1. `scripts/build-backend.ps1`：`GOOS=windows GOARCH=amd64` 编译 Go 后端，并把
   `vendor/fnpack.exe` 一起按 Tauri `externalBin` 约定复制到
   `src-tauri/binaries/`（`fsfw-backend-…exe` / `fnpack-…exe`）；
2. `npm --prefix frontend run build`：Vite 产出 `frontend/dist`；
3. `tauri build --target x86_64-pc-windows-msvc`：编译 Rust 外壳，打 amd64 MSI。

只运行测试：

```powershell
cd backend; go test ./...      # 单元测试依赖 POSIX（/bin/sh、符号链接、FIFO），在 WSL 里跑
npm run typecheck
npm run check
```

> Go 单元测试里的假 fnpack 是 `#!/bin/sh` 脚本，解压测试还会造符号链接 / FIFO；这些在
> Windows 上跑不了。CI 把测试放在 `ubuntu-latest`，Windows runner 只构建 MSI 并做
> 真实安装/卸载冒烟。

### 开发

```powershell
npm run dev        # 先编后端，再 tauri dev（Vite 5273 + 热更新）
```

普通浏览器里调试界面（没有 Tauri 注入、没有原生文件选择器）：

```powershell
# 终端 1
cd backend
go run . --data-dir "$env:TEMP\fsfw-dev" --port 18080 --token ""
# 终端 2
cd frontend
$env:VITE_FSFW_API_BASE="http://127.0.0.1:18080"
npm run dev
```

`--token ""` 会关闭鉴权，只适合本机开发。非 Windows 机器上可用
`scripts/build-backend.sh` 生成对应目标三元组名称的后端，再 `npx tauri dev`。

## 安装与卸载

- 安装包是 **per-user MSI**：装到 `%LOCALAPPDATA%\fpkstudio`，开始菜单与桌面
  快捷方式、控制面板「应用」里的卸载项都在当前用户名下，不需要 UAC。
- 关闭主窗口 = 收进托盘；托盘菜单「退出」才会结束进程（同时停掉 Go 后端）。
- 覆盖安装 / 升级 / 卸载之前，请先从托盘菜单退出：后台进程还在时 Windows Installer
  会提示「文件正在使用」，要求先关掉程序。
- 卸载会删除安装目录、快捷方式、WebView2 缓存，并弹出询问：

  > 是否同时删除用户数据？`Documents\fpkstudio`（项目、导出、日志、设置）

  选「否」保留数据，重装后继续用；选「是」整目录删除。只处理默认数据目录：如果你在设置里
  把项目目录 / 导出目录改到了 `Documents\fpkstudio` 之外，那些目录不会被卸载程序碰。
- 静默卸载不想被询问时，显式传 `CLEANDOCS`：

  ```bat
  msiexec /x "fpkstudio for Windows_1.0.0_x64_zh-CN.msi" /qn CLEANDOCS=0
  msiexec /x "fpkstudio for Windows_1.0.0_x64_zh-CN.msi" /qn CLEANDOCS=1
  ```

  升级安装（覆盖安装新版本）不会触发这个询问，也不会删数据。

## 和原版的差异

- **移除飞牛开放 API**：没有 `trim.file.*` 调用、目录授权、ACL 校验、语义路径显示
  （`/vol1/...` → 「存储空间1/...」）、`@trimjs/web-app` 宿主桥。目录选择改用 Windows
  原生选择器；「在文件管理器里打开」改为调 `explorer.exe`。
- **移除 unix socket / nginx / baseURL**：后端只监听回环 TCP，端口由系统分配，token 由
  Tauri 外壳生成并通过 `window.__FSFW__` 注入。所有接口都必须带 `X-FSFW-Token`
  或 `?token=`，CORS 只放行 Tauri 与本地 Vite dev 的源。
- **移除 `//go:embed` 静态资源**：界面由 Tauri 从 `frontend/dist` 打包加载。
- **路径与平台代码**：Windows 下用 `tar.exe` 处理 tar/zip，`.gz` / `.bz2` 走 Go 标准库；
  `.xz` / `.zst` 需要系统里另装对应工具。解压的成员名校验、符号链接 / 特殊文件清理逻辑
  保持不变。
- **fnpack 随包分发**：`vendor/fnpack.exe`（1.2.3，amd64）会在构建时复制成
  `fnpack.exe`，和 `fsfw.exe` 一起装进安装目录，运行时由外壳通过 `FSFW_FNPACK`
  指给后端，不再依赖 `PATH`。如果用户自己设置了 `FSFW_FNPACK`，以用户的为准。
  只浏览 / 编辑 / 导入 `.fpk` 不需要它。

## 安全模型

- 后端只绑定 `127.0.0.1`（配置层拒绝非回环地址），端口随机。
- 每次启动生成一次性 token；文件权限 `0600` 的 `run/server.json` 只给外壳读。
- 所有 `/api/*` 都校验 token；下载、`<img>`、EventSource 用查询参数携带 token。
- 文件操作仍然走 `os.Root` 沙箱，项目根 / 导出目录之外的路径不可达。
- 前端不放宽 CSP（生产环境无 `unsafe-eval`，`script-src 'self'`）。

## 目录结构

```
backend/                  Go API（模块 fsfw）
├─ main.go                参数解析、依赖装配、父进程看门狗
└─ internal/
   ├─ config/             环境变量 / 命令行配置，数据目录准备
   ├─ fsx/                os.Root 沙箱文件操作与归档解压
   ├─ fnpack/             fnpack 调用（Windows 下不弹控制台）
   ├─ winproc/            CREATE_NO_WINDOW 封装
   ├─ projects/           项目 / 导入 / 打包 / 产物
   ├─ state/              state.json（项目目录 / 导出目录）
   ├─ logging/            文件日志 + 内存环 + SSE
   └─ server/             HTTP 路由、token / CORS、各 API
frontend/                 Vue 3 + Element Plus（Vite）
src-tauri/                Tauri 2 外壳（Rust）
├─ src/backend.rs         后端子进程、runtime 文件轮询、日志
├─ src/main.rs            窗口、托盘、单实例
├─ src/uninstall.rs       卸载时的数据清理模式
├─ wix/main.wxs           per-user MSI 模板（含卸载清理动作）
└─ tauri.conf.json
vendor/fnpack.exe         随包分发的官方 fnpack（amd64，1.2.3）
scripts/                  build-backend.ps1 / build-backend.sh
docs/porting-contract.md  前后端接口契约
.github/workflows/        windows-latest 构建 MSI
```

## 排错

- 启动失败弹窗会带 `Documents\fpkstudio\logs\shell.log` 的末尾。先看这里，再看
  `fsfw.log`。
- 端口 / token 不对：删掉 `run\server.json` 再启动（外壳每次启动也会先删）。
- 托盘图标不见了：Windows 把新图标收进了「隐藏的图标」，在任务栏设置里固定一下。
- 安装时提示需要 WebView2 且下载失败：手动安装 Evergreen Runtime，或改
  `src-tauri/tauri.conf.json` 的 `webviewInstallMode` 为 `offlineInstaller` 重新打包。
- 杀毒软件拦截：MSI 未签名。正式分发请配置 `bundle.windows.certificateThumbprint` /
  `signCommand`。

## 许可

继承原项目 LICENSE（见 `LICENSE`）。
