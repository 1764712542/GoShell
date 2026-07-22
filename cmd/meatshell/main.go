// Package main 是 Meatshell 终端客户端的入口。
// 它初始化日志、配置、国际化、应用控制器和 UI，然后启动主窗口。
package main

import (
	"flag"
	"os"

	"fyne.io/fyne/v2/app"

	"github.com/zhuyao/meatshell/internal/config"
	"github.com/zhuyao/meatshell/internal/i18n"
	"github.com/zhuyao/meatshell/internal/log"
	mapp "github.com/zhuyao/meatshell/internal/app"
	"github.com/zhuyao/meatshell/internal/ui"
)

// version 是应用版本号（构建时可通过 -ldflags 注入）
var version = "1.0.0"

func main() {
	// 解析命令行参数
	debug := flag.Bool("debug", false, "启用调试日志")
	lang := flag.String("lang", "zh-CN", "界面语言（zh-CN / en-US）")
	flag.Parse()

	// 初始化日志
	log.Init(*debug)
	log.Info("meatshell starting", "version", version)

	// 创建 Fyne 应用
	fyneApp := app.New()
	fyneApp.SetIcon(nil) // 可设置应用图标

	// 加载配置存储
	store := config.NewStore()
	if err := store.Load(); err != nil {
		log.Warn("failed to load sessions", "err", err)
	}

	// 初始化国际化
	i18nMgr := i18n.NewManager()
	if err := i18nMgr.Load(*lang); err != nil {
		log.Warn("failed to load i18n, using defaults", "lang", *lang, "err", err)
	}

	// 创建应用控制器
	a := mapp.New(fyneApp, store, i18nMgr)

	// 创建主窗口
	mainWin := ui.NewMainWindow(a)

	log.Info("meatshell ready, showing main window")

	// 启动应用（阻塞直到窗口关闭）
	mainWin.Run()

	log.Info("meatshell exited")
	os.Exit(0)
}
