package server

import (
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// 系统保存对话框返回的绝对路径怎么落地。
//
// 和 `/api/projects/import-path` 同一层信任模型：路径是用户在弹出的原生对话框里
// 选的，进程本来就有当前用户的权限，不需要再套一层目录白名单。这里只做「能不能
// 写」的基本校验，错误都翻成 400，让界面能直接说清楚哪里不对。

// cleanSavePath 校验保存目标：必须是非空绝对路径、父目录已存在、目标不是目录。
//
// 不要求目标文件已存在（保存新文件是常态），也不做扩展名补全——用户选的什么名字
// 就写什么名字。
func cleanSavePath(raw string) (string, error) {
	p := strings.TrimSpace(raw)
	if p == "" {
		return "", badRequest("缺少保存路径")
	}
	if !filepath.IsAbs(p) {
		return "", badRequest("保存路径必须是绝对路径：%s", p)
	}
	p = filepath.Clean(p)

	parent := filepath.Dir(p)
	st, err := os.Stat(parent)
	if err != nil || !st.IsDir() {
		return "", badRequest("保存目录不可用：%s", parent)
	}
	// 目标是已存在的目录：直接报错，别让 os.Rename 去覆盖它（也覆盖不了）。
	if st, err := os.Stat(p); err == nil && st.IsDir() {
		return "", badRequest("保存路径是一个目录：%s", p)
	}
	return p, nil
}

// saveToPath 把 src 的内容写到 path。
//
// 先写目标目录下的临时文件再 rename：中途失败（磁盘满、权限、客户端取消）不会把
// 用户原有的同名文件截成半截。os.Rename 在 Windows（MoveFileEx + REPLACE_EXISTING）
// 和 POSIX 上都会替换已存在的文件，所以不需要先删旧的——rename 失败时旧文件还在。
func saveToPath(path string, src io.Reader) (int64, error) {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".fsfw-save-*")
	if err != nil {
		return 0, err
	}
	tmpName := tmp.Name()
	// rename 成功之后这个路径已经不存在了，Remove 只是失败路径上的兜底。
	defer func() { _ = os.Remove(tmpName) }()

	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return 0, err
	}
	n, copyErr := io.Copy(tmp, src)
	closeErr := tmp.Close()
	if copyErr != nil {
		return n, copyErr
	}
	if closeErr != nil {
		return n, closeErr
	}

	if err := os.Rename(tmpName, path); err != nil {
		return n, err
	}
	return n, nil
}

// saveError 把保存失败翻成 HTTP 错误：权限是 403，其余算服务端故障。
func saveError(path string, err error) error {
	if errors.Is(err, fs.ErrPermission) || os.IsPermission(err) {
		return fail(http.StatusForbidden, "没有权限写入 %s：%v", path, err)
	}
	return internal("写入 %s 失败: %v", path, err)
}
