package gui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"cdpnetool/internal/browser"
	"cdpnetool/internal/config"
	"cdpnetool/internal/logger"
	"cdpnetool/internal/storage"
	"cdpnetool/pkg/api"
	"cdpnetool/pkg/model"
	"cdpnetool/pkg/rulespec"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App 是暴露给前端的 Wails 方法集合，负责管理会话、浏览器、配置和事件。
type App struct {
	ctx            context.Context
	cfg            *config.Config
	log            logger.Logger
	service        api.Service
	currentSession model.SessionID
	browser        *browser.Browser
	db             *storage.DB
	settingsRepo   *storage.SettingsRepo
	configRepo     *storage.ConfigRepo
	eventRepo      *storage.EventRepo
	isDirty        bool
}

// NewApp 创建并返回一个新的 App 实例。
func NewApp() *App {
	cfg := config.NewConfig()
	log := logger.New(logger.Options{
		Level:   cfg.Log.Level,
		Writers: cfg.Log.Writer,
	})
	return &App{
		cfg:     cfg,
		log:     log,
		service: api.NewService(log),
	}
}

// Startup 在应用启动时由 Wails 框架调用，完成数据库和事件仓库的初始化。
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
	a.log.Info("应用启动")

	l := storage.NewGormLogger(a.log)
	db, err := storage.NewDB(a.cfg, l)
	if err != nil {
		a.log.Err(err, "数据库初始化失败")
		return
	}

	a.db = db
	a.settingsRepo = storage.NewSettingsRepo(db)
	a.configRepo = storage.NewConfigRepo(db)
	a.eventRepo = storage.NewEventRepo(db)
	a.log.Debug("事件仓库初始化完成")
}

// Shutdown 在应用关闭时由 Wails 框架调用，负责清理会话、浏览器和数据库资源。
func (a *App) Shutdown(ctx context.Context) {
	a.log.Info("应用关闭中...")

	if a.currentSession != "" {
		if err := a.service.StopSession(a.currentSession); err != nil {
			a.log.Err(err, "停止会话失败", "sessionID", a.currentSession)
		}
	}

	if a.browser != nil {
		if err := a.browser.Stop(2 * time.Second); err != nil {
			a.log.Err(err, "关闭浏览器失败")
		}
	}

	if a.eventRepo != nil {
		a.eventRepo.Stop()
	}

	if a.db != nil {
		if err := a.db.Close(); err != nil {
			a.log.Err(err, "关闭数据库失败")
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

	cfg := model.SessionConfig{DevToolsURL: devToolsURL}
	sid, err := a.service.StartSession(cfg)
	if err != nil {
		a.log.Err(err, "启动会话失败")
		return SessionResult{Success: false, Error: fmt.Sprintf("启动会话失败: %v", err)}
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
		a.log.Err(err, "停止会话失败", "sessionID", sessionID)
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
		a.log.Err(err, "列出目标失败", "sessionID", sessionID)
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
		a.log.Err(err, "附加目标失败", "sessionID", sessionID, "targetID", targetID)
		return OperationResult{Success: false, Error: err.Error()}
	}

	a.log.Debug("已附加目标", "targetID", targetID)
	return OperationResult{Success: true}
}

// DetachTarget 从会话中移除指定页面目标。
func (a *App) DetachTarget(sessionID, targetID string) OperationResult {
	err := a.service.DetachTarget(model.SessionID(sessionID), model.TargetID(targetID))
	if err != nil {
		a.log.Err(err, "移除目标失败", "sessionID", sessionID, "targetID", targetID)
		return OperationResult{Success: false, Error: err.Error()}
	}

	a.log.Debug("已移除目标", "targetID", targetID)
	return OperationResult{Success: true}
}

// SetDirty 供前端更新未保存状态
func (a *App) SetDirty(dirty bool) {
	a.isDirty = dirty
}

// BeforeClose 在窗口关闭前调用，如果有未保存更改则弹出确认框
func (a *App) BeforeClose(ctx context.Context) bool {
	if !a.isDirty {
		return false
	}

	result, err := runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
		Type:          runtime.QuestionDialog,
		Title:         "提醒",
		Message:       "当前有未保存的规则更改，确定要退出吗？",
		DefaultButton: "否",
		Buttons:       []string{"是", "否"},
	})

	if err != nil {
		a.log.Warn("关闭确认对话框出错", "error", err)
		return true
	}

	a.log.Debug("用户选择", "result", result)
	return result == "否"
}

// ExportConfig 弹出原生保存对话框导出配置
func (a *App) ExportConfig(name, rulesJSON string) OperationResult {
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: name + ".json",
		Title:           "导出配置",
		Filters: []runtime.FileFilter{
			{DisplayName: "JSON Files (*.json)", Pattern: "*.json"},
		},
	})

	if err != nil {
		return OperationResult{Success: false, Error: err.Error()}
	}

	if path == "" {
		return OperationResult{Success: true} // 用户取消
	}

	err = os.WriteFile(path, []byte(rulesJSON), 0644)
	if err != nil {
		return OperationResult{Success: false, Error: "文件写入失败: " + err.Error()}
	}

	return OperationResult{Success: true}
}

// EnableInterception 启用指定会话的网络拦截功能。
func (a *App) EnableInterception(sessionID string) OperationResult {
	targets, err := a.service.ListTargets(model.SessionID(sessionID))
	hasAttached := false
	if err == nil {
		for _, t := range targets {
			if t.IsCurrent {
				hasAttached = true
				break
			}
		}
	}

	if !hasAttached {
		return OperationResult{Success: false, Error: "请先在 Targets 标签页附加至少一个目标"}
	}

	err = a.service.EnableInterception(model.SessionID(sessionID))
	if err != nil {
		a.log.Err(err, "启用拦截失败", "sessionID", sessionID)
		return OperationResult{Success: false, Error: err.Error()}
	}

	a.log.Info("已启用拦截", "sessionID", sessionID)
	return OperationResult{Success: true}
}

// DisableInterception 停用指定会话的网络拦截功能。
func (a *App) DisableInterception(sessionID string) OperationResult {
	err := a.service.DisableInterception(model.SessionID(sessionID))
	if err != nil {
		a.log.Err(err, "停用拦截失败", "sessionID", sessionID)
		return OperationResult{Success: false, Error: err.Error()}
	}

	a.log.Info("已停用拦截", "sessionID", sessionID)
	return OperationResult{Success: true}
}

// LoadRules 从 JSON 字符串加载规则配置到指定会话。
func (a *App) LoadRules(sessionID string, rulesJSON string) OperationResult {
	var cfg rulespec.Config
	if err := json.Unmarshal([]byte(rulesJSON), &cfg); err != nil {
		a.log.Err(err, "JSON 解析失败")
		return OperationResult{Success: false, Error: "JSON 解析失败: " + err.Error()}
	}

	err := a.service.LoadRules(model.SessionID(sessionID), &cfg)
	if err != nil {
		a.log.Err(err, "加载规则失败", "sessionID", sessionID)
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
		a.log.Err(err, "获取规则统计失败", "sessionID", sessionID)
		return StatsResult{Success: false, Error: err.Error()}
	}

	return StatsResult{Stats: stats, Success: true}
}

// subscribeEvents 订阅拦截事件并通过 Wails 事件系统推送到前端。
func (a *App) subscribeEvents(sessionID model.SessionID) {
	ch, err := a.service.SubscribeEvents(sessionID)
	if err != nil {
		a.log.Err(err, "订阅事件失败", "sessionID", sessionID)
		return
	}

	a.log.Debug("开始订阅事件", "sessionID", sessionID)
	for evt := range ch {
		// 通过 Wails 事件系统推送到前端
		runtime.EventsEmit(a.ctx, "intercept-event", evt)

		if evt.IsMatched && evt.Matched != nil && a.eventRepo != nil {
			evt.Matched.Session = sessionID
			a.eventRepo.RecordMatched(evt.Matched)
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
		a.log.Err(err, "启动浏览器失败")
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
		a.log.Err(err, "关闭浏览器失败")
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
		a.log.Err(err, "获取所有设置失败")
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
		a.log.Err(err, "设置配置项失败", "key", key)
		return OperationResult{Success: false, Error: err.Error()}
	}

	return OperationResult{Success: true}
}

// SetMultipleSettings 批量设置多个配置项。
func (a *App) SetMultipleSettings(settingsJSON string) OperationResult {
	var settings map[string]string
	if err := json.Unmarshal([]byte(settingsJSON), &settings); err != nil {
		a.log.Err(err, "批量设置 JSON 解析失败")
		return OperationResult{Success: false, Error: "JSON 解析失败: " + err.Error()}
	}

	if err := a.settingsRepo.SetMultiple(settings); err != nil {
		a.log.Err(err, "批量设置失败")
		return OperationResult{Success: false, Error: err.Error()}
	}

	return OperationResult{Success: true}
}

// ConfigListResult 表示配置列表结果。
type ConfigListResult struct {
	Configs []storage.ConfigRecord `json:"configs"`
	Success bool                   `json:"success"`
	Error   string                 `json:"error,omitempty"`
}

// ConfigResult 表示单个配置操作结果。
type ConfigResult struct {
	Config  *storage.ConfigRecord `json:"config"`
	Success bool                  `json:"success"`
	Error   string                `json:"error,omitempty"`
}

// ListConfigs 列出所有已保存的配置。
func (a *App) ListConfigs() ConfigListResult {
	configs, err := a.configRepo.List()
	if err != nil {
		a.log.Err(err, "列出配置失败")
		return ConfigListResult{Success: false, Error: err.Error()}
	}

	return ConfigListResult{Configs: configs, Success: true}
}

// GetConfig 根据 ID 获取指定配置。
func (a *App) GetConfig(id uint) ConfigResult {
	config, err := a.configRepo.GetByID(id)
	if err != nil {
		a.log.Err(err, "获取配置失败", "id", id)
		return ConfigResult{Success: false, Error: err.Error()}
	}

	return ConfigResult{Config: config, Success: true}
}

// NewConfigResult 表示创建新配置的结果（含完整 JSON）。
type NewConfigResult struct {
	Config     *storage.ConfigRecord `json:"config"`
	ConfigJSON string                `json:"configJson"` // 完整的 rulespec.Config JSON
	Success    bool                  `json:"success"`
	Error      string                `json:"error,omitempty"`
}

// CreateNewConfig 创建一个新的空配置并保存到数据库。
func (a *App) CreateNewConfig(name string) NewConfigResult {
	cfg := rulespec.NewConfig(name)

	config, err := a.configRepo.Create(cfg)
	if err != nil {
		a.log.Err(err, "创建配置失败", "name", name)
		return NewConfigResult{Success: false, Error: err.Error()}
	}

	configJSON, err := json.Marshal(cfg)
	if err != nil {
		a.log.Err(err, "序列化配置失败")
		return NewConfigResult{Success: false, Error: err.Error()}
	}

	a.log.Info("新配置已创建", "id", config.ID, "name", name, "configId", cfg.ID)
	return NewConfigResult{Config: config, ConfigJSON: string(configJSON), Success: true}
}

// NewRuleResult 表示创建新规则的结果。
type NewRuleResult struct {
	RuleJSON string `json:"ruleJson"`
	Success  bool   `json:"success"`
	Error    string `json:"error,omitempty"`
}

// GenerateNewRule 生成一个新的空规则
func (a *App) GenerateNewRule(name string, existingCount int) NewRuleResult {
	rule := rulespec.NewRule(name, existingCount)
	ruleJSON, err := json.Marshal(rule)
	if err != nil {
		a.log.Err(err, "序列化规则失败")
		return NewRuleResult{Success: false, Error: err.Error()}
	}

	return NewRuleResult{RuleJSON: string(ruleJSON), Success: true}
}

// SaveConfig 保存配置（创建或更新），dbID 为 0 时创建新配置。
func (a *App) SaveConfig(dbID uint, configJSON string) ConfigResult {
	var cfg rulespec.Config
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		a.log.Err(err, "保存配置 JSON 解析失败")
		return ConfigResult{Success: false, Error: "JSON 解析失败: " + err.Error()}
	}

	config, err := a.configRepo.Save(dbID, &cfg)
	if err != nil {
		a.log.Err(err, "保存配置失败", "dbID", dbID, "configID", cfg.ID)
		return ConfigResult{Success: false, Error: err.Error()}
	}

	a.log.Info("配置已保存", "dbID", config.ID, "configID", cfg.ID, "name", cfg.Name)
	return ConfigResult{Config: config, Success: true}
}

// DeleteConfig 删除指定 ID 的配置。
func (a *App) DeleteConfig(id uint) OperationResult {
	if err := a.configRepo.Delete(id); err != nil {
		a.log.Err(err, "删除配置失败", "id", id)
		return OperationResult{Success: false, Error: err.Error()}
	}

	a.log.Info("配置已删除", "id", id)
	return OperationResult{Success: true}
}

// SetActiveConfig 设置指定配置为当前激活状态。
func (a *App) SetActiveConfig(id uint) OperationResult {
	if err := a.configRepo.SetActive(id); err != nil {
		a.log.Err(err, "设置激活配置失败", "id", id)
		return OperationResult{Success: false, Error: err.Error()}
	}

	if err := a.settingsRepo.SetLastConfigID(fmt.Sprintf("%d", id)); err != nil {
		a.log.Warn("保存上次配置 ID 失败", "id", id, "error", err)
	}

	a.log.Debug("已设置激活配置", "id", id)
	return OperationResult{Success: true}
}

// GetActiveConfig 获取当前激活的配置。
func (a *App) GetActiveConfig() ConfigResult {
	config, err := a.configRepo.GetActive()
	if err != nil {
		a.log.Err(err, "获取激活配置失败")
		return ConfigResult{Success: false, Error: err.Error()}
	}

	return ConfigResult{Config: config, Success: true}
}

// RenameConfig 重命名指定的配置。
func (a *App) RenameConfig(id uint, newName string) OperationResult {
	if err := a.configRepo.Rename(id, newName); err != nil {
		a.log.Err(err, "重命名配置失败", "id", id, "newName", newName)
		return OperationResult{Success: false, Error: err.Error()}
	}

	a.log.Debug("配置已重命名", "id", id, "newName", newName)
	return OperationResult{Success: true}
}

// ImportConfig 导入配置（根据配置 ID 判断覆盖或新增）。
func (a *App) ImportConfig(configJSON string) ConfigResult {
	var cfg rulespec.Config
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		a.log.Err(err, "导入配置 JSON 解析失败")
		return ConfigResult{Success: false, Error: "JSON 解析失败: " + err.Error()}
	}

	config, err := a.configRepo.Upsert(&cfg)
	if err != nil {
		a.log.Err(err, "导入配置失败", "configID", cfg.ID)
		return ConfigResult{Success: false, Error: err.Error()}
	}

	a.log.Info("配置已导入", "dbID", config.ID, "configID", cfg.ID, "name", cfg.Name)
	return ConfigResult{Config: config, Success: true}
}

// LoadActiveConfigToSession 加载当前激活的配置到活跃会话。
func (a *App) LoadActiveConfigToSession() OperationResult {
	if a.currentSession == "" {
		return OperationResult{Success: false, Error: "没有活跃会话"}
	}

	config, err := a.configRepo.GetActive()
	if err != nil {
		a.log.Err(err, "获取激活配置失败")
		return OperationResult{Success: false, Error: err.Error()}
	}
	if config == nil {
		return OperationResult{Success: false, Error: "没有激活的配置"}
	}

	cfg, err := a.configRepo.ToRulespecConfig(config)
	if err != nil {
		a.log.Err(err, "转换配置失败", "id", config.ID)
		return OperationResult{Success: false, Error: err.Error()}
	}

	if err := a.service.LoadRules(a.currentSession, cfg); err != nil {
		a.log.Err(err, "加载规则到会话失败", "sessionID", a.currentSession)
		return OperationResult{Success: false, Error: err.Error()}
	}

	a.log.Info("已加载激活配置到会话", "sessionID", a.currentSession, "configID", config.ID)
	return OperationResult{Success: true}
}

// MatchedEventHistoryResult 表示匹配事件历史查询结果。
type MatchedEventHistoryResult struct {
	Events  []storage.MatchedEventRecord `json:"events"`
	Total   int64                        `json:"total"`
	Success bool                         `json:"success"`
	Error   string                       `json:"error,omitempty"`
}

// QueryMatchedEventHistory 根据条件查询匹配事件历史记录。
func (a *App) QueryMatchedEventHistory(sessionID, finalResult, url, method string, startTime, endTime int64, offset, limit int) MatchedEventHistoryResult {
	if a.eventRepo == nil {
		a.log.Error("查询事件历史失败: 事件仓库未初始化")
		return MatchedEventHistoryResult{Success: false, Error: "事件仓库未初始化"}
	}

	events, total, err := a.eventRepo.Query(storage.QueryOptions{
		SessionID:   sessionID,
		FinalResult: finalResult,
		URL:         url,
		Method:      method,
		StartTime:   startTime,
		EndTime:     endTime,
		Offset:      offset,
		Limit:       limit,
	})
	if err != nil {
		a.log.Err(err, "查询事件历史失败")
		return MatchedEventHistoryResult{Success: false, Error: err.Error()}
	}

	return MatchedEventHistoryResult{Events: events, Total: total, Success: true}
}

// CleanupEventHistory 清理指定天数之前的旧事件记录。
func (a *App) CleanupEventHistory(retentionDays int) OperationResult {
	if a.eventRepo == nil {
		a.log.Error("清理事件失败: 事件仓库未初始化")
		return OperationResult{Success: false, Error: "事件仓库未初始化"}
	}

	deleted, err := a.eventRepo.CleanupOldEvents(retentionDays)
	if err != nil {
		a.log.Err(err, "清理旧事件失败", "retentionDays", retentionDays)
		return OperationResult{Success: false, Error: err.Error()}
	}

	a.log.Info("已清理旧事件", "retentionDays", retentionDays, "deletedCount", deleted)
	return OperationResult{Success: true}
}
