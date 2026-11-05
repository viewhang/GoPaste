package app

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"

	"github.com/egoist/mygo"

	"gopaste/internal/locales"
	"gopaste/internal/types"
)

// fileAuthorization 保存原文件需要授权的文件夹及授权后继续执行的粘贴操作
type fileAuthorization struct {
	path  string
	retry func()
}

// fileReadError 区分读取权限与其他文件错误并保留完整路径和系统原因
func fileReadError(err error) error {
	message := locales.FileAccessFailed()
	if errors.Is(err, fs.ErrPermission) {
		message = locales.FileReadPermissionDenied()
	}
	return fmt.Errorf("%s\n%w", message, err)
}

// pasteError 显示粘贴失败原因并为 macOS 原文件权限错误提供授权入口
func (a *App) pasteError(rec *types.Record, err error, retry func()) {
	a.toast(err.Error())
	var pathError *fs.PathError
	if !isDarwin || rec.Kind != types.KindFile || !errors.Is(err, fs.ErrPermission) || !errors.As(err, &pathError) {
		return
	}
	rec = a.fullRecord(rec)
	if rec == nil {
		return
	}
	for _, file := range rec.Files {
		if file.Path == pathError.Path {
			folder := file.Path
			if !file.Dir {
				folder = filepath.Dir(folder)
			}
			a.fileAuthorization = &fileAuthorization{path: folder, retry: retry}
			a.hintOpen = true
			return
		}
	}
}

// authorizePasteFolder 打开定位原文件所在文件夹的系统选择器并在授权后继续粘贴
func (a *App) authorizePasteFolder() {
	request := a.fileAuthorization
	a.choosingPasteFolder = true
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: a.win, Title: locales.AuthorizeFolder(), DefaultPath: request.path,
			Message: locales.AuthorizeFolderHint(), ButtonLabel: locales.AuthorizeAndPaste(),
			Directory: true, ShowHiddenFiles: true,
		})
		a.win.Update(func() {
			a.choosingPasteFolder = false
			a.win.Focus()
			a.finishFileAuthorization(request, paths, err)
		})
	}()
}

// finishFileAuthorization 校验所选文件夹包含原文件并在取消或失败时保留原粘贴请求
func (a *App) finishFileAuthorization(request *fileAuthorization, paths []string, err error) {
	if err == nil && len(paths) == 0 {
		return
	}
	if err == nil {
		var original, selected, relative string
		selected, err = filepath.EvalSymlinks(paths[0])
		if err == nil {
			original, err = filepath.EvalSymlinks(request.path)
		}
		if err == nil {
			relative, err = filepath.Rel(selected, original)
		}
		if err == nil && !filepath.IsLocal(relative) {
			err = fmt.Errorf("%s\n%s", locales.SelectContainingFolder(), request.path)
		}
	}
	if err != nil {
		a.toast(err.Error())
		a.fileAuthorization = request
		a.hintOpen = true
		return
	}
	a.toast("")
	request.retry()
}
