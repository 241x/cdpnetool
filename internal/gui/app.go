package gui

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"cdpnetool/internal/browser"
	"cdpnetool/internal/logger"
	"cdpnetool/internal/storage"
	"cdpnetool/pkg/api"
	"cdpnetool/pkg/model"
	"cdpnetool/pkg/rulespec"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App 是暴露给前端的 Wails 方法集合，负责管理会话、浏览器、规则和事件。
type App struct {
	ctx     context.Context
	log     logger.Logger
	service api.Service

	// currentSession 当前活跃的会话 ID（简化版，后续可支持多 session）
	currentSession model.SessionID

	// browser 已启动的浏览器进程实例
	browser *browser.Browser

	// 数据库连接
	db *storage.DB

	// 存储仓库
	settingsRepo *storage.SettingsRepo
	ruleSetRepo  *storage.RuleSetRepo
	eventRepo    *storage.EventRepo
}

// NewApp 创建并返回一个新的 App 实例。
func NewApp() *App {
	log := logger.NewDefaultLogger(logger.LogLevelInfo, nil)
	log.Debug("创建 App 实例")
	return &App{
		log:     log,
		service: api.NewService(log),
	}
}

// Startup 在应用启动时由 Wails 框架调用，完成数据库和事件仓库的初始化。
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
	a.log.Info("应用启动")

	// 初始化数据库
	db, err := storage.NewDB()
	if err != nil {
		a.log.Error("数据库初始化失败", "error", err)
		return
	}
	a.db = db

	// 初始化仓库
	a.settingsRepo = storage.NewSettingsRepo(db)
	a.ruleSetRepo = storage.NewRuleSetRepo(db)
	a.eventRepo = storage.NewEventRepo(db)
	a.log.Debug("事件仓库初始化完成")
}

// Shutdown 在应用关闭时由 Wails 框架调用，负责清理会话、浏览器和数据库资源。
func (a *App) Shutdown(ctx context.Context) {
	a.log.Info("应用关闭中...")

	if a.currentSession != "" {
		if err := a.service.StopSession(a.currentSession); err != nil {
			a.log.Error("停止会话失败", "sessionID", a.currentSession, "error", err)
		}
	}

	// 关闭启动的浏览器
	if a.browser != nil {
		if err := a.browser.Stop(2 * time.Second); err != nil {
			a.log.Error("关闭浏览器失败", "error", err)
		}
	}

	// 停止事件异步写入
	if a.eventRepo != nil {
		a.eventRepo.Stop()
	}

	// 关闭数据库连接
	if a.db != nil {
		if err := a.db.Close(); err != nil {
			a.log.Error("关闭数据库失败", "error", err)
		}
	}

	a.log.Info("应用已关闭")
}

// SessionResult 表示返回给前端的会话操作结果。
type SessionResult struct {
	SessionID string `json:"sessionId"`
	Success   bool   `json:"success"`
	Error     string `json:"error,omitempty"`
}

// StartSession 创建新的拦截会话，并启动事件订阅。
func (a *App) StartSession(devToolsURL string) SessionResult {
	a.log.Info("启动会话", "devToolsURL", devToolsURL)

	cfg := model.SessionConfig{
		DevToolsURL: devToolsURL,
	}
	sid, err := a.service.StartSession(cfg)
	if err != nil {
		a.log.Error("启动会话失败", "error", err)
		return SessionResult{Success: false, Error: err.Error()}
	}

	a.currentSession = sid
	// 启动事件订阅
	go a.subscribeEvents(sid)

	a.log.Info("会话启动成功", "sessionID", sid)
	return SessionResult{SessionID: string(sid), Success: true}
}

// StopSession 停止指定的会话。
func (a *App) StopSession(sessionID string) SessionResult {
	a.log.Info("停止会话", "sessionID", sessionID)

	err := a.service.StopSession(model.SessionID(sessionID))
	if err != nil {
		a.log.Error("停止会话失败", "sessionID", sessionID, "error", err)
		return SessionResult{Success: false, Error: err.Error()}
	}

	if a.currentSession == model.SessionID(sessionID) {
		a.currentSession = ""
	}
	return SessionResult{Success: true}
}

// GetCurrentSession 返回当前活跃会话的 ID。
func (a *App) GetCurrentSession() string {
	return string(a.currentSession)
}

// TargetListResult 表示返回给前端的目标列表结果。
type TargetListResult struct {
	Targets []model.TargetInfo `json:"targets"`
	Success bool               `json:"success"`
	Error   string             `json:"error,omitempty"`
}

// ListTargets 列出指定会话中的浏览器页面目标。
func (a *App) ListTargets(sessionID string) TargetListResult {
	targets, err := a.service.ListTargets(model.SessionID(sessionID))
	if err != nil {
		a.log.Error("列出目标失败", "sessionID", sessionID, "error", err)
		return TargetListResult{Success: false, Error: err.Error()}
	}
	return TargetListResult{Targets: targets, Success: true}
}

// OperationResult 表示通用操作的结果。
type OperationResult struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// AttachTarget 附加指定页面目标到会话进行拦截。
func (a *App) AttachTarget(sessionID, targetID string) OperationResult {
	err := a.service.AttachTarget(model.SessionID(sessionID), model.TargetID(targetID))
	if err != nil {
		a.log.Error("附加目标失败", "sessionID", sessionID, "targetID", targetID, "error", err)
		return OperationResult{Success: false, Error: err.Error()}
	}
	a.log.Debug("已附加目标", "targetID", targetID)
	return OperationResult{Success: true}
}

// DetachTarget 从会话中移除指定页面目标。
func (a *App) DetachTarget(sessionID, targetID string) OperationResult {
	err := a.service.DetachTarget(model.SessionID(sessionID), model.TargetID(targetID))
	if err != nil {
		a.log.Error("移除目标失败", "sessionID", sessionID, "targetID", targetID, "error", err)
		return OperationResult{Success: false, Error: err.Error()}
	}
	a.log.Debug("已移除目标", "targetID", targetID)
	return OperationResult{Success: true}
}

// EnableInterception 启用指定会话的网络拦截功能。
func (a *App) EnableInterception(sessionID string) OperationResult {
	err := a.service.EnableInterception(model.SessionID(sessionID))
	if err != nil {
		a.log.Error("启用拦截失败", "sessionID", sessionID, "error", err)
		return OperationResult{Success: false, Error: err.Error()}
	}
	a.log.Info("已启用拦截", "sessionID", sessionID)
	return OperationResult{Success: true}
}

// DisableInterception 停用指定会话的网络拦截功能。
func (a *App) DisableInterception(sessionID string) OperationResult {
	err := a.service.DisableInterception(model.SessionID(sessionID))
	if err != nil {
		a.log.Error("停用拦截失败", "sessionID", sessionID, "error", err)
		return OperationResult{Success: false, Error: err.Error()}
	}
	a.log.Info("已停用拦截", "sessionID", sessionID)
	return OperationResult{Success: true}
}

// LoadRules 从 JSON 字符串加载规则配置到指定会话。
func (a *App) LoadRules(sessionID string, rulesJSON string) OperationResult {
	var cfg rulespec.Config
	if err := json.Unmarshal([]byte(rulesJSON), &cfg); err != nil {
		a.log.Error("JSON 解析失败", "error", err)
		return OperationResult{Success: false, Error: "JSON 解析失败: " + err.Error()}
	}

	err := a.service.LoadRules(model.SessionID(sessionID), &cfg)
	if err != nil {
		a.log.Error("加载规则失败", "sessionID", sessionID, "error", err)
		return OperationResult{Success: false, Error: err.Error()}
	}

	a.log.Info("规则加载成功", "sessionID", sessionID, "ruleCount", len(cfg.Rules))
	return OperationResult{Success: true}
}

// StatsResult 表示规则统计结果。
type StatsResult struct {
	Stats   model.EngineStats `json:"stats"`
	Success bool              `json:"success"`
	Error   string            `json:"error,omitempty"`
}

// GetRuleStats 获取指定会话的规则命中统计信息。
func (a *App) GetRuleStats(sessionID string) StatsResult {
	stats, err := a.service.GetRuleStats(model.SessionID(sessionID))
	if err != nil {
		a.log.Error("获取规则统计失败", "sessionID", sessionID, "error", err)
		return StatsResult{Success: false, Error: err.Error()}
	}
	return StatsResult{Stats: stats, Success: true}
}

// subscribeEvents 订阅拦截事件并通过 Wails 事件系统推送到前端。
func (a *App) subscribeEvents(sessionID model.SessionID) {
	ch, err := a.service.SubscribeEvents(sessionID)
	if err != nil {
		a.log.Error("订阅事件失败", "sessionID", sessionID, "error", err)
		return
	}

	a.log.Debug("开始订阅事件", "sessionID", sessionID)
	for evt := range ch {
		// 通过 Wails 事件系统推送到前端
		runtime.EventsEmit(a.ctx, "intercept-event", evt)
		// 异步写入数据库
		if a.eventRepo != nil {
			a.eventRepo.Record(evt)
		}
	}
	a.log.Debug("事件订阅已结束", "sessionID", sessionID)
}

// LaunchBrowserResult 表示启动浏览器的结果。
type LaunchBrowserResult struct {
	DevToolsURL string `json:"devToolsUrl"`
	Success     bool   `json:"success"`
	Error       string `json:"error,omitempty"`
}

// LaunchBrowser 启动新的浏览器实例，如果已有浏览器运行则先关闭。
func (a *App) LaunchBrowser(headless bool) LaunchBrowserResult {
	a.log.Info("启动浏览器", "headless", headless)

	// 如果已有浏览器运行，先关闭
	if a.browser != nil {
		if err := a.browser.Stop(2 * time.Second); err != nil {
			a.log.Warn("关闭旧浏览器实例失败", "error", err)
		}
		a.browser = nil
	}

	opts := browser.Options{
		Headless: headless,
	}

	b, err := browser.Start(opts)
	if err != nil {
		a.log.Error("启动浏览器失败", "error", err)
		return LaunchBrowserResult{Success: false, Error: err.Error()}
	}

	a.browser = b
	a.log.Info("浏览器启动成功", "devToolsURL", b.DevToolsURL)
	return LaunchBrowserResult{DevToolsURL: b.DevToolsURL, Success: true}
}

// CloseBrowser 关闭已启动的浏览器实例。
func (a *App) CloseBrowser() OperationResult {
	if a.browser == nil {
		return OperationResult{Success: false, Error: "没有正在运行的浏览器"}
	}

	err := a.browser.Stop(2 * time.Second)
	a.browser = nil
	if err != nil {
		a.log.Error("关闭浏览器失败", "error", err)
		return OperationResult{Success: false, Error: err.Error()}
	}

	a.log.Info("浏览器已关闭")
	return OperationResult{Success: true}
}

// GetBrowserStatus 获取当前浏览器的运行状态。
func (a *App) GetBrowserStatus() LaunchBrowserResult {
	if a.browser == nil {
		return LaunchBrowserResult{Success: false}
	}
	return LaunchBrowserResult{DevToolsURL: a.browser.DevToolsURL, Success: true}
}

// SettingsResult 表示设置操作的结果。
type SettingsResult struct {
	Settings map[string]string `json:"settings"`
	Success  bool              `json:"success"`
	Error    string            `json:"error,omitempty"`
}

// GetAllSettings 获取所有应用设置。
func (a *App) GetAllSettings() SettingsResult {
	settings, err := a.settingsRepo.GetAll()
	if err != nil {
		a.log.Error("获取所有设置失败", "error", err)
		return SettingsResult{Success: false, Error: err.Error()}
	}
	return SettingsResult{Settings: settings, Success: true}
}

// GetSetting 获取单个设置项的值。
func (a *App) GetSetting(key string) string {
	return a.settingsRepo.GetWithDefault(key, "")
}

// SetSetting 设置单个配置项的值。
func (a *App) SetSetting(key, value string) OperationResult {
	if err := a.settingsRepo.Set(key, value); err != nil {
		a.log.Error("设置配置项失败", "key", key, "error", err)
		return OperationResult{Success: false, Error: err.Error()}
	}
	return OperationResult{Success: true}
}

// SetMultipleSettings 批量设置多个配置项。
func (a *App) SetMultipleSettings(settingsJSON string) OperationResult {
	var settings map[string]string
	if err := json.Unmarshal([]byte(settingsJSON), &settings); err != nil {
		a.log.Error("批量设置 JSON 解析失败", "error", err)
		return OperationResult{Success: false, Error: "JSON 解析失败: " + err.Error()}
	}

	if err := a.settingsRepo.SetMultiple(settings); err != nil {
		a.log.Error("批量设置失败", "error", err)
		return OperationResult{Success: false, Error: err.Error()}
	}
	return OperationResult{Success: true}
}

// RuleSetListResult 表示规则集列表结果。
type RuleSetListResult struct {
	RuleSets []storage.RuleSetRecord `json:"ruleSets"`
	Success  bool                    `json:"success"`
	Error    string                  `json:"error,omitempty"`
}

// RuleSetResult 表示单个规则集操作结果。
type RuleSetResult struct {
	RuleSet *storage.RuleSetRecord `json:"ruleSet"`
	Success bool                   `json:"success"`
	Error   string                 `json:"error,omitempty"`
}

// ListRuleSets 列出所有已保存的规则集。
func (a *App) ListRuleSets() RuleSetListResult {
	ruleSets, err := a.ruleSetRepo.List()
	if err != nil {
		a.log.Error("列出规则集失败", "error", err)
		return RuleSetListResult{Success: false, Error: err.Error()}
	}
	return RuleSetListResult{RuleSets: ruleSets, Success: true}
}

// GetRuleSet 根据 ID 获取指定规则集。
func (a *App) GetRuleSet(id uint) RuleSetResult {
	ruleSet, err := a.ruleSetRepo.GetByID(id)
	if err != nil {
		a.log.Error("获取规则集失败", "id", id, "error", err)
		return RuleSetResult{Success: false, Error: err.Error()}
	}
	return RuleSetResult{RuleSet: ruleSet, Success: true}
}

// SaveRuleSet 保存规则集（创建或更新），id 为 0 时创建新规则集。
func (a *App) SaveRuleSet(id uint, name string, rulesJSON string) RuleSetResult {
	var cfg rulespec.Config
	if err := json.Unmarshal([]byte(rulesJSON), &cfg); err != nil {
		a.log.Error("保存规则集 JSON 解析失败", "error", err)
		return RuleSetResult{Success: false, Error: "JSON 解析失败: " + err.Error()}
	}

	ruleSet, err := a.ruleSetRepo.SaveFromConfig(id, name, &cfg)
	if err != nil {
		a.log.Error("保存规则集失败", "id", id, "name", name, "error", err)
		return RuleSetResult{Success: false, Error: err.Error()}
	}

	a.log.Info("规则集已保存", "id", ruleSet.ID, "name", name)
	return RuleSetResult{RuleSet: ruleSet, Success: true}
}

// DeleteRuleSet 删除指定 ID 的规则集。
func (a *App) DeleteRuleSet(id uint) OperationResult {
	if err := a.ruleSetRepo.Delete(id); err != nil {
		a.log.Error("删除规则集失败", "id", id, "error", err)
		return OperationResult{Success: false, Error: err.Error()}
	}
	a.log.Info("规则集已删除", "id", id)
	return OperationResult{Success: true}
}

// SetActiveRuleSet 设置指定规则集为当前激活状态。
func (a *App) SetActiveRuleSet(id uint) OperationResult {
	if err := a.ruleSetRepo.SetActive(id); err != nil {
		a.log.Error("设置激活规则集失败", "id", id, "error", err)
		return OperationResult{Success: false, Error: err.Error()}
	}

	// 记住上次使用的规则集
	if err := a.settingsRepo.SetLastRuleSetID(fmt.Sprintf("%d", id)); err != nil {
		a.log.Warn("保存上次规则集 ID 失败", "id", id, "error", err)
	}

	a.log.Debug("已设置激活规则集", "id", id)
	return OperationResult{Success: true}
}

// GetActiveRuleSet 获取当前激活的规则集。
func (a *App) GetActiveRuleSet() RuleSetResult {
	ruleSet, err := a.ruleSetRepo.GetActive()
	if err != nil {
		a.log.Error("获取激活规则集失败", "error", err)
		return RuleSetResult{Success: false, Error: err.Error()}
	}
	return RuleSetResult{RuleSet: ruleSet, Success: true}
}

// RenameRuleSet 重命名指定的规则集。
func (a *App) RenameRuleSet(id uint, newName string) OperationResult {
	if err := a.ruleSetRepo.Rename(id, newName); err != nil {
		a.log.Error("重命名规则集失败", "id", id, "newName", newName, "error", err)
		return OperationResult{Success: false, Error: err.Error()}
	}
	a.log.Debug("规则集已重命名", "id", id, "newName", newName)
	return OperationResult{Success: true}
}

// DuplicateRuleSet 复制指定规则集并以新名称保存。
func (a *App) DuplicateRuleSet(id uint, newName string) RuleSetResult {
	ruleSet, err := a.ruleSetRepo.Duplicate(id, newName)
	if err != nil {
		a.log.Error("复制规则集失败", "id", id, "newName", newName, "error", err)
		return RuleSetResult{Success: false, Error: err.Error()}
	}
	a.log.Info("规则集已复制", "sourceID", id, "newID", ruleSet.ID, "newName", newName)
	return RuleSetResult{RuleSet: ruleSet, Success: true}
}

// LoadActiveRuleSetToSession 加载当前激活的规则集到活跃会话。
func (a *App) LoadActiveRuleSetToSession() OperationResult {
	if a.currentSession == "" {
		return OperationResult{Success: false, Error: "没有活跃会话"}
	}

	ruleSet, err := a.ruleSetRepo.GetActive()
	if err != nil {
		a.log.Error("获取激活规则集失败", "error", err)
		return OperationResult{Success: false, Error: err.Error()}
	}
	if ruleSet == nil {
		return OperationResult{Success: false, Error: "没有激活的规则集"}
	}

	cfg, err := a.ruleSetRepo.ToConfig(ruleSet)
	if err != nil {
		a.log.Error("转换规则集失败", "id", ruleSet.ID, "error", err)
		return OperationResult{Success: false, Error: err.Error()}
	}

	if err := a.service.LoadRules(a.currentSession, cfg); err != nil {
		a.log.Error("加载规则到会话失败", "sessionID", a.currentSession, "error", err)
		return OperationResult{Success: false, Error: err.Error()}
	}

	a.log.Info("已加载激活规则集到会话", "sessionID", a.currentSession, "ruleSetID", ruleSet.ID)
	return OperationResult{Success: true}
}

// EventHistoryResult 表示事件历史查询结果。
type EventHistoryResult struct {
	Events  []storage.InterceptEventRecord `json:"events"`
	Total   int64                          `json:"total"`
	Success bool                           `json:"success"`
	Error   string                         `json:"error,omitempty"`
}

// QueryEventHistory 根据条件查询事件历史记录。
func (a *App) QueryEventHistory(sessionID, eventType, url, method string, startTime, endTime int64, offset, limit int) EventHistoryResult {
	if a.eventRepo == nil {
		a.log.Error("查询事件历史失败: 事件仓库未初始化")
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
		a.log.Error("查询事件历史失败", "error", err)
		return EventHistoryResult{Success: false, Error: err.Error()}
	}
	return EventHistoryResult{Events: events, Total: total, Success: true}
}

// EventStatsResult 表示事件统计结果。
type EventStatsResult struct {
	Stats   *storage.EventStats `json:"stats"`
	Success bool                `json:"success"`
	Error   string              `json:"error,omitempty"`
}

// GetEventStats 获取事件统计信息。
func (a *App) GetEventStats() EventStatsResult {
	if a.eventRepo == nil {
		a.log.Error("获取事件统计失败: 事件仓库未初始化")
		return EventStatsResult{Success: false, Error: "事件仓库未初始化"}
	}

	stats, err := a.eventRepo.GetStats()
	if err != nil {
		a.log.Error("获取事件统计失败", "error", err)
		return EventStatsResult{Success: false, Error: err.Error()}
	}
	return EventStatsResult{Stats: stats, Success: true}
}

// CleanupEventHistory 清理指定天数之前的旧事件记录。
func (a *App) CleanupEventHistory(retentionDays int) OperationResult {
	if a.eventRepo == nil {
		a.log.Error("清理事件失败: 事件仓库未初始化")
		return OperationResult{Success: false, Error: "事件仓库未初始化"}
	}

	deleted, err := a.eventRepo.CleanupOldEvents(retentionDays)
	if err != nil {
		a.log.Error("清理旧事件失败", "retentionDays", retentionDays, "error", err)
		return OperationResult{Success: false, Error: err.Error()}
	}

	a.log.Info("已清理旧事件", "retentionDays", retentionDays, "deletedCount", deleted)
	return OperationResult{Success: true}
}
