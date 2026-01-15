package cdp

import (
	"context"
	"fmt"
	"sync"
	"time"

	logger "cdpnetool/internal/logger"
)

// workerPool 并发工作池，用于限制拦截事件的并发处理数量
type workerPool struct {
	sem         chan struct{}
	queue       chan func()
	queueCap    int
	log         logger.Logger
	totalSubmit int64
	totalDrop   int64
	mu          sync.Mutex
	stopMonitor chan struct{}
}

// newWorkerPool 创建工作池，size 为 0 表示无限制
func newWorkerPool(size int) *workerPool {
	if size <= 0 {
		return &workerPool{}
	}
	// 队列容量 = worker 数量 * 8，提供足够的突发请求缓冲
	return &workerPool{
		sem:      make(chan struct{}, size),
		queue:    make(chan func(), size*8),
		queueCap: size * 8,
	}
}

// setLogger 设置日志记录器
func (p *workerPool) setLogger(l logger.Logger) {
	p.log = l
}

// start 启动工作池，创建固定数量的 worker 协程
func (p *workerPool) start(ctx context.Context) {
	if p.sem == nil {
		return
	}
	for i := 0; i < cap(p.sem); i++ {
		go p.worker(ctx)
	}
	p.stopMonitor = make(chan struct{})
	go p.monitor(ctx)
}

// stop 停止监控协程
func (p *workerPool) stop() {
	if p.stopMonitor != nil {
		close(p.stopMonitor)
	}
}

// monitor 定期输出工作池状态监控日志
func (p *workerPool) monitor(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-p.stopMonitor:
			return
		case <-ticker.C:
			qLen, qCap, submit, drop := p.stats()
			if p.log != nil && submit > 0 {
				usage := float64(qLen) / float64(qCap) * 100
				dropRate := float64(drop) / float64(submit) * 100
				p.log.Info("工作池状态监控", "queueLen", qLen, "queueCap", qCap, "usage", fmt.Sprintf("%.1f%%", usage), "totalSubmit", submit, "totalDrop", drop, "dropRate", fmt.Sprintf("%.2f%%", dropRate))
			}
		}
	}
}

// worker 工作协程，从队列中取任务并执行
func (p *workerPool) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case fn := <-p.queue:
			if fn != nil {
				fn()
			}
		}
	}
}

// submit 提交任务到工作池，返回是否成功入队
func (p *workerPool) submit(fn func()) bool {
	if p.sem == nil {
		go fn()
		return true
	}
	p.mu.Lock()
	p.totalSubmit++
	p.mu.Unlock()
	select {
	case p.queue <- fn:
		return true
	default:
		p.mu.Lock()
		p.totalDrop++
		drop := p.totalDrop
		submit := p.totalSubmit
		p.mu.Unlock()
		if p.log != nil {
			p.log.Warn("工作池队列已满，任务被丢弃", "queueCap", p.queueCap, "totalSubmit", submit, "totalDrop", drop)
		}
		return false
	}
}

// stats 返回工作池统计信息
func (p *workerPool) stats() (queueLen, queueCap, totalSubmit, totalDrop int64) {
	if p.sem == nil {
		return 0, 0, 0, 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return int64(len(p.queue)), int64(p.queueCap), p.totalSubmit, p.totalDrop
}
