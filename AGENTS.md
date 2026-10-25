# AGENTS.md

给在这个仓库里干活的 AI / 自动化助手看的手册。先读这里，再读
[README.md](README.md)（讲设计）与 [docs/porting-contract.md](docs/porting-contract.md)
（前后端接口契约，改接口前必读）。

## 这是什么

FSFW（fpkstudio for Windows）：Tauri 2 外壳 + Go 回环 HTTP API + Vue 3 界面。
原项目是飞牛 fnOS 的 fpk 打包工具（unix socket + nginx + 开放 API），这里已经去掉所有
`trim.*`、目录授权和微应用宿主，改成 Windows 桌面程序。**不要**把 fnOS 时代的假设带回来：
没有 unix socket、没有 `/vol1`、没有 baseURL 子路径、没有 `@trimjs/web-app`。

## 命令

在仓库根目录：

```powershell
npm install                      # Tauri CLI（仅根目录需要）
npm --prefix frontend install    # Vue 前端依赖

npm run build                    # 后端 → 前端 → tauri build（MSI）
npm run dev                      # 后端 → tauri dev
npm run typecheck                # vue-tsc
npm run check                    # 前端 10 个检查脚本
npm run test:backend             # cd backend && go test ./...（POSIX 测试，Windows 上用 WSL）
npm run set-version -- 1.0.1     # 同时改 tauri.conf / Cargo.toml / Cargo.lock
```

单独的步骤：

```powershell
powershell -File scripts/build-backend.ps1     # 只编 Go（Windows 目标）
npm --prefix frontend run build                # 只编前端
npx tauri build                                # 只打 MSI（要求后端、前端产物已就绪）
```

Linux 开发机上编后端用 `scripts/build-backend.sh`（目标三元组跟随本机），
`cargo check --target x86_64-pc-windows-msvc` 能验证 Rust 代码但不产出 MSI。

改完必须验证：

- 动 Go：`cd backend && go test ./...`，并且
  `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./...`；
- 动前端：`npm --prefix frontend run typecheck && npm --prefix frontend run check`；
- 动 Rust / Tauri 配置 / WiX：`cd src-tauri && cargo check --target x86_64-pc-windows-msvc`，
  并检查 `tauri.conf.json` 的 schema（`npx tauri inspect wix-upgrade-code` 之类能读配置的命令）；
- 动安装 / 卸载逻辑：必须在 Windows 上实际装一次、卸一次（见 README「安装与卸载」）。

## 进程模型

```
fsfw.exe (Tauri)
  ├─ 解析 <Documents>\fpkstudio，启动 fsfw-backend.exe --port 0 --token <随机>
  ├─ 轮询 <data>\run\server.json 拿 port/token
  ├─ 建窗口，注入 window.__FSFW__ = {apiBase, token, version, dataDir}
  └─ 托盘常驻；退出时 kill 后端
fsfw-backend.exe (Go)
  ├─ 只监听 127.0.0.1:<随机端口>
  ├─ 所有 /api/* 校验 token（header 或 ?token=），CORS 只放行 Tauri / Vite
  └─ --parent-pid 看门狗：外壳没了就退出，避免孤儿进程
```

**不要**把前端静态资源塞回 Go 的 `//go:embed`，也不要把 API 改成 unix socket——Windows
上没有这套东西，Tauri 已经负责界面与资源。

## 目录

```
backend/
  main.go                  flags / 装配 / 看门狗
  internal/config/         FSFW_* 环境变量 + 命令行覆盖；只允许回环监听
  internal/server/         路由、token、CORS、runtime 文件、/api/system/reveal
  internal/fsx/            os.Root 沙箱；archive.go 里 Windows 用 tar.exe / 标准库
  internal/projects/       项目、导入、打包、产物
  internal/state/          state.json（项目目录 / 导出目录）
  internal/logging/        文件日志 + 内存环 + SSE
frontend/src/
  runtime.ts               window.__FSFW__ / VITE_FSFW_* 回落
  api/                     fetch 封装（自动带 token）、endpoints、types
  host/                    桌面能力：reveal、Tauri 文件选择器、错误文案
  stores/ components/ ...  界面
src-tauri/
  src/main.rs              窗口、托盘、单实例、退出顺序
  src/backend.rs           后端子进程、端口轮询、日志、explorer
  src/uninstall.rs         --uninstall-clean-docs
  src/dialogs.rs           MessageBoxW
  wix/main.wxs             per-user MSI 模板（从 tauri-bundler 2.10.4 的模板改）
  tauri.conf.json          CSP、externalBin、MSI 配置
scripts/build-backend.*    后端构建 + staging vendor/fnpack.exe
vendor/fnpack.exe         随包分发的官方 fnpack（amd64），不要用 src-tauri/binaries 里的副本改它
.github/workflows/         windows-latest 构建 MSI（手动输入版本，无测试/冒烟）
```

## 约定

- 后端：配置一律 `FSFW_` 前缀；路径统一走 `filepath` / `fsx`，不要拼 `"/vol1"`；错误信息
  用中文、能指导用户下一步。新增 API 先更新 `docs/porting-contract.md`。
- 前端：Composition API + `<script setup>` + TS；中英文语言包必须键一致
  （`npm run check:i18n`）；颜色走 Tailwind / Element Plus 变量，别写死浅色。
- Rust：错误给用户看的走 `dialogs::error`；子进程一律 `CREATE_NO_WINDOW`；不要 `unwrap`
  文件系统操作，失败要有可读原因。
- WiX：`src-tauri/wix/main.wxs` 是从 `tauri-bundler` 对应版本复制后改的。升级
  `@tauri-apps/cli` 时必须重新对比上游模板（新增的占位符 / WebView2 逻辑会漏），
  否则 MSI 会在 link 阶段报缺变量。当前对齐 `tauri-cli 2.12.4` / `tauri-bundler 2.10.4`。
- **仅 amd64**：`GOARCH=amd64`、Tauri `--target x86_64-pc-windows-msvc`、Rust 的
  `compile_error!` 三处都别放开。随包 fnpack 也是 amd64，换架构必须同时换它。
- **fnpack 随包**：`vendor/fnpack.exe` 是唯一源；`scripts/build-backend.ps1` 复制成
  `src-tauri/binaries/fnpack-x86_64-pc-windows-msvc.exe`，由 Tauri 的 `externalBin`
  装到 `fsfw.exe` 旁边。运行时 `src-tauri/src/backend.rs` 用 `FSFW_FNPACK` 指过去；
  用户已设置该环境变量时不覆盖。子进程（fnpack / tar）一律走 `internal/winproc.Hide`，
  否则会闪控制台窗口。
- **per-user 是硬约束**：`InstallScope="perUser"` + `LocalAppDataFolder`，不要改回
  `perMachine` / Program Files；不要新增需要管理员权限的自定义动作。
- **版本号**：只通过 `node scripts/set-version.mjs <x.y.z>` 改，必须同时更新
  `tauri.conf.json`、`Cargo.toml`、`Cargo.lock`；Go 后端版本由构建脚本从
  `tauri.conf.json` 读取。前端「关于」显示的版本来自 Tauri 注入的
  `window.__FSFW__.version`（即 Cargo 包版本），不要另外写死。
- **安装选项**：`CREATE_DESKTOP_SHORTCUT` / `LAUNCH_APP` 默认 `1`，由 `OptionsDlg`
  的两个 CheckBox 控制。桌面快捷方式靠组件 `<Condition>` 决定是否安装；「安装后打开」
  靠 `ExitDialog` 的 `DoAction` 条件。安装器文案在 `wix/locale-zh-CN.wxl`，
  **WXS 必须保持 ASCII**，中文只放 WXL；改 WXL 路径时同步
  `tauri.conf.json` 的 `bundle.windows.wix.language`。
- 卸载清理：只在 `REMOVE="ALL" AND NOT UPGRADINGPRODUCTCODE` 时执行；升级不能删数据。
  交互式卸载弹询问；静默 / 基本 UI（UILevel < 4）默认保留数据、不弹窗，要删得传
  `CLEANDOCS=1`。

## Windows 上的坑（已经踩过的）

- `os.Getuid`、`/usr/bin/tar`、`unzip`、`syscall.Socketpair` 之类在 Windows 上不存在或
  不可用；归档解压见 `fsx/archive.go` 的平台分支。
- `explorer.exe` 成功也常返回 1，不能拿退出码判断成功；`/select,<path>` 是单个参数。
- Go 后端的 stdout 被重定向到 `logs/shell.log`；不要往 stdout 打印需要用户立刻看到的东西，
  真正的用户提示走 `dialogs` / 前端。
- 托盘 `TrayIcon` 由 Tauri 的 manager 持有，局部变量 drop 不会消失，但不要只 `mem::forget`。
- **HTML5 拖放**：界面里的拖放（导入 fpk、把图片拖进图标编辑器）走的是浏览器 DnD，
  所以主窗口必须 `disable_drag_drop_handler()`（`src-tauri/src/main.rs`）。Tauri 默认
  会注册自己的拖放处理器，wry 随之给 WebView2 调 `SetAllowExternalDrop(false)`，页面
  连 `dragover` / `drop` 都收不到，表现为「拖到窗口上有光标，松手什么也没发生」。
  去掉原生处理器后，掉在拖放区以外的文件会变成 WebView2 的导航目标，所以 `main.ts`
  里还有一层 `dragover` / `drop` 的默认行为拦截，别删。
- 卸载自定义动作在 `InstallValidate` 之后运行，此时安装目录里的文件还在；不要把它挪到
  `RemoveFiles` 之后。
- MSI 组件 GUID：`externalBin` 由 bundler 每次随机生成。改 `externalBin` 列表 / 名称时
  升一个版本号，避免升级安装留下旧文件。
