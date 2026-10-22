# fnpack.exe 被杀软误报的处理

现象：安装完成后 `%LOCALAPPDATA%\fpkstudio\fnpack.exe` 被 Windows Defender（或其它
杀软）隔离，打包功能因此报「找不到 fnpack」或「执行 fnpack 失败」。

先说结论：**没有「改个文件名 / 加壳 / 让安装程序自动加白名单」这种可靠且正当的免杀
手段。** 可持续的做法只有两条：给所有随包二进制做 Authenticode 签名，以及向杀软厂商
提交误报样本。下面按优先级列。

## 0. 先确认文件本身没被替换

随包的 fnpack 是官方 1.2.3 amd64 构建，来自
<https://static2.fnnas.com/fnpack/fnpack-1.2.3-windows-amd64>，`vendor/README.md` 里记了
哈希；我们已把它和官方 URL 下载的文件逐字节比对过（完全一致）：

```powershell
Get-FileHash .\vendor\fnpack.exe -Algorithm SHA256
# 应为 D7AF4BD716B009C58F5BCD931615F39DB121E7D4B75DC759E575C4FB2879B6EE
```

`vendor/fnpack.exe` 必须与官方文件一致；构建脚本每次都会校验，不一致直接失败。

安装后的副本分两种情况：

```powershell
# 未签名的构建：哈希应与官方一致
Get-FileHash "$env:LOCALAPPDATA\fpkstudio\fnpack.exe" -Algorithm SHA256

# 已签名的构建：哈希会变（签名追加在 PE 末尾），改看签名
Get-AuthenticodeSignature "$env:LOCALAPPDATA\fpkstudio\fnpack.exe" | Format-List *
```

- 未签名且哈希不一致：不要加白，先查供应链（构建机、上传的源文件）。
- 未签名且哈希一致：基本可以确定是「未签名 / 低信誉 / Go 二进制启发式」导致的误报。
- 已签名：`Status` 应为 `Valid`，`SignerCertificate.Subject` 是发布者名称；签名有效但
  仍被报，按第 2 节提交误报。
- 可以再上传 VirusTotal 看是只有 Defender 报（典型误报），还是多家报（需要进一步查）。
- 记住 Defender 的**检测名**（例如 `Trojan:Win32/Wacatac.B!ml`）和引擎版本，提交误报要用。

官方 fnpack 本身**没有 Authenticode 签名**（检查 PE security directory 为 0）。这是它
容易被启发式命中的主要原因之一。

## 1. 代码签名（最有效的长期方案）

### 要签哪些文件

MSI 里每一个 PE 都要签，否则杀软仍会盯着没签的那个：

| 文件 | 签名方式 |
| --- | --- |
| `fnpack.exe` | 构建脚本在打包前签（`scripts/sign.ps1`） |
| `fsfw-backend.exe` | 构建脚本在打包前签（`scripts/sign.ps1`） |
| `fsfw.exe` | Tauri 打包时签 |
| `*.msi` | Tauri 打包时签 |

只签 MSI 不签内部 exe 没有用：Defender 解包后会逐个扫描。

### 证书

- **OV 代码签名证书**：能建立发布者身份，SmartScreen 信誉需要时间积累。
- **EV 代码签名证书**：信誉更好、SmartScreen 更快建立；成本更高，通常配硬件令牌/HSM。
- **Azure Trusted Signing**：适合 CI，按签名次数计费，需要完成组织/身份验证。
- 证书必须带**时间戳**（RFC 3161），否则证书过期后旧安装包签名失效。
- 所有版本用同一张证书、同一发布者名称，不要频繁换。

### 在 FSFW 里启用

1. 构建前设置证书环境变量（二选一）：

   ```powershell
   # 证书已导入 CurrentUser\My
   $env:FSFW_SIGN_CERT_SHA1 = "0123456789ABCDEF..."

   # 或者用 PFX
   $env:FSFW_SIGN_PFX = "C:\keys\codesign.pfx"
   $env:FSFW_SIGN_PFX_PASSWORD = "..."
   ```

2. `scripts/build-backend.ps1` 会自动调用 `scripts/sign.ps1`，签好
   `fsfw-backend.exe` 和 `fnpack.exe`，并执行 `signtool verify /pa`。

3. 让 Tauri 签 `fsfw.exe` 和 MSI：在 `src-tauri/tauri.conf.json` 的
   `bundle.windows` 下加：

   ```json
   "certificateThumbprint": "0123456789ABCDEF...",
   "digestAlgorithm": "sha256",
   "timestampUrl": "http://timestamp.digicert.com"
   ```

   Tauri 会用同一张证书签主程序和安装包。

4. 构建完成后逐个复核：

   ```powershell
   signtool verify /pa /v "$env:LOCALAPPDATA\fpkstudio\fnpack.exe"
   signtool verify /pa /v "src-tauri\target\x86_64-pc-windows-msvc\release\bundle\msi\*.msi"
   ```

### 关于签第三方 fnpack

用我们的证书签官方 fnpack，等于声明「这个文件由本发布者分发并对完整性负责」。这在
再分发场景很常见，但如果 fnOS 官方有分发条款限制，最稳妥的是**请官方出一个已签名的
fnpack.exe**，我们只做哈希校验和随包复制。无论谁签，签名只能减少启发式误报，不能
保证 100% 不报。

## 2. 向杀软厂商提交误报

签名之后仍可能被某个引擎报（新哈希没有信誉）。提交时提供完整信息：

- 产品名：fpkstudio for Windows (FSFW)
- 文件：`fnpack.exe`，官方 fnOS fnpack 1.2.3 amd64
- SHA-256 与文件大小
- 检测名、Defender 引擎版本、Windows 版本
- 说明这是官方命令行工具，随桌面应用分发，用于打包 fpk
- 附上下载来源：<https://developer.fnnas.com/docs/cli/fnpack/> /
  <https://static2.fnnas.com/fnpack/fnpack-1.2.3-windows-amd64>，以及与官方文件逐字节
  一致的说明

入口：

- Microsoft：<https://www.microsoft.com/en-us/wdsi/filesubmission>
  选择「软件开发者」/「错误检测」，上传 `fnpack.exe`（必要时也传 MSI）。
- 其它引擎：各自的 false positive 表单；VirusTotal 的检测结果页可以跳到厂商。
- 企业环境：由管理员按**文件哈希 + 发布者**在 Defender / Intune 里允许，而不是让
  安装程序自己去改杀软配置。

微软的 ML 误报（`!ml` 结尾）通常在 24–72 小时内通过签名更新解决。每次发布换了哈希都
可能要再提一次，所以签名 + 稳定发布者比逐次提交更重要。

## 3. 降低启发式命中（辅助手段）

- **加版本资源**：官方 fnpack 是 `-s -w` 的 Go 程序，没有 `VERSIONINFO`、图标、清单。
  Defender 的通用 ML 对「无版本信息 + 未签名 + 释放到用户目录」的组合很敏感。可以用
  `rcedit` 给副本写入 `CompanyName` / `ProductName` / `FileDescription` /
  `FileVersion` / `OriginalFilename`，或在有源码时重新构建并加 `go-winres`。改完必须
  重新签名；不要在签名后再改文件。
- **不要加壳/压缩**：UPX 之类会显著提高误报率。
- **不要**在安装程序里自动添加杀软排除项、改 Defender 配置或关闭实时保护。这类行为
  本身会被当成恶意行为，也会让企业环境直接拒绝安装。
- **不要**靠改文件名、改扩展名、Base64/加密释放来绕过扫描。这是对抗杀软，不是解决
  误报，后续只会更难维护。

## 4. 分发层面的兜底

如果短时间内无法签名，或者用户所在环境策略很严，可以：

### 4.1 提供不含 fnpack 的精简 MSI

构建时用 Tauri 的 `--config` 覆盖 `externalBin`，不打进 fnpack：

```powershell
npm run build:slim
```

它等价于用 `src-tauri/tauri.slim.conf.json` 覆盖 `externalBin`：只带
`fsfw-backend.exe`，不带 `fnpack.exe`。产物在同一个 `bundle\msi` 目录，产品名 / 升级码
与完整包一致，可以直接替换安装。

这样安装包只带后端和界面。用户自己准备官方 `fnpack.exe`，放进 `PATH`，或设置
`FSFW_FNPACK` 指向它。应用只在打包时需要 fnpack，浏览 / 编辑 / 导入 `.fpk` 不受影响。

### 4.2 让用户手动恢复 / 允许

误报被隔离后：

1. Windows 安全中心 → 病毒和威胁防护 → 保护历史记录；
2. 找到 `fnpack.exe` 那条 → 操作 → **还原**；
3. 如果仍被拦，管理员可为
   `%LOCALAPPDATA%\fpkstudio\fnpack.exe` 或该目录添加排除项（这是用户/管理员的
   安全决策，不应由安装程序代做）；
4. 或者把官方 fnpack 放到别处，设置 `FSFW_FNPACK` 指过去。

### 4.3 企业白名单

提供 SHA-256、发布者名称、安装路径，由管理员在 Defender / 终端安全平台按哈希或
发布者放行；升级前重新确认哈希。

## 5. 检查清单

- [ ] `vendor/fnpack.exe` 哈希与官方一致
- [ ] `fnpack.exe`、`fsfw-backend.exe`、`fsfw.exe`、MSI 全部签名并通过
      `signtool verify /pa`
- [ ] 签名带 RFC 3161 时间戳
- [ ] 向 Microsoft 及其它报毒引擎提交了误报（附哈希、检测名、引擎版本）
- [ ] 没有在安装器里自动改杀软配置
- [ ] 发布说明里告知用户：若被隔离，如何还原 / 设置 `FSFW_FNPACK`
