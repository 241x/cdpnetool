package repo

import (
	"encoding/json"
	"sync"
	"time"

	"cdpnetool/internal/logger"
	"cdpnetool/internal/storage/model"
	"cdpnetool/pkg/domain"

	"gorm.io/gorm"
)

// EventRepoOptions 事件仓库配置选项
type EventRepoOptions struct {
	BatchSize     int           // 批量写入大小
	FlushInterval time.Duration // 自动刷新间隔
	MaxBufferSize int           // 缓冲区最大容量（防止内存溢出）
}

// DefaultEventRepoOptions 默认配置
func DefaultEventRepoOptions() EventRepoOptions {
	return EventRepoOptions{
		BatchSize:     50,
		FlushInterval: 5 * time.Second,
		MaxBufferSize: 1000,
	}
}

// EventRepo 事件仓库（存储匹配事件到数据库）
type EventRepo struct {
	BaseRepository[model.NetworkEventRecord]
	log      logger.Logger
	opts     EventRepoOptions
	buffer   []model.NetworkEventRecord
	bufferMu sync.Mutex
	flushCh  chan struct{}
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

// NewEventRepo 创建事件仓库实例
func NewEventRepo(db *gorm.DB, log logger.Logger, opts ...EventRepoOptions) *EventRepo {
	opt := DefaultEventRepoOptions()
	if len(opts) > 0 {
		opt = opts[0]
	}

	r := &EventRepo{
		BaseRepository: *NewBaseRepository[model.NetworkEventRecord](db),
		log:            log,
		opts:           opt,
		buffer:         make([]model.NetworkEventRecord, 0, opt.BatchSize),
		flushCh:        make(chan struct{}, 1),
		stopCh:         make(chan struct{}),
	}
	// 启动异步写入协程
	r.wg.Add(1)
	go r.asyncWriter()
	return r
}

// asyncWriter 异步批量写入协程
func (r *EventRepo) asyncWriter() {
	defer r.wg.Done()
	ticker := time.NewTicker(r.opts.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-r.stopCh:
			// 停止前刷新剩余数据
			r.flush()
			return
		case <-ticker.C:
			r.flush()
		case <-r.flushCh:
			r.flush()
		}
	}
}

// flush 刷新缓冲区到数据库
func (r *EventRepo) flush() {
	r.bufferMu.Lock()
	if len(r.buffer) == 0 {
		r.bufferMu.Unlock()
		return
	}
	toWrite := r.buffer
	r.buffer = make([]model.NetworkEventRecord, 0, r.opts.BatchSize)
	r.bufferMu.Unlock()

	// 批量插入
	if err := r.Db.CreateInBatches(toWrite, r.opts.BatchSize).Error; err != nil {
		r.log.Error("批量保存匹配事件到数据库失败", "error", err, "count", len(toWrite))
	}
}

// Stop 停止异步写入
func (r *EventRepo) Stop() {
	close(r.stopCh)
	r.wg.Wait()
}

// Record 记录网络事件（异步写入数据库，只存储匹配事件）
func (r *EventRepo) Record(evt *domain.NetworkEvent) {
	// 只记录匹配事件
	if !evt.IsMatched {
		return
	}

	r.bufferMu.Lock()
	// 容量保护：如果缓冲区已满，丢弃新事件并记录警告
	if len(r.buffer) >= r.opts.MaxBufferSize {
		r.bufferMu.Unlock()
		r.log.Warn("事件缓冲区已满，丢弃当前事件", "url", evt.Request.URL)
		return
	}
	r.bufferMu.Unlock()

	// 序列化规则列表
	matchedRulesJSON, _ := json.Marshal(evt.MatchedRules)
	requestJSON, _ := json.Marshal(evt.Request)
	responseJSON, _ := json.Marshal(evt.Response)

	record := model.NetworkEventRecord{
		SessionID:        string(evt.Session),
		TargetID:         string(evt.Target),
		URL:              evt.Request.URL,
		Method:           evt.Request.Method,
		StatusCode:       evt.Response.StatusCode,
		FinalResult:      evt.FinalResult,
		MatchedRulesJSON: string(matchedRulesJSON),
		RequestJSON:      string(requestJSON),
		ResponseJSON:     string(responseJSON),
		Timestamp:        evt.Timestamp,
		CreatedAt:        time.Now(),
	}

	r.bufferMu.Lock()
	r.buffer = append(r.buffer, record)
	needFlush := len(r.buffer) >= r.opts.BatchSize
	r.bufferMu.Unlock()

	if needFlush {
		select {
		case r.flushCh <- struct{}{}:
		default:
		}
	}
}

// QueryOptions 查询选项
type QueryOptions struct {
	SessionID   string
	FinalResult string // blocked / modified / passed
	URL         string
	Method      string
	StartTime   int64
	EndTime     int64
	Offset      int
	Limit       int
}

// Query 查询匹配事件历史
func (r *EventRepo) Query(opts QueryOptions) ([]model.NetworkEventRecord, int64, error) {
	query := r.Db.Model(&model.NetworkEventRecord{})

	// 应用过滤条件
	if opts.SessionID != "" {
		query = query.Where("session_id = ?", opts.SessionID)
	}
	if opts.FinalResult != "" {
		query = query.Where("final_result = ?", opts.FinalResult)
	}
	if opts.URL != "" {
		query = query.Where("url LIKE ?", "%"+opts.URL+"%")
	}
	if opts.Method != "" {
		query = query.Where("method = ?", opts.Method)
	}
	if opts.StartTime > 0 {
		query = query.Where("timestamp >= ?", opts.StartTime)
	}
	if opts.EndTime > 0 {
		query = query.Where("timestamp <= ?", opts.EndTime)
	}

	// 计算总数
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页
	if opts.Limit <= 0 {
		opts.Limit = 100
	}
	if opts.Limit > 1000 {
		opts.Limit = 1000
	}

	var records []model.NetworkEventRecord
	err := query.Order("timestamp DESC").
		Offset(opts.Offset).
		Limit(opts.Limit).
		Find(&records).Error

	return records, total, err
}

// DeleteOldEvents 删除旧事件（数据清理）
func (r *EventRepo) DeleteOldEvents(beforeTimestamp int64) (int64, error) {
	result := r.Db.Where("timestamp < ?", beforeTimestamp).Delete(&model.NetworkEventRecord{})
	return result.RowsAffected, result.Error
}

// DeleteBySession 删除指定会话的事件
func (r *EventRepo) DeleteBySession(sessionID string) error {
	return r.Db.Where("session_id = ?", sessionID).Delete(&model.NetworkEventRecord{}).Error
}

// CleanupOldEvents 根据保留天数清理旧事件
func (r *EventRepo) CleanupOldEvents(retentionDays int) (int64, error) {
	if retentionDays <= 0 {
		retentionDays = 7 // 默认保留 7 天
	}
	cutoff := time.Now().AddDate(0, 0, -retentionDays).UnixMilli()
	return r.DeleteOldEvents(cutoff)
}

// ClearAll 清空所有事件
func (r *EventRepo) ClearAll() error {
	return r.Db.Where("1 = 1").Delete(&model.NetworkEventRecord{}).Error
}
