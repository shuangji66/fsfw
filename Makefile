# Makefile: builds the FSFW Go backend.
#
#   make                    # 本机（Linux 开发机）构建 -> .tmp/fsfw-backend
#   make backend-windows    # 交叉编译 Windows amd64 -> .tmp/fsfw-backend.exe
#   make release V=1.0.1    # 去掉调试信息、注入版本号
#   make test               # 后端单元测试
#   make vet                # go vet
#   make clean              # 清理构建产物
#
# 前端与 Tauri 外壳由 src-tauri/ 自己的构建流程处理（npm run tauri build），
# 这里不再碰 npm，也不再往 Go 二进制里塞 embed 资源：后端只提供本地 HTTP API。
#
# Go 缓存 / 模块目录默认放项目本地；环境里已设置 GOCACHE / GOPATH / TMPDIR 时
# 沿用环境值（?= 只在未定义时赋值）。

ROOT      := $(abspath $(dir $(lastword $(MAKEFILE_LIST))))
BACKEND   := $(ROOT)/backend
OUT_DIR   := $(ROOT)/.tmp
OUT       := $(OUT_DIR)/fsfw-backend
OUT_WIN   := $(OUT_DIR)/fsfw-backend.exe

# 版本号：默认 1.0.0，可经 V 覆盖（如 make release V=1.0.1）。
version   ?= 1.0.0
V         ?= $(version)
LDFLAGS   ?= -X main.version=$(V)

GOCACHE   ?= $(ROOT)/.gocache
GOPATH    ?= $(ROOT)/.gopath
# go build 的工作目录走 os.TempDir()，也就是 $TMPDIR 或 /tmp。有些环境（含网络盘、
# 容器挂载卷）下 /tmp 权限受限，构建会报 "open /tmp/go-build.../exe/a.out: permission
# denied"，指到项目本地即可。
TMPDIR    ?= $(ROOT)/.tmp
export GOCACHE GOPATH TMPDIR

# 纯 Go，不需要 cgo：产物静态链接，交叉编译也不用准备工具链。
# 环境里已设 CGO_ENABLED 则沿用。
CGO_ENABLED ?= 0
export CGO_ENABLED

# -buildvcs=false：仓库可能没有 .git（发布包 / 子目录检出），没必要因此失败。
# 保留调用者已经设置的其它 GOFLAGS。
export GOFLAGS := -buildvcs=false $(GOFLAGS)

.PHONY: all backend backend-windows release test vet clean

all: backend

# 本机构建（Linux 开发机）。GOOS 不写死，交给 go 的默认值。
backend: ## 构建本机后端
	@mkdir -p "$(GOCACHE)" "$(GOPATH)" "$(TMPDIR)" "$(OUT_DIR)"
	cd "$(BACKEND)" && go build -trimpath -ldflags "$(LDFLAGS)" -o "$(OUT)" .
	@echo "==> $(OUT)"

backend-windows: ## 交叉编译 Windows 后端
	@mkdir -p "$(GOCACHE)" "$(GOPATH)" "$(TMPDIR)" "$(OUT_DIR)"
	cd "$(BACKEND)" && GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o "$(OUT_WIN)" .
	@echo "==> $(OUT_WIN)"

release: ## Release build (strip symbols)
	$(MAKE) backend backend-windows LDFLAGS="-s -w -X main.version=$(V)"

test: ## Run backend unit tests
	@mkdir -p "$(GOCACHE)" "$(GOPATH)" "$(TMPDIR)"
	cd "$(BACKEND)" && go test ./...

vet: ## Run go vet
	@mkdir -p "$(GOCACHE)" "$(GOPATH)" "$(TMPDIR)"
	cd "$(BACKEND)" && go vet ./...

clean:
	rm -f "$(OUT)" "$(OUT_WIN)"
	@# 只删构建产物；GOCACHE / GOPATH 留着，重编译快很多。
