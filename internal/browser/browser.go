package browser

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"cdpnetool/internal/logger"
)

// Options 浏览器启动选项
type Options struct {
	ExecPath            string        // 浏览器可执行文件路径
	UserDataDir         string        // 用户数据目录
	RemoteDebuggingPort int           // CDP端口，0表示自动选择
	Headless            bool          // 是否以无头模式启动
	Args                []string      // 额外启动参数
	Env                 []string      // 额外环境变量
	ClearUserData       bool          // 启动前是否清空用户数据目录
	Logger              logger.Logger // 日志记录器
}

// Browser 已启动的浏览器进程句柄
type Browser struct {
	cmd         *exec.Cmd
	DevToolsURL string
	port        int
	logger      logger.Logger
}

// Start 启动浏览器并等待CDP服务就绪
func Start(ctx context.Context, opts Options) (*Browser, error) {
	l := opts.Logger
	if l == nil {
		l = logger.NewNop()
	}

	exe := opts.ExecPath
	if exe == "" {
		exe = findExecutable()
	}
	if exe == "" {
		return nil, errors.New("browser executable not found (chrome/edge/chromium)")
	}

	l.Info("准备启动浏览器", "path", exe)

	port := opts.RemoteDebuggingPort
	if port == 0 {
		port = 9222
	}

	finalPort, err := pickPort(l, port)
	if err != nil {
		return nil, fmt.Errorf("failed to pick port: %w", err)
	}
	l.Debug("选用调试端口", "port", finalPort)

	if opts.UserDataDir == "" {
		opts.UserDataDir = filepath.Join(os.TempDir(), "cdpnetool-chrome-profile")
	}

	if opts.ClearUserData {
		l.Debug("正在清空用户数据目录", "dir", opts.UserDataDir)
		if err := os.RemoveAll(opts.UserDataDir); err != nil {
			l.Warn("清空用户数据目录失败", "dir", opts.UserDataDir, "error", err)
		}
	}

	if err := os.MkdirAll(opts.UserDataDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create user data dir: %w", err)
	}

	port = finalPort
	args := buildLaunchArgs(port, opts)
	cmd := exec.CommandContext(ctx, exe, args...)
	if len(opts.Env) > 0 {
		cmd.Env = append(os.Environ(), opts.Env...)
	}

	l.Debug("浏览器启动命令", "args", args)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start browser: %w", err)
	}

	b := &Browser{
		cmd:         cmd,
		DevToolsURL: fmt.Sprintf("http://127.0.0.1:%d", port),
		port:        port,
		logger:      l,
	}
	waitCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	l.Debug("等待 DevTools 就绪", "url", b.DevToolsURL)
	if err := waitDevToolsReady(waitCtx, b.DevToolsURL); err != nil {
		if stopErr := b.Stop(2 * time.Second); stopErr != nil {
			l.Warn("启动失败后关闭浏览器出错", "error", stopErr)
		}
		return nil, fmt.Errorf("devtools not ready: %w", err)
	}

	l.Info("浏览器启动成功", "url", b.DevToolsURL)
	return b, nil
}

// Stop 关闭浏览器进程
func (b *Browser) Stop(timeout time.Duration) error {
	if b == nil || b.cmd == nil || b.cmd.Process == nil {
		return nil
	}
	done := make(chan error, 1)
	go func() { done <- b.cmd.Wait() }()
	// Windows上直接Kill以避免悬挂
	if err := b.cmd.Process.Kill(); err != nil {
		b.logger.Warn("终止浏览器进程失败", "error", err)
	}
	select {
	case <-time.After(timeout):
		return errors.New("browser stop timeout")
	case err := <-done:
		return err
	}
}

// findExecutable 查找可用的浏览器执行路径（Chrome/Edge/Chromium）
func findExecutable() string {
	candidates := getBrowserPaths()
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// getBrowserPaths 返回各平台下 Chrome、Edge 和 Chromium 的默认安装路径
func getBrowserPaths() []string {
	switch runtime.GOOS {
	case "windows":
		return []string{
			// Chrome
			filepath.Join(os.Getenv("ProgramFiles"), "Google", "Chrome", "Application", "chrome.exe"),
			filepath.Join(os.Getenv("ProgramFiles(x86)"), "Google", "Chrome", "Application", "chrome.exe"),
			filepath.Join(os.Getenv("LOCALAPPDATA"), "Google", "Chrome", "Application", "chrome.exe"),
			// Edge
			filepath.Join(os.Getenv("ProgramFiles(x86)"), "Microsoft", "Edge", "Application", "msedge.exe"),
			filepath.Join(os.Getenv("ProgramFiles"), "Microsoft", "Edge", "Application", "msedge.exe"),
			// Chromium
			filepath.Join(os.Getenv("ProgramFiles"), "Chromium", "Application", "chrome.exe"),
			filepath.Join(os.Getenv("ProgramFiles(x86)"), "Chromium", "Application", "chrome.exe"),
			filepath.Join(os.Getenv("LOCALAPPDATA"), "Chromium", "Application", "chrome.exe"),
		}
	case "darwin":
		return []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			filepath.Join(os.Getenv("HOME"), "Applications", "Google Chrome.app", "Contents", "MacOS", "Google Chrome"),
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
		}
	case "linux":
		return []string{
			"/usr/bin/google-chrome",
			"/usr/bin/google-chrome-stable",
			"/usr/bin/microsoft-edge",
			"/usr/bin/microsoft-edge-stable",
			"/usr/bin/chromium",
			"/usr/bin/chromium-browser",
			"/snap/bin/chromium",
		}
	default:
		return nil
	}
}

// pickPort 尝试使用指定端口，如果被占用则选择随机空闲端口
func pickPort(l logger.Logger, preferred int) (int, error) {
	// 先尝试首选端口
	if preferred > 0 {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", preferred))
		if err == nil {
			if closeErr := ln.Close(); closeErr != nil {
				l.Warn("关闭临时监听器失败", "error", closeErr)
			}
			return preferred, nil
		}
	}

	// 首选端口不可用，选择随机空闲端口
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("failed to find free port: %w", err)
	}
	defer func() {
		if closeErr := ln.Close(); closeErr != nil {
			l.Warn("关闭随机监听器失败", "error", closeErr)
		}
	}()

	return ln.Addr().(*net.TCPAddr).Port, nil
}

// buildLaunchArgs 构建浏览器启动参数
func buildLaunchArgs(port int, opts Options) []string {
	args := []string{
		fmt.Sprintf("--remote-debugging-port=%d", port),
		"--no-first-run",
		"--no-default-browser-check",
		"--start-maximized",
	}

	// Linux 环境下添加额外参数
	if runtime.GOOS == "linux" {
		args = append(args, "--disable-dev-shm-usage")
	}

	// 用户数据目录
	if opts.UserDataDir != "" {
		args = append(args, fmt.Sprintf("--user-data-dir=%s", opts.UserDataDir))
	}

	// 无头模式
	if opts.Headless {
		args = append(args, "--headless=new", "--disable-gpu")
	}

	// 额外参数
	if len(opts.Args) > 0 {
		args = append(args, opts.Args...)
	}

	return args
}

// waitDevToolsReady 轮询 DevTools 服务是否就绪
func waitDevToolsReady(ctx context.Context, base string) error {
	url := fmt.Sprintf("%s/json/version", base)
	cli := &http.Client{Timeout: 500 * time.Millisecond}
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("devtools not ready after timeout: %w", ctx.Err())
		case <-ticker.C:
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			if err != nil {
				continue
			}
			resp, err := cli.Do(req)
			if err == nil && resp.StatusCode == 200 {
				resp.Body.Close()
				return nil
			}
			if resp != nil {
				resp.Body.Close()
			}
		}
	}
}
