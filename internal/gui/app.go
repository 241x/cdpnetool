package gui

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"cdpnetool/internal/browser"
	"cdpnetool/internal/storage"
	"cdpnetool/pkg/api"
	"cdpnetool/pkg/model"
	"cdpnetool/pkg/rulespec"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App 暴露给前端的方法集合
type App struct {
	ctx     context.Context
	service api.Service

	// 当前活跃的 session（简化版，后续可支持多 session）
	currentSession model.SessionID

	// 已启动的浏览器进程
	browser *browser.Browser

	// 存储仓库
	settingsRepo *storage.SettingsRepo
	ruleSetRepo  *storage.RuleSetRepo
	eventRepo    *storage.EventRepo
}

// NewApp 创建 App 实例
func NewApp() *App {
	return &App{
		service:      api.NewService(),
		settingsRepo: storage.NewSettingsRepo(),
		ruleSetRepo:  storage.NewRuleSetRepo(),
	}
}

// Startup 由 Wails 在应用启动时调用
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx

	// 初始化数据库
	if err := storage.Init(); err != nil {
		fmt.Printf("数据库初始化失败: %v\n", err)
	}

	// 初始化事件仓库（异步写入）
	a.eventRepo = storage.NewEventRepo()
}

// Shutdown 由 Wails 在应用关闭时调用
func (a *App) Shutdown(ctx context.Context) {
	if a.currentSession != "" {
		_ = a.service.StopSession(a.currentSession)
	}
	// 关闭启动的浏览器
	if a.browser != nil {
		_ = a.browser.Stop(2 * time.Second)
	}
	// 停止事件异步写入
	if a.eventRepo != nil {
		a.eventRepo.Stop()
	}
	// 关闭数据库连接
	_ = storage.Close()
}

// ========== Session 管理 ==========

// SessionResult 返回给前端的会话结果
type SessionResult struct {
	SessionID string `json:"sessionId"`
	Success   bool   `json:"success"`
	Error     string `json:"error,omitempty"`
}

// StartSession 创建拦截会话
func (a *App) StartSession(devToolsURL string) SessionResult {
	cfg := model.SessionConfig{
		DevToolsURL: devToolsURL,
	}
	sid, err := a.service.StartSession(cfg)
	if err != nil {
		return SessionResult{Success: false, Error: err.Error()}
	}
	a.currentSession = sid
	// 启动事件订阅
	go a.subscribeEvents(sid)
	// 启动 Pending 订阅
	go a.subscribePending(sid)
	return SessionResult{SessionID: string(sid), Success: true}
}

// StopSession 停止会话
func (a *App) StopSession(sessionID string) SessionResult {
	err := a.service.StopSession(model.SessionID(sessionID))
	if err != nil {
		return SessionResult{Success: false, Error: err.Error()}
	}
	if a.currentSession == model.SessionID(sessionID) {
		a.currentSession = ""
	}
	return SessionResult{Success: true}
}

// GetCurrentSession 获取当前活跃会话
func (a *App) GetCurrentSession() string {
	return string(a.currentSession)
}

// ========== Target 管理 ==========

// TargetListResult 返回给前端的目标列表
type TargetListResult struct {
	Targets []model.TargetInfo `json:"targets"`
	Success bool               `json:"success"`
	Error   string             `json:"error,omitempty"`
}

// ListTargets 列出浏览器页面目标
func (a *App) ListTargets(sessionID string) TargetListResult {
	targets, err := a.service.ListTargets(model.SessionID(sessionID))
	if err != nil {
		return TargetListResult{Success: false, Error: err.Error()}
	}
	return TargetListResult{Targets: targets, Success: true}
}

// OperationResult 通用操作结果
type OperationResult struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// AttachTarget 附加指定页面目标
func (a *App) AttachTarget(sessionID, targetID string) OperationResult {
	err := a.service.AttachTarget(model.SessionID(sessionID), model.TargetID(targetID))
	if err != nil {
		return OperationResult{Success: false, Error: err.Error()}
	}
	return OperationResult{Success: true}
}

// DetachTarget 移除指定页面目标
func (a *App) DetachTarget(sessionID, targetID string) OperationResult {
	err := a.service.DetachTarget(model.SessionID(sessionID), model.TargetID(targetID))
	if err != nil {
		return OperationResult{Success: false, Error: err.Error()}
	}
	return OperationResult{Success: true}
}

// ========== 拦截控制 ==========

// EnableInterception 启用拦截
func (a *App) EnableInterception(sessionID string) OperationResult {
	err := a.service.EnableInterception(model.SessionID(sessionID))
	if err != nil {
		return OperationResult{Success: false, Error: err.Error()}
	}
	return OperationResult{Success: true}
}

// DisableInterception 停用拦截
func (a *App) DisableInterception(sessionID string) OperationResult {
	err := a.service.DisableInterception(model.SessionID(sessionID))
	if err != nil {
		return OperationResult{Success: false, Error: err.Error()}
	}
	return OperationResult{Success: true}
}

// ========== 规则管理 ==========

// LoadRules 从 JSON 字符串加载规则
func (a *App) LoadRules(sessionID string, rulesJSON string) OperationResult {
	var rs rulespec.RuleSet
	if err := json.Unmarshal([]byte(rulesJSON), &rs); err != nil {
		return OperationResult{Success: false, Error: "JSON 解析失败: " + err.Error()}
	}
	err := a.service.LoadRules(model.SessionID(sessionID), rs)
	if err != nil {
		return OperationResult{Success: false, Error: err.Error()}
	}
	return OperationResult{Success: true}
}

// StatsResult 规则统计结果
type StatsResult struct {
	Stats   model.EngineStats `json:"stats"`
	Success bool              `json:"success"`
	Error   string            `json:"error,omitempty"`
}

// GetRuleStats 获取规则命中统计
func (a *App) GetRuleStats(sessionID string) StatsResult {
	stats, err := a.service.GetRuleStats(model.SessionID(sessionID))
	if err != nil {
		return StatsResult{Success: false, Error: err.Error()}
	}
	return StatsResult{Stats: stats, Success: true}
}

// ========== 事件推送 ==========

// subscribeEvents 订阅拦截事件并推送到前端
func (a *App) subscribeEvents(sessionID model.SessionID) {
	ch, err := a.service.SubscribeEvents(sessionID)
	if err != nil {
		return
	}
	for evt := range ch {
		// 通过 Wails 事件系统推送到前端
		runtime.EventsEmit(a.ctx, "intercept-event", evt)
		// 异步写入数据库
		if a.eventRepo != nil {
			a.eventRepo.Record(evt)
		}
	}
}

// ========== Pending 审批 ==========

// PendingListResult 待审批列表结果
type PendingListResult struct {
	Items   []model.PendingItem `json:"items"`
	Success bool                `json:"success"`
	Error   string              `json:"error,omitempty"`
}

// subscribePending 订阅 Pending 事件并推送到前端
func (a *App) subscribePending(sessionID model.SessionID) {
	ch, err := a.service.SubscribePending(sessionID)
	if err != nil {
		return
	}
	for item := range ch {
		// 通过 Wails 事件系统推送到前端
		runtime.EventsEmit(a.ctx, "pending-item", item)
	}
}

// ApproveRequest 审批请求阶段
func (a *App) ApproveRequest(itemID string, mutationsJSON string) OperationResult {
	var mutations rulespec.Rewrite
	if mutationsJSON != "" {
		if err := json.Unmarshal([]byte(mutationsJSON), &mutations); err != nil {
			return OperationResult{Success: false, Error: "JSON 解析失败: " + err.Error()}
		}
	}
	err := a.service.ApproveRequest(itemID, mutations)
	if err != nil {
		return OperationResult{Success: false, Error: err.Error()}
	}
	return OperationResult{Success: true}
}

// ApproveResponse 审批响应阶段
func (a *App) ApproveResponse(itemID string, mutationsJSON string) OperationResult {
	var mutations rulespec.Rewrite
	if mutationsJSON != "" {
		if err := json.Unmarshal([]byte(mutationsJSON), &mutations); err != nil {
			return OperationResult{Success: false, Error: "JSON 解析失败: " + err.Error()}
		}
	}
	err := a.service.ApproveResponse(itemID, mutations)
	if err != nil {
		return OperationResult{Success: false, Error: err.Error()}
	}
	return OperationResult{Success: true}
}

// Reject 拒绝审批项
func (a *App) Reject(itemID string) OperationResult {
	err := a.service.Reject(itemID)
	if err != nil {
		return OperationResult{Success: false, Error: err.Error()}
	}
	return OperationResult{Success: true}
}

// ========== 浏览器管理 ==========

// LaunchBrowserResult 启动浏览器结果
type LaunchBrowserResult struct {
	DevToolsURL string `json:"devToolsUrl"`
	Success     bool   `json:"success"`
	Error       string `json:"error,omitempty"`
}

// LaunchBrowser 启动新的浏览器实例
func (a *App) LaunchBrowser(headless bool) LaunchBrowserResult {
	// 如果已有浏览器运行，先关闭
	if a.browser != nil {
		_ = a.browser.Stop(2 * time.Second)
		a.browser = nil
	}

	opts := browser.Options{
		Headless: headless,
	}

	b, err := browser.Start(opts)
	if err != nil {
		return LaunchBrowserResult{Success: false, Error: err.Error()}
	}

	a.browser = b
	return LaunchBrowserResult{DevToolsURL: b.DevToolsURL, Success: true}
}

// CloseBrowser 关闭已启动的浏览器
func (a *App) CloseBrowser() OperationResult {
	if a.browser == nil {
		return OperationResult{Success: false, Error: "没有正在运行的浏览器"}
	}

	err := a.browser.Stop(2 * time.Second)
	a.browser = nil
	if err != nil {
		return OperationResult{Success: false, Error: err.Error()}
	}
	return OperationResult{Success: true}
}

// GetBrowserStatus 获取浏览器状态
func (a *App) GetBrowserStatus() LaunchBrowserResult {
	if a.browser == nil {
		return LaunchBrowserResult{Success: false}
	}
	return LaunchBrowserResult{DevToolsURL: a.browser.DevToolsURL, Success: true}
}

// ========== 设置管理 ==========

// SettingsResult 设置结果
type SettingsResult struct {
	Settings map[string]string `json:"settings"`
	Success  bool              `json:"success"`
	Error    string            `json:"error,omitempty"`
}

// GetAllSettings 获取所有设置
func (a *App) GetAllSettings() SettingsResult {
	settings, err := a.settingsRepo.GetAll()
	if err != nil {
		return SettingsResult{Success: false, Error: err.Error()}
	}
	return SettingsResult{Settings: settings, Success: true}
}

// GetSetting 获取单个设置
func (a *App) GetSetting(key string) string {
	return a.settingsRepo.GetWithDefault(key, "")
}

// SetSetting 设置单个值
func (a *App) SetSetting(key, value string) OperationResult {
	if err := a.settingsRepo.Set(key, value); err != nil {
		return OperationResult{Success: false, Error: err.Error()}
	}
	return OperationResult{Success: true}
}

// SetMultipleSettings 批量设置
func (a *App) SetMultipleSettings(settingsJSON string) OperationResult {
	var settings map[string]string
	if err := json.Unmarshal([]byte(settingsJSON), &settings); err != nil {
		return OperationResult{Success: false, Error: "JSON 解析失败: " + err.Error()}
	}
	if err := a.settingsRepo.SetMultiple(settings); err != nil {
		return OperationResult{Success: false, Error: err.Error()}
	}
	return OperationResult{Success: true}
}

// ========== 规则集存储 ==========

// RuleSetListResult 规则集列表结果
type RuleSetListResult struct {
	RuleSets []storage.RuleSetRecord `json:"ruleSets"`
	Success  bool                    `json:"success"`
	Error    string                  `json:"error,omitempty"`
}

// RuleSetResult 单个规则集结果
type RuleSetResult struct {
	RuleSet *storage.RuleSetRecord `json:"ruleSet"`
	Success bool                   `json:"success"`
	Error   string                 `json:"error,omitempty"`
}

// ListRuleSets 列出所有规则集
func (a *App) ListRuleSets() RuleSetListResult {
	ruleSets, err := a.ruleSetRepo.List()
	if err != nil {
		return RuleSetListResult{Success: false, Error: err.Error()}
	}
	return RuleSetListResult{RuleSets: ruleSets, Success: true}
}

// GetRuleSet 获取规则集
func (a *App) GetRuleSet(id uint) RuleSetResult {
	ruleSet, err := a.ruleSetRepo.GetByID(id)
	if err != nil {
		return RuleSetResult{Success: false, Error: err.Error()}
	}
	return RuleSetResult{RuleSet: ruleSet, Success: true}
}

// SaveRuleSet 保存规则集（创建或更新）
func (a *App) SaveRuleSet(id uint, name string, rulesJSON string) RuleSetResult {
	var rs rulespec.RuleSet
	if err := json.Unmarshal([]byte(rulesJSON), &rs); err != nil {
		return RuleSetResult{Success: false, Error: "JSON 解析失败: " + err.Error()}
	}

	ruleSet, err := a.ruleSetRepo.SaveFromRuleSet(id, name, &rs)
	if err != nil {
		return RuleSetResult{Success: false, Error: err.Error()}
	}
	return RuleSetResult{RuleSet: ruleSet, Success: true}
}

// DeleteRuleSet 删除规则集
func (a *App) DeleteRuleSet(id uint) OperationResult {
	if err := a.ruleSetRepo.Delete(id); err != nil {
		return OperationResult{Success: false, Error: err.Error()}
	}
	return OperationResult{Success: true}
}

// SetActiveRuleSet 设置激活的规则集
func (a *App) SetActiveRuleSet(id uint) OperationResult {
	if err := a.ruleSetRepo.SetActive(id); err != nil {
		return OperationResult{Success: false, Error: err.Error()}
	}
	// 记住上次使用的规则集
	_ = a.settingsRepo.SetLastRuleSetID(fmt.Sprintf("%d", id))
	return OperationResult{Success: true}
}

// GetActiveRuleSet 获取激活的规则集
func (a *App) GetActiveRuleSet() RuleSetResult {
	ruleSet, err := a.ruleSetRepo.GetActive()
	if err != nil {
		return RuleSetResult{Success: false, Error: err.Error()}
	}
	return RuleSetResult{RuleSet: ruleSet, Success: true}
}

// RenameRuleSet 重命名规则集
func (a *App) RenameRuleSet(id uint, newName string) OperationResult {
	if err := a.ruleSetRepo.Rename(id, newName); err != nil {
		return OperationResult{Success: false, Error: err.Error()}
	}
	return OperationResult{Success: true}
}

// DuplicateRuleSet 复制规则集
func (a *App) DuplicateRuleSet(id uint, newName string) RuleSetResult {
	ruleSet, err := a.ruleSetRepo.Duplicate(id, newName)
	if err != nil {
		return RuleSetResult{Success: false, Error: err.Error()}
	}
	return RuleSetResult{RuleSet: ruleSet, Success: true}
}

// LoadActiveRuleSetToSession 加载激活规则集到当前会话
func (a *App) LoadActiveRuleSetToSession() OperationResult {
	if a.currentSession == "" {
		return OperationResult{Success: false, Error: "没有活跃会话"}
	}

	ruleSet, err := a.ruleSetRepo.GetActive()
	if err != nil {
		return OperationResult{Success: false, Error: err.Error()}
	}
	if ruleSet == nil {
		return OperationResult{Success: false, Error: "没有激活的规则集"}
	}

	rs, err := a.ruleSetRepo.ToRuleSet(ruleSet)
	if err != nil {
		return OperationResult{Success: false, Error: err.Error()}
	}

	if err := a.service.LoadRules(a.currentSession, *rs); err != nil {
		return OperationResult{Success: false, Error: err.Error()}
	}
	return OperationResult{Success: true}
}

// ========== 事件历史 ==========

// EventHistoryResult 事件历史结果
type EventHistoryResult struct {
	Events  []storage.InterceptEventRecord `json:"events"`
	Total   int64                          `json:"total"`
	Success bool                           `json:"success"`
	Error   string                         `json:"error,omitempty"`
}

// QueryEventHistory 查询事件历史
func (a *App) QueryEventHistory(sessionID, eventType, url, method string, startTime, endTime int64, offset, limit int) EventHistoryResult {
	if a.eventRepo == nil {
		return EventHistoryResult{Success: false, Error: "事件仓库未初始化"}
	}

	events, total, err := a.eventRepo.Query(storage.QueryOptions{
		SessionID: sessionID,
		Type:      eventType,
		URL:       url,
		Method:    method,
		StartTime: startTime,
		EndTime:   endTime,
		Offset:    offset,
		Limit:     limit,
	})
	if err != nil {
		return EventHistoryResult{Success: false, Error: err.Error()}
	}
	return EventHistoryResult{Events: events, Total: total, Success: true}
}

// GetEventStats 获取事件统计
func (a *App) GetEventStats() struct {
	Stats   *storage.EventStats `json:"stats"`
	Success bool                `json:"success"`
	Error   string              `json:"error,omitempty"`
} {
	if a.eventRepo == nil {
		return struct {
			Stats   *storage.EventStats `json:"stats"`
			Success bool                `json:"success"`
			Error   string              `json:"error,omitempty"`
		}{Success: false, Error: "事件仓库未初始化"}
	}

	stats, err := a.eventRepo.GetStats()
	if err != nil {
		return struct {
			Stats   *storage.EventStats `json:"stats"`
			Success bool                `json:"success"`
			Error   string              `json:"error,omitempty"`
		}{Success: false, Error: err.Error()}
	}
	return struct {
		Stats   *storage.EventStats `json:"stats"`
		Success bool                `json:"success"`
		Error   string              `json:"error,omitempty"`
	}{Stats: stats, Success: true}
}

// CleanupEventHistory 清理旧事件
func (a *App) CleanupEventHistory(retentionDays int) OperationResult {
	if a.eventRepo == nil {
		return OperationResult{Success: false, Error: "事件仓库未初始化"}
	}

	_, err := a.eventRepo.CleanupOldEvents(retentionDays)
	if err != nil {
		return OperationResult{Success: false, Error: err.Error()}
	}
	return OperationResult{Success: true}
}
