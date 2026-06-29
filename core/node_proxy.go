package core

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/cdle/sillyplus/core/common"
	"github.com/cdle/sillyplus/core/logs"
	"github.com/cdle/sillyplus/utils"
	"github.com/gin-gonic/gin"
)

// Node 插件反向代理管理器
// 当 Node 插件声明 @http 路由时，自动分配端口、启动子进程、注册 Gin 反向代理

type nodeProxyEntry struct {
	UUID       string
	Port       int
	Cmd        *exec.Cmd
	RouteCount int
	Proxy      *httputil.ReverseProxy
	StopChan   chan struct{}
}

var (
	nodeProxies     sync.Map // uuid → *nodeProxyEntry
	usedPorts       sync.Map // port → true
	reloadingPlugins sync.Map // uuid → true, 标记正在重载的插件，防止后台 goroutine 清理
	portMutex       sync.Mutex
	proxyBasePort   = 40000
	proxyMaxPort    = 50000
)

// StartNodeProxy 启动 Node 插件的反向代理
// 1. 分配端口
// 2. 启动 Node 子进程（设置 HTTP_LISTEN_PORT）
// 3. 等待端口就绪
// 4. 注册 Gin 反向代理路由
func StartNodeProxy(f *common.Function) {
	if f.Type != "node" || len(f.Https) == 0 {
		return
	}

	uuid := f.UUID
	// 检查是否已启动
	if _, loaded := nodeProxies.Load(uuid); loaded {
		return
	}

	// 分配端口
	port := getAvailablePort()
	if port == 0 {
		logs.Error("无法为插件 [%s] 分配端口", f.Title)
		return
	}

	// 启动 Node 子进程（优先系统 Node）
	bin := GetNodeBin()
	pluginPath := f.Path
	cmd := exec.Command(bin, pluginPath)
	cmd.Dir = filepath.Dir(pluginPath)
	cmd.Env = append(os.Environ(),
		"PLUGIN_ID="+uuid,
		"RUNTIME_ID="+utils.GenUUID(),
		"HTTP_LISTEN_PORT="+fmt.Sprint(port),
	)

	// 捕获 stdout/stderr
	stdout, err := cmd.StdoutPipe()
	if err == nil {
		go io.Copy(consoleLogWriter(uuid, "stdout"), stdout)
	}
	stderr, err2 := cmd.StderrPipe()
	if err2 == nil {
		go io.Copy(consoleLogWriter(uuid, "stderr"), stderr)
	}

	if err := cmd.Start(); err != nil {
		logs.Error("启动 Node 插件 [%s] 反向代理失败: %s", f.Title, err.Error())
		return
	}

	// 等待端口就绪（最长 5 秒）
	ready := waitForPort(port, 5*time.Second)
	if !ready {
		logs.Error("插件 [%s] HTTP 服务未在 %d 秒内就绪，已终止", f.Title, 5)
		cmd.Process.Kill()
		return
	}

	// 创建反向代理
	targetURL := &url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("127.0.0.1:%d", port),
	}
	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		logs.Warn("反向代理 [%s] 错误: %s", f.Title, err.Error())
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte("502 Bad Gateway: plugin unreachable"))
	}

	// 注册 Gin 路由
	for _, route := range f.Https {
		registerProxyRoute(uuid, *route, proxy)
	}

	// 保存状态
	entry := &nodeProxyEntry{
		UUID:       uuid,
		Port:       port,
		Cmd:        cmd,
		RouteCount: len(f.Https),
		Proxy:      proxy,
		StopChan:   make(chan struct{}),
	}
	nodeProxies.Store(uuid, entry)

	// 监听进程退出，自动清理
	go func() {
		cmd.Wait()
		// 如果正在重载，不清理（由 RestartNodeProxy 处理）
		if _, reloading := reloadingPlugins.Load(uuid); reloading {
			return
		}
		// 进程意外退出
		if _, ok := nodeProxies.Load(uuid); ok {
			logs.Warn("插件 [%s] 进程已退出，清理反向代理路由", f.Title)
			StopNodeProxy(uuid)
		}
	}()

	logs.Info("Node 插件 [%s] 反向代理就绪: 127.0.0.1:%d → %d 个路由", f.Title, port, len(f.Https))
}

// StopNodeProxy 停止并清理 Node 插件反向代理
func StopNodeProxy(uuid string) {
	v, ok := nodeProxies.Load(uuid)
	if !ok {
		return
	}
	entry := v.(*nodeProxyEntry)

	// 关闭内部 stop 信号
	select {
	case <-entry.StopChan:
		// already closed
	default:
		close(entry.StopChan)
	}

	// 杀掉进程
	if entry.Cmd != nil && entry.Cmd.Process != nil {
		entry.Cmd.Process.Kill()
	}

	// 完全停止时释放端口和路由（关闭用StopAllNodeProxies）
	usedPorts.Delete(entry.Port)
	nodeProxies.Delete(uuid)
	logs.Info("已停止插件 [%s] 反向代理 (端口 %d)", entry.UUID, entry.Port)
}

// RestartNodeProxy 重启 Node 插件反向代理（重载时调用，保留端口和路由）
// 如果找不到旧的反向代理记录，则启动新的反向代理
func RestartNodeProxy(f *common.Function) {
	uuid := f.UUID
	v, ok := nodeProxies.Load(uuid)
	if !ok {
		// 找不到旧记录，可能是初次启动失败，启动新的反向代理
		logs.Info("插件 [%s] 未找到旧的反向代理记录，尝试启动新的反向代理", f.Title)
		StartNodeProxy(f)
		return
	}
	entry := v.(*nodeProxyEntry)
	port := entry.Port

	// 标记为正在重载，防止旧进程退出时后台 goroutine 清理路由和端口
	reloadingPlugins.Store(uuid, true)
	defer reloadingPlugins.Delete(uuid)

	// 杀掉旧进程
	if entry.Cmd != nil && entry.Cmd.Process != nil {
		entry.Cmd.Process.Kill()
	}

	// 等待旧进程退出，释放端口（最多等 3 秒）
	for i := 0; i < 15; i++ {
		time.Sleep(200 * time.Millisecond)
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			ln.Close()
			break
		}
	}

	// 启动新进程，监听同一端口
	bin := GetNodeBin()
	cmd := exec.Command(bin, f.Path)
	cmd.Dir = filepath.Dir(f.Path)
	cmd.Env = append(os.Environ(),
		"PLUGIN_ID="+uuid,
		"RUNTIME_ID="+utils.GenUUID(),
		"HTTP_LISTEN_PORT="+fmt.Sprint(port),
	)

	// 捕获 stdout/stderr
	stdout, err := cmd.StdoutPipe()
	if err == nil {
		go io.Copy(consoleLogWriter(uuid, "stdout"), stdout)
	}
	stderr, err2 := cmd.StderrPipe()
	if err2 == nil {
		go io.Copy(consoleLogWriter(uuid, "stderr"), stderr)
	}

	if err := cmd.Start(); err != nil {
		logs.Error("重启 Node 插件 [%s] 失败: %s", f.Title, err.Error())
		StopNodeProxy(uuid)
		return
	}

	// 等待端口就绪（最长 5 秒）
	ready := waitForPort(port, 5*time.Second)
	if !ready {
		logs.Error("插件 [%s] HTTP 服务未在 5 秒内就绪，已终止", f.Title)
		cmd.Process.Kill()
		StopNodeProxy(uuid)
		return
	}

	// 更新反向代理指向新进程
	targetURL := &url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("127.0.0.1:%d", port),
	}
	entry.Proxy = httputil.NewSingleHostReverseProxy(targetURL)
	entry.Cmd = cmd
	entry.StopChan = make(chan struct{})
	nodeProxies.Store(uuid, entry)

	// 监听新进程退出，自动清理
	// 注意：这里不需要检查 reloadingPlugins，因为这是新进程的 goroutine
	// reloadingPlugins 只用于防止旧进程退出时旧 goroutine 的清理
	// 但需要检查 entry.Cmd 是否仍指向本进程，防止二次重载时误清理新条目
	go func() {
		cmd.Wait()
		if v, ok := nodeProxies.Load(uuid); ok {
			current := v.(*nodeProxyEntry)
			if current.Cmd != cmd {
				// entry 已被更新（发生了二次重载），不清理
				return
			}
			logs.Warn("插件 [%s] 进程已退出，清理反向代理路由", f.Title)
			StopNodeProxy(uuid)
		}
	}()

	logs.Info("已重启插件 [%s] 反向代理 (端口 %d, 保留路由)", f.Title, port)
}

// StopAllNodeProxies 停止所有 Node 插件反向代理（傻妞关闭时调用）
func StopAllNodeProxies() {
	nodeProxies.Range(func(key, value any) bool {
		StopNodeProxy(key.(string))
		return true
	})
}

// getAvailablePort 获取可用端口
func getAvailablePort() int {
	portMutex.Lock()
	defer portMutex.Unlock()

	for port := proxyBasePort; port <= proxyMaxPort; port++ {
		if _, used := usedPorts.Load(port); used {
			continue
		}
		// 检查端口是否真的可用
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			ln.Close()
			usedPorts.Store(port, true)
			return port
		}
	}
	return 0
}

// waitForPort 等待端口就绪，超时返回 false
func waitForPort(port int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 100*time.Millisecond)
		if err == nil {
			conn.Close()
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}

// registerProxyRoute 注册单个反向代理路由到 Gin
// handler 通过 uuid 动态查找当前 proxy，这样重载后旧路由自动指向新进程
func registerProxyRoute(uuid string, route common.Http, proxy *httputil.ReverseProxy) {
	method := route.Method
	path := route.Path

	handler := func(c *gin.Context) {
		v, ok := nodeProxies.Load(uuid)
		if !ok {
			c.String(http.StatusBadGateway, "502: plugin proxy not available")
			return
		}
		entry := v.(*nodeProxyEntry)
		entry.Proxy.ServeHTTP(c.Writer, c.Request)
	}

	// 避免重复注册导致 panic，用 recover 兜底
	defer func() {
		if r := recover(); r != nil {
			logs.Warn("路由已存在，跳过注册: %s %s (冲突: %v)", method, path, r)
		}
	}()

	var registered bool
	switch method {
	case "GET":
		Server.GET(path, handler)
		registered = true
	case "POST":
		Server.POST(path, handler)
		registered = true
	case "PUT":
		Server.PUT(path, handler)
		registered = true
	case "DELETE":
		Server.DELETE(path, handler)
		registered = true
	case "ANY":
		Server.Any(path, handler)
		registered = true
	}

	if registered {
		logs.Debug("注册反向代理路由: %s %s → [%s]", method, path, uuid)
	}
}



// consoleLogWriter 返回一个写入器，将 Node 插件的 stdout/stderr 输出到傻妞日志
func consoleLogWriter(uuid string, stream string) io.Writer {
	return &pluginLogWriter{uuid: uuid, stream: stream}
}

type pluginLogWriter struct {
	uuid   string
	stream string
}

func (w *pluginLogWriter) Write(p []byte) (n int, err error) {
	text := string(p)
	if text == "" {
		return len(p), nil
	}
	// 去除末尾换行
	if text[len(text)-1] == '\n' {
		text = text[:len(text)-1]
	}
	if text != "" {
		logs.Info("[插件 %s/%s] %s", w.uuid, w.stream, text)
	}
	return len(p), nil
}
