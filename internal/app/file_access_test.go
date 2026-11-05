package app

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/egoist/mygo/ui"

	"gopaste/internal/locales"
	"gopaste/internal/types"
)

// TestFileAuthorization 验证权限错误分类、文件夹范围校验、取消和授权后的粘贴续接
func TestFileAuthorization(t *testing.T) {
	t.Setenv("MYGO_LOCALE", "zh-Hans")
	root := t.TempDir()
	dir, other := filepath.Join(root, "图片"), filepath.Join(root, "图片-其他")
	for _, folder := range []string{dir, other} {
		if err := os.Mkdir(folder, 0700); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(dir, "原文件.jpg")
	if err := os.WriteFile(path, []byte("图片内容"), 0600); err != nil {
		t.Fatal(err)
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	rec := &types.Record{Kind: types.KindFile, Files: []types.FileRef{{Path: path}}}
	for _, cause := range []error{fs.ErrPermission, syscall.EPERM, syscall.EIO} {
		a := NewApp()
		resumed := 0
		err := fileReadError(&fs.PathError{Op: "open", Path: path, Err: cause})
		a.pasteError(rec, err, func() {
			resumed++
			data, err := a.clipboardData(rec, false)
			if err != nil {
				t.Fatal(err)
			}
			files, err := data.Files()
			if err != nil || len(files) != 1 || files[0] != path {
				t.Fatalf("授权后未重新读取原文件：%v，%v", files, err)
			}
		})
		permission := errors.Is(cause, fs.ErrPermission)
		if !errors.Is(err, cause) || !strings.Contains(a.hintFlash, path) ||
			strings.Contains(a.hintFlash, locales.FileReadPermissionDenied()) != permission {
			t.Fatalf("错误分类或完整原因丢失：%v", err)
		}
		if !isDarwin || !permission {
			if a.fileAuthorization != nil || a.hintOpen {
				t.Fatal("不适用的错误触发了文件授权")
			}
			continue
		}
		request := a.fileAuthorization
		if request == nil || !a.hintOpen || request.path != dir {
			t.Fatal("权限失败未定位原文件所在文件夹")
		}
		a.finishFileAuthorization(request, nil, nil)
		if resumed != 0 || a.fileAuthorization != request {
			t.Fatal("取消授权仍执行粘贴或丢失重试入口")
		}
		a.finishFileAuthorization(request, []string{other}, nil)
		if resumed != 0 || !a.hintOpen || !strings.Contains(a.hintFlash, locales.SelectContainingFolder()) {
			t.Fatal("选择无关文件夹后仍粘贴或未说明原因")
		}
		a.finishFileAuthorization(request, nil, syscall.EIO)
		if resumed != 0 || a.fileAuthorization != request || !strings.Contains(a.hintFlash, syscall.EIO.Error()) {
			t.Fatal("系统选择器失败后丢失完整原因或重试入口")
		}
		selected := resolved
		if cause == syscall.EPERM {
			selected = root
		}
		a.finishFileAuthorization(request, []string{selected}, nil)
		if resumed != 1 || a.hintOpen || a.hintFlash != "" || a.fileAuthorization != nil {
			t.Fatal("授权后未继续粘贴或清除旧错误")
		}
	}
	if isDarwin {
		a := NewApp()
		rec := &types.Record{Kind: types.KindFile, Files: []types.FileRef{{Path: dir, Dir: true}}}
		a.pasteError(rec, fileReadError(&fs.PathError{Op: "open", Path: dir, Err: fs.ErrPermission}), func() {})
		if a.fileAuthorization == nil || a.fileAuthorization.path != dir {
			t.Fatal("原记录为文件夹时未定位该文件夹")
		}
	}
}

// TestHintDetails 验证两种主题和面板中的完整错误可查看、滚动且不触发背景快捷键
func TestHintDetails(t *testing.T) {
	t.Setenv("MYGO_LOCALE", "zh-Hans")
	for _, form := range []int{formPanel, formBar} {
		for _, theme := range []int{themeClassic, themeGlass} {
			t.Run(fmt.Sprintf("form-%d-theme-%d", form, theme), func(t *testing.T) {
				a := NewApp()
				a.form, a.settings.Theme = form, theme
				a.records = []*types.Record{{ID: "one", Kind: types.KindText, Text: "记录一"}, {ID: "two", Kind: types.KindText, Text: "记录二"}}
				initTestStore(t, a)
				message := locales.FileReadPermissionDenied() + "\nopen /Users/test/Library/Containers/" + strings.Repeat("long-path/", 80) + "图片.jpg: operation not permitted"
				a.toast(message)
				a.fileAuthorization = &fileAuthorization{path: "/Users/test/Library/Containers"}
				width, height := 432, 596
				if form == formBar {
					width, height = 900, 238
				}
				tt := ui.NewTester(a.view, width, height)
				if err := tt.Click(locales.FileReadPermissionDenied()); err != nil {
					t.Fatal(err)
				}
				if !a.hintOpen || !tt.HasText(message) || !tt.HasText(locales.OperationDetails()) {
					t.Fatal("点击提示后未显示完整错误")
				}
				button, ok := tt.Find(locales.Done())
				if !ok || button.Y < 0 || button.Y+button.H > float32(height) {
					t.Fatalf("长错误挤出关闭按钮：%+v", button)
				}
				button, ok = tt.Find(locales.AuthorizeFolder())
				if !ok || button.Y < 0 || button.Y+button.H > float32(height) {
					t.Fatalf("长错误挤出文件夹授权按钮：%+v", button)
				}
				tt.Scroll(float32(width)/2, float32(height)/2, 0, 2000)
				if os.Getenv("SHOTS") != "" {
					writePNG(t, filepath.Join("..", "..", ".mygo", "shots", fmt.Sprintf("file-error-%d-%d.png", form, theme)), tt.Image())
				}
				tt.Key(0, ui.KeyDown)
				tt.Key(0, ui.KeySpace)
				if a.sel != 0 || a.preview {
					t.Fatal("详情窗口的按键操作穿透到历史记录")
				}
				tt.Key(0, ui.KeyEscape)
				if a.hintOpen || a.hintFlash != message {
					t.Fatal("关闭详情时丢失错误信息或未关闭")
				}
			})
		}
	}
}
