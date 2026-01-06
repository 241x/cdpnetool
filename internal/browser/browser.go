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
    "time"
)

// Options 浏览器启动选项
type Options struct {
    ExecPath            string   // 浏览器可执行文件路径
    UserDataDir         string   // 用户数据目录
    RemoteDebuggingPort int      // CDP端口，0表示自动选择
    Headless            bool     // 是否以无头模式启动
    Args                []string // 额外启动参数
    Env                 []string // 额外环境变量
}

// Browser 已启动的浏览器进程句柄
type Browser struct {
    cmd         *exec.Cmd
    DevToolsURL string
    port        int
}

// Start 启动浏览器并等待CDP服务就绪
func Start(opts Options) (*Browser, error) {
    exe := opts.ExecPath
    if exe == "" {
        exe = defaultChromePath()
    }
    if exe == "" {
        return nil, errors.New("chrome executable not found")
    }
    port := opts.RemoteDebuggingPort
    if port == 0 {
        p, err := pickFreePort()
        if err != nil {
            port = 9222
        } else {
            port = p
        }
    }
    args := []string{
        fmt.Sprintf("--remote-debugging-port=%d", port),
    }
    if opts.UserDataDir != "" {
        _ = os.MkdirAll(opts.UserDataDir, 0o755)
        args = append(args, fmt.Sprintf("--user-data-dir=%s", opts.UserDataDir))
    } else {
        dir := filepath.Join(os.TempDir(), "cdpnetool-chrome")
        _ = os.MkdirAll(dir, 0o755)
        args = append(args, fmt.Sprintf("--user-data-dir=%s", dir))
    }
    if opts.Headless {
        args = append(args, "--headless=new", "--disable-gpu")
    }
    if len(opts.Args) > 0 {
        args = append(args, opts.Args...)
    }
    cmd := exec.Command(exe, args...)
    if len(opts.Env) > 0 {
        cmd.Env = append(os.Environ(), opts.Env...)
    }
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    if err := cmd.Start(); err != nil {
        return nil, err
    }
    b := &Browser{cmd: cmd, DevToolsURL: fmt.Sprintf("http://127.0.0.1:%d", port), port: port}
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    if err := waitDevToolsReady(ctx, b.DevToolsURL); err != nil {
        _ = b.Stop(2 * time.Second)
        return nil, err
    }
    return b, nil
}

// Stop 关闭浏览器进程（尽力而为）
func (b *Browser) Stop(timeout time.Duration) error {
    if b == nil || b.cmd == nil || b.cmd.Process == nil {
        return nil
    }
    done := make(chan error, 1)
    go func() { done <- b.cmd.Wait() }()
    // Windows上直接Kill以避免悬挂
    _ = b.cmd.Process.Kill()
    select {
    case <-time.After(timeout):
        return errors.New("browser stop timeout")
    case err := <-done:
        return err
    }
}

// defaultChromePath 返回常见的Chrome可执行路径（Windows优先）
func defaultChromePath() string {
    // 常见路径，优先选择64位安装目录
    candidates := []string{
        `C:\Program Files\Google\Chrome\Application\chrome.exe`,
        `C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
    }
    for _, p := range candidates {
        if _, err := os.Stat(p); err == nil {
            return p
        }
    }
    // 退化为PATH查找
    if p, err := exec.LookPath("chrome"); err == nil {
        return p
    }
    return ""
}

// pickFreePort 选择一个本地空闲端口
func pickFreePort() (int, error) {
    l, err := net.Listen("tcp", "127.0.0.1:0")
    if err != nil { return 0, err }
    defer l.Close()
    return l.Addr().(*net.TCPAddr).Port, nil
}

// waitDevToolsReady 轮询DevTools服务是否就绪
func waitDevToolsReady(ctx context.Context, base string) error {
    url := fmt.Sprintf("%s/json/version", base)
    cli := &http.Client{Timeout: 500 * time.Millisecond}
    ticker := time.NewTicker(300 * time.Millisecond)
    defer ticker.Stop()
    for {
        req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
        resp, err := cli.Do(req)
        if err == nil && resp.StatusCode == 200 {
            resp.Body.Close()
            return nil
        }
        if resp != nil { resp.Body.Close() }
        select {
        case <-ctx.Done():
            return errors.New("devtools not ready")
        case <-ticker.C:
        }
    }
}

