#!/usr/bin/env node
// 给一次构建注入版本号。
//
//   node scripts/set-version.mjs 1.0.1
//
// 同时改三处，避免 MSI 版本、运行时版本、Go 后端版本对不上：
//   - src-tauri/tauri.conf.json（Tauri 打包 / MSI ProductVersion）
//   - src-tauri/Cargo.toml（fsfw.exe 的 package_info，前端 window.__FSFW__.version）
//   - src-tauri/Cargo.lock（锁定文件里的 fsfw 版本，避免 cargo 认为锁文件过期）
//
// Go 后端版本由 scripts/build-backend.ps1 从 tauri.conf.json 读取后通过 -ldflags 注入。
import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const version = (process.argv[2] ?? '').trim()

if (!/^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/.test(version)) {
  console.error(`版本号格式不对：${version || '(空)'}`)
  console.error('需要 x.y.z（Cargo 只接受三段式 SemVer，MSI 也用它）。')
  process.exit(1)
}

const [major, minor, patch] = version.split('.').map(Number)
if (major > 255 || minor > 255 || patch > 65535) {
  console.error('版本号超出 MSI 限制：major/minor <= 255，patch <= 65535。')
  process.exit(1)
}

function read(rel) {
  return fs.readFileSync(path.join(root, rel), 'utf8')
}

function write(rel, text) {
  fs.writeFileSync(path.join(root, rel), text)
}

const tauriPath = 'src-tauri/tauri.conf.json'
const tauri = JSON.parse(read(tauriPath))
tauri.version = version
write(tauriPath, `${JSON.stringify(tauri, null, 2)}\n`)

const cargoPath = 'src-tauri/Cargo.toml'
const cargo = read(cargoPath)
if (!/^version = "[^"]+"/m.test(cargo)) {
  console.error('Cargo.toml 里没有找到包版本号')
  process.exit(1)
}
write(cargoPath, cargo.replace(/^version = "[^"]+"/m, `version = "${version}"`))

const lockPath = 'src-tauri/Cargo.lock'
const lock = read(lockPath)
const lockRe = /(\[\[package\]\]\nname = "fsfw"\nversion = ")[^"]+(")/
if (!lockRe.test(lock)) {
  console.error('Cargo.lock 里没有找到 fsfw 的版本号')
  process.exit(1)
}
write(lockPath, lock.replace(lockRe, `$1${version}$2`))

console.log(`version -> ${version}`)
console.log(`  ${tauriPath}`)
console.log(`  ${cargoPath}`)
console.log(`  ${lockPath}`)
