/** 与 Go 侧 internal/server 的响应结构一一对应。 */

export interface FnpackInfo {
  bin: string
  version: string
  message: string
  ok: boolean
}

export interface SystemInfo {
  version: string
  /** 运行平台：Windows 构建是 `windows`。 */
  platform: string
  pid: number
  dataDir: string
  projectsDir: string
  outputDir: string
  logDir: string
  tmpDir: string
  logFile: string
  fnpack: FnpackInfo
  lastSeq: number
  startedAt: string
}

export interface ProjectInfo {
  name: string
  /** 项目目录的绝对路径（换过项目根目录之后 projectsDir + name 是拼不出来的）。 */
  absPath: string
  appname: string
  version: string
  displayName: string
  platform: string
  docker: boolean
  ui: boolean
  building: boolean
  modTime: string
}

export interface ProjectExists {
  exists: boolean
  docker: boolean
  ui: boolean
}

export interface PackSettings {
  multiPlatform: boolean
  platforms: string[]
  serverDir: string
  copyToOutput: boolean
}

export interface PackResult {
  platform: string
  ok: boolean
  file: string
  path: string
  size: number
  log: string
}

export interface Artifact {
  name: string
  scope: 'project' | 'output'
  path: string
  size: number
  sizeRaw: string
  modTime: string
}

export interface FsEntry {
  name: string
  dir: boolean
  size: number
  mode: string
  modTime: string
  symlink: boolean
}

export interface FsListing {
  path: string
  abs: string
  entries: FsEntry[]
}

export interface FileContent {
  path: string
  size: number
  sizeRaw: string
  text: boolean
  content: string
  /** 为真表示文件太大，后端没有把内容带回来（编辑器要给「去下载」的提示）。 */
  tooLarge: boolean
}

/** 解压结果（`/api/fs/extract`）。 */
export interface ExtractResult {
  /** 识别出来的归档类型：tar / tar.gz / zip / gz…。 */
  kind: string
  /** 解压到的目录（相对项目根目录）。 */
  dir: string
  /** 本次真正写进去的文件数。 */
  files: number
  /** 因为同名已存在而跳过的文件数。 */
  skipped: number
  /** 归档里带、但没有还原的符号链接数（后端不还原链接，见 fsx.sanitizeExtracted）。 */
  links: number
  /** 归档里带、但没有还原的非普通文件数（FIFO / 设备节点，见 fsx.sanitizeExtracted）。 */
  specials: number
}

export interface LogEntry {
  seq: number
  time: string
  level: 'debug' | 'info' | 'warn' | 'error'
  source: string
  msg: string
}

export interface DirSettings {
  /** 当前生效的绝对路径（可能来自用户设置，也可能是默认值）。 */
  projectRoot: string
  outputDir: string
  /** 该路径是不是用户设置过的（为假时界面上标「默认」）。 */
  projectRootFromUser: boolean
  outputDirFromUser: boolean
  defaultProjectRoot: string
  defaultOutputDir: string
  /** 为真表示 FSFW_OUTPUT_DIR 与项目目录撞了、已被忽略，生效的是默认输出目录。 */
  outputEnvIgnored: boolean
}

export interface SettingsInfo {
  dirs: DirSettings
  dataDir: string
}

/** 导出 / 保存到本机后的结果。 */
export interface SaveResult {
  ok: boolean
  /** 实际写入的绝对路径。 */
  path: string
  /** 写入的字节数。 */
  size: number
}
