package storage

import (
	"encoding/json"
	"sync"
	"time"

	"cdpnetool/pkg/model"
)

// EventRepo 事件仓库（只存储匹配事件到数据库）
type EventRepo struct {
	db        *DB
	buffer    []MatchedEventRecord
	bufferMu  sync.Mutex
	batchSize int
	flushCh   chan struct{}
	stopCh    chan struct{}
	wg        sync.WaitGroup
}

// NewEventRepo 创建事件仓库实例
func NewEventRepo(db *DB) *EventRepo {
	r := &EventRepo{
		db:        db,
		buffer:    make([]MatchedEventRecord, 0, 100),
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
	r.buffer = make([]MatchedEventRecord, 0, 100)
	r.bufferMu.Unlock()

	// 批量插入
	if err := r.db.GormDB().CreateInBatches(toWrite, 100).Error; err != nil {
		// 记录错误但不阻塞
		_ = err
	}
}

// Stop 停止异步写入
func (r *EventRepo) Stop() {
	close(r.stopCh)
	r.wg.Wait()
}

// RecordMatched 记录匹配事件（异步写入数据库）
func (r *EventRepo) RecordMatched(evt *model.MatchedEvent) {
	// 序列化规则列表
	matchedRulesJSON, _ := json.Marshal(evt.MatchedRules)
	originalJSON, _ := json.Marshal(evt.Original)
	modifiedJSON, _ := json.Marshal(evt.Modified)

	record := MatchedEventRecord{
		SessionID:        string(evt.Session),
		TargetID:         string(evt.Target),
		URL:              evt.URL,
		Method:           evt.Method,
		Stage:            evt.Stage,
		StatusCode:       evt.StatusCode,
		FinalResult:      evt.FinalResult,
		MatchedRulesJSON: string(matchedRulesJSON),
		OriginalJSON:     string(originalJSON),
		ModifiedJSON:     string(modifiedJSON),
		Timestamp:        evt.Timestamp,
		CreatedAt:        time.Now(),
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
func (r *EventRepo) Query(opts QueryOptions) ([]MatchedEventRecord, int64, error) {
	query := r.db.GormDB().Model(&MatchedEventRecord{})

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

	var records []MatchedEventRecord
	err := query.Order("timestamp DESC").
		Offset(opts.Offset).
		Limit(opts.Limit).
		Find(&records).Error

	return records, total, err
}

// GetByID 根据ID获取事件
func (r *EventRepo) GetByID(id uint) (*MatchedEventRecord, error) {
	var record MatchedEventRecord
	err := r.db.GormDB().First(&record, id).Error
	if err != nil {
		return nil, err
	}
	return &record, nil
}

// DeleteOldEvents 删除旧事件（数据清理）
func (r *EventRepo) DeleteOldEvents(beforeTimestamp int64) (int64, error) {
	result := r.db.GormDB().Where("timestamp < ?", beforeTimestamp).Delete(&MatchedEventRecord{})
	return result.RowsAffected, result.Error
}

// DeleteBySession 删除指定会话的事件
func (r *EventRepo) DeleteBySession(sessionID string) error {
	return r.db.GormDB().Where("session_id = ?", sessionID).Delete(&MatchedEventRecord{}).Error
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
	return r.db.GormDB().Where("1 = 1").Delete(&MatchedEventRecord{}).Error
}
