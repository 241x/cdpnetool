package interceptor

import (
	"context"
	"sync"
	"time"

	"cdpnetool/internal/logger"
	"cdpnetool/internal/pool"

	"github.com/mafredri/cdp"
	"github.com/mafredri/cdp/protocol/fetch"
)

// Interceptor 拦截控制器，负责管理拦截功能的启用/禁用和事件流消费
type Interceptor struct {
	stateMu sync.RWMutex
	enabled bool
	pool    *pool.Pool
	handler HandlerFunc
	log     logger.Logger
}

// HandlerFunc 事件处理函数类型
// 参数：client, ctx, event
type HandlerFunc func(client *cdp.Client, ctx context.Context, ev *fetch.RequestPausedReply)

// New 创建拦截控制器
func New(handler HandlerFunc, log logger.Logger) *Interceptor {
	if log == nil {
		log = logger.NewNop()
	}
	return &Interceptor{
		handler: handler,
		log:     log,
	}
}

// EnableTarget 为单个目标启用拦截
func (i *Interceptor) EnableTarget(client *cdp.Client, ctx context.Context) error {
	if client == nil {
		return nil
	}

	// 启用 Network
	if err := client.Network.Enable(ctx, nil); err != nil {
		return err
	}

	// 启用 Fetch
	p := "*"
	patterns := []fetch.RequestPattern{
		{URLPattern: &p, RequestStage: fetch.RequestStageRequest},
		{URLPattern: &p, RequestStage: fetch.RequestStageResponse},
	}
	if err := client.Fetch.Enable(ctx, &fetch.EnableArgs{Patterns: patterns}); err != nil {
		return err
	}

	// 如果已配置 worker pool 且未启动，现在启动
	if i.pool != nil && i.pool.IsEnabled() {
		i.pool.SetLogger(i.log)
		i.pool.Start(ctx)
	}

	// 启动事件消费
	go i.consume(client, ctx)
	return nil
}

// DisableTarget 为单个目标禁用拦截
func (i *Interceptor) DisableTarget(client *cdp.Client, ctx context.Context) error {
	if client == nil {
		return nil
	}
	return client.Fetch.Disable(ctx)
}

// consume 消费拦截事件流
func (i *Interceptor) consume(client *cdp.Client, ctx context.Context) {
	rp, err := client.Fetch.RequestPaused(ctx)
	if err != nil {
		i.log.Err(err, "订阅拦截事件流失败")
		return
	}
	defer rp.Close()

	i.log.Info("开始消费拦截事件流")
	for {
		ev, err := rp.Recv()
		if err != nil {
			i.log.Err(err, "接收拦截事件失败")
			return
		}
		i.dispatchPaused(client, ctx, ev)
	}
}

// dispatchPaused 调度单次事件处理
func (i *Interceptor) dispatchPaused(client *cdp.Client, ctx context.Context, ev *fetch.RequestPausedReply) {
	if i.pool == nil {
		go i.handler(client, ctx, ev)
		return
	}
	submitted := i.pool.Submit(func() {
		i.handler(client, ctx, ev)
	})
	if !submitted {
		i.degradeAndContinue(client, ctx, ev, "并发队列已满")
	}
}

// degradeAndContinue 降级处理：直接放行
func (i *Interceptor) degradeAndContinue(client *cdp.Client, ctx context.Context, ev *fetch.RequestPausedReply, reason string) {
	i.log.Warn("执行降级策略：直接放行", "reason", reason, "requestID", ev.RequestID)
	ctx2, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()

	var err error
	if ev.ResponseStatusCode == nil {
		err = client.Fetch.ContinueRequest(ctx2, &fetch.ContinueRequestArgs{RequestID: ev.RequestID})
	} else {
		err = client.Fetch.ContinueResponse(ctx2, &fetch.ContinueResponseArgs{RequestID: ev.RequestID})
	}

	if err != nil {
		i.log.Warn("降级策略执行失败", "error", err, "requestID", ev.RequestID)
	}
}

// SetPool 设置并发池
func (i *Interceptor) SetPool(p *pool.Pool) {
	i.pool = p
}

// IsEnabled 检查是否启用
func (i *Interceptor) IsEnabled() bool {
	i.stateMu.RLock()
	defer i.stateMu.RUnlock()
	return i.enabled
}

// SetEnabled 设置启用状态
func (i *Interceptor) SetEnabled(enabled bool) {
	i.stateMu.Lock()
	defer i.stateMu.Unlock()
	i.enabled = enabled
}

// GetPoolStats 返回并发工作池的运行统计
func (i *Interceptor) GetPoolStats() (queueLen, queueCap, totalSubmit, totalDrop int64) {
	if i.pool == nil {
		return 0, 0, 0, 0
	}
	return i.pool.Stats()
}
