package storage

import (
	"sync"
	"time"

	"cdpnetool/pkg/model"
)

// EventRepo 事件历史仓库
type EventRepo struct {
	// 异步写入缓冲
	buffer    []InterceptEventRecord
	bufferMu  sync.Mutex
	batchSize int
	flushCh   chan struct{}
	stopCh    chan struct{}
	wg        sync.WaitGroup
}

// NewEventRepo 创建事件仓库实例
func NewEventRepo() *EventRepo {
	r := &EventRepo{
		buffer:    make([]InterceptEventRecord, 0, 100),
		batchSize: 50,
		flushCh:   make(chan struct{}, 1),
		stopCh:    make(chan struct{}),
	}
	// 启动异步写入协程
	r.wg.Add(1)
	go r.asyncWriter()
	return r
}

// asyncWriter 异步批量写入协程
func (r *EventRepo) asyncWriter() {
	defer r.wg.Done()
	ticker := time.NewTicker(5 * time.Second)
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
	r.buffer = make([]InterceptEventRecord, 0, 100)
	r.bufferMu.Unlock()

	// 批量插入
	if err := DB().CreateInBatches(toWrite, 100).Error; err != nil {
		// 记录错误但不阻塞
		_ = err
	}
}

// Stop 停止异步写入
func (r *EventRepo) Stop() {
	close(r.stopCh)
	r.wg.Wait()
}

// Record 记录事件（异步）
func (r *EventRepo) Record(evt model.Event) {
	record := InterceptEventRecord{
		SessionID:  string(evt.Session),
		TargetID:   string(evt.Target),
		Type:       evt.Type,
		URL:        evt.URL,
		Method:     evt.Method,
		Stage:      evt.Stage,
		StatusCode: evt.StatusCode,
		Error:      evt.Error,
		Timestamp:  evt.Timestamp,
		CreatedAt:  time.Now(),
	}
	if evt.Rule != nil {
		ruleID := string(*evt.Rule)
		record.RuleID = &ruleID
	}

	r.bufferMu.Lock()
	r.buffer = append(r.buffer, record)
	needFlush := len(r.buffer) >= r.batchSize
	r.bufferMu.Unlock()

	if needFlush {
		select {
		case r.flushCh <- struct{}{}:
		default:
		}
	}
}

// Query 查询事件历史
func (r *EventRepo) Query(opts QueryOptions) ([]InterceptEventRecord, int64, error) {
	query := DB().Model(&InterceptEventRecord{})

	// 应用过滤条件
	if opts.SessionID != "" {
		query = query.Where("session_id = ?", opts.SessionID)
	}
	if opts.Type != "" {
		query = query.Where("type = ?", opts.Type)
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

	var records []InterceptEventRecord
	err := query.Order("timestamp DESC").
		Offset(opts.Offset).
		Limit(opts.Limit).
		Find(&records).Error

	return records, total, err
}

// QueryOptions 查询选项
type QueryOptions struct {
	SessionID string
	Type      string
	URL       string
	Method    string
	StartTime int64
	EndTime   int64
	Offset    int
	Limit     int
}

// DeleteOldEvents 删除旧事件（数据清理）
func (r *EventRepo) DeleteOldEvents(beforeTimestamp int64) (int64, error) {
	result := DB().Where("timestamp < ?", beforeTimestamp).Delete(&InterceptEventRecord{})
	return result.RowsAffected, result.Error
}

// DeleteBySession 删除指定会话的事件
func (r *EventRepo) DeleteBySession(sessionID string) error {
	return DB().Where("session_id = ?", sessionID).Delete(&InterceptEventRecord{}).Error
}

// GetStats 获取事件统计
func (r *EventRepo) GetStats() (*EventStats, error) {
	var stats EventStats

	// 总数
	if err := DB().Model(&InterceptEventRecord{}).Count(&stats.Total).Error; err != nil {
		return nil, err
	}

	// 按类型统计
	type typeCount struct {
		Type  string
		Count int64
	}
	var typeCounts []typeCount
	if err := DB().Model(&InterceptEventRecord{}).
		Select("type, count(*) as count").
		Group("type").
		Find(&typeCounts).Error; err != nil {
		return nil, err
	}
	stats.ByType = make(map[string]int64)
	for _, tc := range typeCounts {
		stats.ByType[tc.Type] = tc.Count
	}

	return &stats, nil
}

// EventStats 事件统计
type EventStats struct {
	Total  int64            `json:"total"`
	ByType map[string]int64 `json:"byType"`
}

// CleanupOldEvents 根据保留天数清理旧事件
func (r *EventRepo) CleanupOldEvents(retentionDays int) (int64, error) {
	if retentionDays <= 0 {
		retentionDays = 7 // 默认保留 7 天
	}
	cutoff := time.Now().AddDate(0, 0, -retentionDays).UnixMilli()
	return r.DeleteOldEvents(cutoff)
}
