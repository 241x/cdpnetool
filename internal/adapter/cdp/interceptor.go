package cdp

import (
	"context"
	"time"

	"cdpnetool/internal/logger"
	"cdpnetool/internal/pool"

	"github.com/mafredri/cdp"
	"github.com/mafredri/cdp/protocol/fetch"
)

// Interceptor 物理拦截适配器
type Interceptor struct {
	log  logger.Logger
	pool *pool.Pool
}

// NewInterceptor 创建物理拦截适配器
func NewInterceptor(l logger.Logger, p *pool.Pool) *Interceptor {
	if l == nil {
		l = logger.NewNop()
	}
	return &Interceptor{log: l, pool: p}
}

// Enable 开启指定 Client 的拦截
func (i *Interceptor) Enable(ctx context.Context, client *cdp.Client) error {
	p := "*"
	patterns := []fetch.RequestPattern{
		{URLPattern: &p, RequestStage: fetch.RequestStageRequest},
		{URLPattern: &p, RequestStage: fetch.RequestStageResponse},
	}
	return client.Fetch.Enable(ctx, &fetch.EnableArgs{Patterns: patterns})
}

// Disable 关闭指定 Client 的拦截
func (i *Interceptor) Disable(ctx context.Context, client *cdp.Client) error {
	return client.Fetch.Disable(ctx)
}

// ContinueRequest 直接放行请求
func (i *Interceptor) ContinueRequest(ctx context.Context, client *cdp.Client, id fetch.RequestID) error {
	ctx2, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()
	err := client.Fetch.ContinueRequest(ctx2, &fetch.ContinueRequestArgs{RequestID: id})
	if err != nil {
		i.log.Err(err, "物理放行请求失败", "requestID", id)
	}
	return err
}

// ContinueResponse 直接放行响应
func (i *Interceptor) ContinueResponse(ctx context.Context, client *cdp.Client, id fetch.RequestID) error {
	ctx2, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()
	err := client.Fetch.ContinueResponse(ctx2, &fetch.ContinueResponseArgs{RequestID: id})
	if err != nil {
		i.log.Err(err, "物理放行响应失败", "requestID", id)
	}
	return err
}

// Consume 开启事件消费循环
func (i *Interceptor) Consume(ctx context.Context, client *cdp.Client, handler func(ev *fetch.RequestPausedReply)) {
	rp, err := client.Fetch.RequestPaused(ctx)
	if err != nil {
		i.log.Err(err, "订阅拦截事件流失败")
		return
	}
	defer rp.Close()

	for {
		ev, err := rp.Recv()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				i.log.Err(err, "接收拦截事件失败")
				return
			}
		}

		// 调试日志：接收到 CDP 事件
		stage := "request"
		if ev.ResponseStatusCode != nil {
			stage = "response"
		}
		i.log.Debug("[Interceptor] 接收 CDP 事件", "requestID", ev.RequestID, "stage", stage, "url", ev.Request.URL)

		if i.pool != nil {
			submitted := i.pool.Submit(func() {
				defer func() {
					if r := recover(); r != nil {
						i.log.Err(nil, "handler panic 捕获", "requestID", ev.RequestID, "panic", r)
						// 尝试降级放行
						if ev.ResponseStatusCode == nil {
							_ = i.ContinueRequest(ctx, client, ev.RequestID)
						} else {
							_ = i.ContinueResponse(ctx, client, ev.RequestID)
						}
					}
				}()
				handler(ev)
			})
			if !submitted {
				i.log.Warn("[Interceptor] 并发池已满，执行降级放行", "requestID", ev.RequestID, "url", ev.Request.URL)
				if ev.ResponseStatusCode == nil {
					if err := i.ContinueRequest(ctx, client, ev.RequestID); err != nil {
						i.log.Err(err, "降级放行请求失败", "requestID", ev.RequestID)
					}
				} else {
					if err := i.ContinueResponse(ctx, client, ev.RequestID); err != nil {
						i.log.Err(err, "降级放行响应失败", "requestID", ev.RequestID)
					}
				}
			}
		} else {
			go func(ev *fetch.RequestPausedReply) {
				defer func() {
					if r := recover(); r != nil {
						i.log.Err(nil, "handler panic 捕获", "requestID", ev.RequestID, "panic", r)
						// 尝试降级放行
						if ev.ResponseStatusCode == nil {
							_ = i.ContinueRequest(ctx, client, ev.RequestID)
						} else {
							_ = i.ContinueResponse(ctx, client, ev.RequestID)
						}
					}
				}()
				handler(ev)
			}(ev)
		}
	}
}
