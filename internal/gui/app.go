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
	"cdpnetool/internal/storage/db"
	"cdpnetool/internal/storage/model"
	"cdpnetool/internal/storage/repo"
	"cdpnetool/pkg/api"
	"cdpnetool/pkg/domain"
	"cdpnetool/pkg/rulespec"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"gorm.io/gorm"
	gl "gorm.io/gorm/logger"
)

// App 负责管理会话、浏览器、配置和事件，供前端调用。
type App struct {
	ctx             context.Context
	cfg             *config.Config
	log             logger.Logger
	service         api.Service
	currentSession  domain.SessionID
	browser         *browser.Browser
	gdb             *gorm.DB
	settingsRepo    *repo.SettingsRepo
	configRepo      *repo.ConfigRepo
	eventRepo       *repo.EventRepo
	isDirty         bool
	cancelSubscribe context.CancelFunc
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

// Startup 初始化数据库和仓库。
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
	a.log.Info("应用启动")

	gormLogger := db.NewLogger(a.log).LogMode(gl.Info)
	gdb, err := db.New(db.Options{
		Name:   a.cfg.Sqlite.Db,
		Prefix: a.cfg.Sqlite.Prefix,
		Logger: gormLogger,
	})
	if err != nil {
		a.log.Err(err, "数据库初始化失败")
		return
	}

	err = db.Migrate(gdb,
		&model.Setting{},
		&model.ConfigRecord{},
		&model.NetworkEventRecord{},
	)
	if err != nil {
		a.log.Err(err, "数据库迁移失败")
		return
	}

	a.gdb = gdb
	a.settingsRepo = repo.NewSettingsRepo(gdb)
	a.configRepo = repo.NewConfigRepo(gdb)
	a.eventRepo = repo.NewEventRepo(gdb, a.log)
	a.log.Debug("数据持久化层初始化完成")
}

// Shutdown 负责清理资源。
func (a *App) Shutdown(ctx context.Context) {
	a.log.Info("应用关闭中...")

	if a.cancelSubscribe != nil {
		a.cancelSubscribe()
	}

	if a.currentSession != "" {
		_ = a.service.StopSession(ctx, a.currentSession)
	}

	if a.browser != nil {
		_ = a.browser.Stop(2 * time.Second)
	}

	if a.eventRepo != nil {
		a.eventRepo.Stop()
	}

	if a.gdb != nil {
		if sqlDB, err := a.gdb.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}

	a.log.Info("应用已关闭")
}

// StartSession 创建新的拦截会话，并启动事件订阅。
func (a *App) StartSession(devToolsURL string) api.Response[SessionData] {
	a.log.Info("启动会话", "devToolsURL", devToolsURL)

	// 停止旧的订阅
	if a.cancelSubscribe != nil {
		a.cancelSubscribe()
		a.cancelSubscribe = nil
	}

	cfg := domain.SessionConfig{DevToolsURL: devToolsURL}
	sid, err := a.service.StartSession(a.ctx, cfg)
	if err != nil {
		code, msg := a.translateError(err)
		return api.Fail[SessionData](code, msg)
	}

	a.currentSession = sid

	// 启动事件订阅
	ctx, cancel := context.WithCancel(a.ctx)
	a.cancelSubscribe = cancel
	go a.subscribeEvents(ctx, sid)

	a.log.Info("会话启动成功", "sessionID", sid)
	return api.OK(SessionData{SessionID: string(sid)})
}

// StopSession 停止指定的会话。
func (a *App) StopSession(sessionID string) api.Response[api.EmptyData] {
	a.log.Info("停止会话", "sessionID", sessionID)

	// 取消事件订阅
	if a.cancelSubscribe != nil {
		a.cancelSubscribe()
		a.cancelSubscribe = nil
	}

	err := a.service.StopSession(a.ctx, domain.SessionID(sessionID))
	if err != nil {
		code, msg := a.translateError(err)
		return api.Fail[api.EmptyData](code, msg)
	}

	if a.currentSession == domain.SessionID(sessionID) {
		a.currentSession = ""
	}

	return api.OK(api.EmptyData{})
}

// GetCurrentSession 返回当前活跃会话的 ID。
func (a *App) GetCurrentSession() api.Response[SessionData] {
	return api.OK(SessionData{SessionID: string(a.currentSession)})
}

// ListTargets 列出指定会话中的浏览器页面目标。
func (a *App) ListTargets(sessionID string) api.Response[TargetListData] {
	targets, err := a.service.ListTargets(a.ctx, domain.SessionID(sessionID))
	if err != nil {
		code, msg := a.translateError(err)
		return api.Fail[TargetListData](code, msg)
	}

	return api.OK(TargetListData{Targets: targets})
}

// AttachTarget 附加指定页面目标到会话进行拦截。
func (a *App) AttachTarget(sessionID, targetID string) api.Response[api.EmptyData] {
	err := a.service.AttachTarget(a.ctx, domain.SessionID(sessionID), domain.TargetID(targetID))
	if err != nil {
		code, msg := a.translateError(err)
		return api.Fail[api.EmptyData](code, msg)
	}

	a.log.Debug("已附加目标", "targetID", targetID)
	return api.OK(api.EmptyData{})
}

// DetachTarget 从会话中移除指定页面目标。
func (a *App) DetachTarget(sessionID, targetID string) api.Response[api.EmptyData] {
	err := a.service.DetachTarget(a.ctx, domain.SessionID(sessionID), domain.TargetID(targetID))
	if err != nil {
		code, msg := a.translateError(err)
		return api.Fail[api.EmptyData](code, msg)
	}

	a.log.Debug("已移除目标", "targetID", targetID)
	return api.OK(api.EmptyData{})
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
func (a *App) ExportConfig(name, rulesJSON string) api.Response[api.EmptyData] {
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: name + ".json",
		Title:           "导出配置",
		Filters: []runtime.FileFilter{
			{DisplayName: "JSON Files (*.json)", Pattern: "*.json"},
		},
	})

	if err != nil {
		code, msg := a.translateError(err)
		return api.Fail[api.EmptyData](code, msg)
	}

	if path == "" {
		return api.OK(api.EmptyData{})
	}

	err = os.WriteFile(path, []byte(rulesJSON), 0644)
	if err != nil {
		code, msg := a.translateError(err)
		return api.Fail[api.EmptyData](code, msg)
	}

	return api.OK(api.EmptyData{})
}

// EnableInterception 启用指定会话的网络拦截功能。
func (a *App) EnableInterception(sessionID string) api.Response[api.EmptyData] {
	err := a.service.EnableInterception(a.ctx, domain.SessionID(sessionID))
	if err != nil {
		code, msg := a.translateError(err)
		return api.Fail[api.EmptyData](code, msg)
	}

	a.log.Info("已启用拦截", "sessionID", sessionID)
	return api.OK(api.EmptyData{})
}

// DisableInterception 停用指定会话的网络拦截功能。
func (a *App) DisableInterception(sessionID string) api.Response[api.EmptyData] {
	err := a.service.DisableInterception(a.ctx, domain.SessionID(sessionID))
	if err != nil {
		code, msg := a.translateError(err)
		return api.Fail[api.EmptyData](code, msg)
	}

	a.log.Info("已停用拦截", "sessionID", sessionID)
	return api.OK(api.EmptyData{})
}

// LoadRules 从 JSON 字符串加载规则配置到指定会话。
func (a *App) LoadRules(sessionID string, rulesJSON string) api.Response[api.EmptyData] {
	var cfg rulespec.Config
	if err := json.Unmarshal([]byte(rulesJSON), &cfg); err != nil {
		code, msg := a.translateError(err)
		return api.Fail[api.EmptyData](code, msg)
	}

	err := a.service.LoadRules(a.ctx, domain.SessionID(sessionID), &cfg)
	if err != nil {
		code, msg := a.translateError(err)
		return api.Fail[api.EmptyData](code, msg)
	}

	a.log.Info("规则加载成功", "sessionID", sessionID, "ruleCount", len(cfg.Rules))
	return api.OK(api.EmptyData{})
}

// GetRuleStats 获取指定会话的规则命中统计信息。
func (a *App) GetRuleStats(sessionID string) api.Response[StatsData] {
	stats, err := a.service.GetRuleStats(a.ctx, domain.SessionID(sessionID))
	if err != nil {
		code, msg := a.translateError(err)
		return api.Fail[StatsData](code, msg)
	}

	return api.OK(StatsData{Stats: stats})
}

// subscribeEvents 订阅拦截事件并通过 Wails 事件系统推送到前端。
func (a *App) subscribeEvents(ctx context.Context, sessionID domain.SessionID) {
	ch, err := a.service.SubscribeEvents(ctx, sessionID)
	if err != nil {
		a.log.Err(err, "订阅事件失败", "sessionID", sessionID)
		return
	}

	a.log.Debug("开始订阅事件", "sessionID", sessionID)
	for {
		select {
		case evt, ok := <-ch:
			if !ok {
				a.log.Debug("事件通道已关闭", "sessionID", sessionID)
				return
			}

			// 填充 sessionID
			evt.Session = sessionID

			// 通过 Wails 事件系统推送到前端
			runtime.EventsEmit(a.ctx, "intercept-event", evt)

			// 记录到数据库
			if a.eventRepo != nil {
				a.eventRepo.Record(&evt)
			}

		case <-ctx.Done():
			a.log.Debug("事件订阅被取消", "sessionID", sessionID)
			return
		}
	}
}

// LaunchBrowser 启动新的浏览器实例，如果已有浏览器运行则先关闭。
func (a *App) LaunchBrowser(headless bool) api.Response[BrowserData] {
	a.log.Info("启动浏览器", "headless", headless)

	if a.browser != nil {
		if err := a.browser.Stop(2 * time.Second); err != nil {
			a.log.Warn("关闭旧浏览器实例失败", "error", err)
		}
		a.browser = nil
	}

	opts := browser.Options{
		Logger:        a.log,
		Headless:      headless,
		ClearUserData: true,
	}

	b, err := browser.Start(a.ctx, opts)
	if err != nil {
		code, msg := a.translateError(err)
		return api.Fail[BrowserData](code, msg)
	}

	a.browser = b
	a.log.Info("浏览器启动成功", "devToolsURL", b.DevToolsURL)
	return api.OK(BrowserData{DevToolsURL: b.DevToolsURL})
}

// CloseBrowser 关闭已启动的浏览器实例。
func (a *App) CloseBrowser() api.Response[api.EmptyData] {
	if a.browser == nil {
		code, msg := a.translateError(domain.ErrBrowserNotRunning)
		return api.Fail[api.EmptyData](code, msg)
	}

	err := a.browser.Stop(2 * time.Second)
	a.browser = nil
	if err != nil {
		code, msg := a.translateError(err)
		return api.Fail[api.EmptyData](code, msg)
	}

	a.log.Info("浏览器已关闭")
	return api.OK(api.EmptyData{})
}

// GetBrowserStatus 获取当前浏览器的运行状态。
func (a *App) GetBrowserStatus() api.Response[BrowserData] {
	if a.browser == nil {
		return api.OK(BrowserData{})
	}

	return api.OK(BrowserData{DevToolsURL: a.browser.DevToolsURL})
}

// GetAllSettings 获取所有应用设置。
func (a *App) GetAllSettings() api.Response[SettingsData] {
	settings, err := a.settingsRepo.GetAll(a.ctx)
	if err != nil {
		code, msg := a.translateError(err)
		return api.Fail[SettingsData](code, msg)
	}

	return api.OK(SettingsData{Settings: settings})
}

// GetSetting 获取单个设置项的值。
func (a *App) GetSetting(key string) api.Response[SettingData] {
	value := a.settingsRepo.GetWithDefault(a.ctx, key, "")
	return api.OK(SettingData{Value: value})
}

// SetSetting 设置单个配置项的值。
func (a *App) SetSetting(key, value string) api.Response[api.EmptyData] {
	if err := a.settingsRepo.Set(a.ctx, key, value); err != nil {
		code, msg := a.translateError(err)
		return api.Fail[api.EmptyData](code, msg)
	}

	return api.OK(api.EmptyData{})
}

// SetMultipleSettings 批量设置多个配置项。
func (a *App) SetMultipleSettings(settingsJSON string) api.Response[api.EmptyData] {
	var settings map[string]string
	if err := json.Unmarshal([]byte(settingsJSON), &settings); err != nil {
		code, msg := a.translateError(err)
		return api.Fail[api.EmptyData](code, msg)
	}

	if err := a.settingsRepo.SetMultiple(a.ctx, settings); err != nil {
		code, msg := a.translateError(err)
		return api.Fail[api.EmptyData](code, msg)
	}

	return api.OK(api.EmptyData{})
}

// ListConfigs 列出所有已保存的配置。
func (a *App) ListConfigs() api.Response[ConfigListData] {
	configs, err := a.configRepo.List(a.ctx)
	if err != nil {
		code, msg := a.translateError(err)
		return api.Fail[ConfigListData](code, msg)
	}

	return api.OK(ConfigListData{Configs: configs})
}

// GetConfig 根据 ID 获取指定配置。
func (a *App) GetConfig(id uint) api.Response[ConfigData] {
	config, err := a.configRepo.FindOne(a.ctx, id)
	if err != nil {
		code, msg := a.translateError(err)
		return api.Fail[ConfigData](code, msg)
	}

	return api.OK(ConfigData{Config: config})
}

// CreateNewConfig 创建一个新的空配置并保存到数据库。
func (a *App) CreateNewConfig(name string) api.Response[NewConfigData] {
	cfg := rulespec.NewConfig(name)

	config, err := a.configRepo.Create(a.ctx, cfg)
	if err != nil {
		code, msg := a.translateError(err)
		return api.Fail[NewConfigData](code, msg)
	}

	configJSON, err := json.Marshal(cfg)
	if err != nil {
		code, msg := a.translateError(err)
		return api.Fail[NewConfigData](code, msg)
	}

	a.log.Info("新配置已创建", "id", config.ID, "name", name, "configId", cfg.ID)
	return api.OK(NewConfigData{Config: config, ConfigJSON: string(configJSON)})
}

// GenerateNewRule 生成一个新的空规则
func (a *App) GenerateNewRule(name string, existingCount int) api.Response[NewRuleData] {
	rule := rulespec.NewRule(name, existingCount)
	ruleJSON, err := json.Marshal(rule)
	if err != nil {
		code, msg := a.translateError(err)
		return api.Fail[NewRuleData](code, msg)
	}

	return api.OK(NewRuleData{RuleJSON: string(ruleJSON)})
}

// SaveConfig 保存配置（创建或更新），dbID 为 0 时创建新配置。
func (a *App) SaveConfig(dbID uint, configJSON string) api.Response[ConfigData] {
	var cfg rulespec.Config
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		code, msg := a.translateError(err)
		return api.Fail[ConfigData](code, msg)
	}

	config, err := a.configRepo.Save(a.ctx, dbID, &cfg)
	if err != nil {
		code, msg := a.translateError(err)
		return api.Fail[ConfigData](code, msg)
	}

	a.log.Info("配置已保存", "dbID", config.ID, "configID", cfg.ID, "name", cfg.Name)
	return api.OK(ConfigData{Config: config})
}

// DeleteConfig 删除指定 ID 的配置。
func (a *App) DeleteConfig(id uint) api.Response[api.EmptyData] {
	if err := a.configRepo.Delete(a.ctx, id); err != nil {
		code, msg := a.translateError(err)
		return api.Fail[api.EmptyData](code, msg)
	}

	a.log.Info("配置已删除", "id", id)
	return api.OK(api.EmptyData{})
}

// SetActiveConfig 设置指定配置为当前激活状态。
func (a *App) SetActiveConfig(id uint) api.Response[api.EmptyData] {
	if err := a.configRepo.SetActive(a.ctx, id); err != nil {
		code, msg := a.translateError(err)
		return api.Fail[api.EmptyData](code, msg)
	}

	if err := a.settingsRepo.SetLastConfigID(a.ctx, fmt.Sprintf("%d", id)); err != nil {
		a.log.Warn("保存上次配置 ID 失败", "id", id, "error", err)
	}

	a.log.Debug("已设置激活配置", "id", id)
	return api.OK(api.EmptyData{})
}

// GetActiveConfig 获取当前激活的配置。
func (a *App) GetActiveConfig() api.Response[ConfigData] {
	config, err := a.configRepo.GetActive(a.ctx)
	if err != nil {
		code, msg := a.translateError(err)
		return api.Fail[ConfigData](code, msg)
	}

	return api.OK(ConfigData{Config: config})
}

// RenameConfig 重命名指定的配置。
func (a *App) RenameConfig(id uint, newName string) api.Response[api.EmptyData] {
	if err := a.configRepo.Rename(a.ctx, id, newName); err != nil {
		code, msg := a.translateError(err)
		return api.Fail[api.EmptyData](code, msg)
	}

	a.log.Debug("配置已重命名", "id", id, "newName", newName)
	return api.OK(api.EmptyData{})
}

// ImportConfig 导入配置（根据配置 ID 判断覆盖或新增）。
func (a *App) ImportConfig(configJSON string) api.Response[ConfigData] {
	var cfg rulespec.Config
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		code, msg := a.translateError(err)
		return api.Fail[ConfigData](code, msg)
	}

	config, err := a.configRepo.Upsert(a.ctx, &cfg)
	if err != nil {
		code, msg := a.translateError(err)
		return api.Fail[ConfigData](code, msg)
	}

	a.log.Info("配置已导入", "dbID", config.ID, "configID", cfg.ID, "name", cfg.Name)
	return api.OK(ConfigData{Config: config})
}

// LoadActiveConfigToSession 加载当前激活的配置到活跃会话。
func (a *App) LoadActiveConfigToSession() api.Response[api.EmptyData] {
	if a.currentSession == "" {
		code, msg := a.translateError(domain.ErrSessionNotFound)
		return api.Fail[api.EmptyData](code, msg)
	}

	config, err := a.configRepo.GetActive(a.ctx)
	if err != nil {
		code, msg := a.translateError(err)
		return api.Fail[api.EmptyData](code, msg)
	}
	if config == nil {
		code, msg := a.translateError(domain.ErrConfigNotFound)
		return api.Fail[api.EmptyData](code, msg)
	}

	cfg, err := a.configRepo.ToRulespecConfig(config)
	if err != nil {
		code, msg := a.translateError(err)
		return api.Fail[api.EmptyData](code, msg)
	}

	if err := a.service.LoadRules(a.ctx, a.currentSession, cfg); err != nil {
		code, msg := a.translateError(err)
		return api.Fail[api.EmptyData](code, msg)
	}

	a.log.Info("已加载激活配置到会话", "sessionID", a.currentSession, "configID", config.ID)
	return api.OK(api.EmptyData{})
}

// QueryMatchedEventHistory 根据条件查询匹配事件历史记录。
func (a *App) QueryMatchedEventHistory(sessionID, finalResult, url, method string, startTime, endTime int64, offset, limit int) api.Response[EventHistoryData] {
	if a.eventRepo == nil {
		code, msg := a.translateError(domain.ErrDatabaseNotInitialized)
		return api.Fail[EventHistoryData](code, msg)
	}

	events, total, err := a.eventRepo.Query(a.ctx, repo.QueryOptions{
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
		code, msg := a.translateError(err)
		return api.Fail[EventHistoryData](code, msg)
	}

	return api.OK(EventHistoryData{Events: events, Total: total})
}

// CleanupEventHistory 清理指定天数之前的旧事件记录。
func (a *App) CleanupEventHistory(retentionDays int) api.Response[api.EmptyData] {
	if a.eventRepo == nil {
		code, msg := a.translateError(domain.ErrDatabaseNotInitialized)
		return api.Fail[api.EmptyData](code, msg)
	}

	deleted, err := a.eventRepo.CleanupOldEvents(a.ctx, retentionDays)
	if err != nil {
		code, msg := a.translateError(err)
		return api.Fail[api.EmptyData](code, msg)
	}

	a.log.Info("已清理旧事件", "retentionDays", retentionDays, "deletedCount", deleted)
	return api.OK(api.EmptyData{})
}

// GetVersion 获取应用版本号
func (a *App) GetVersion() api.Response[VersionData] {
	return api.OK(VersionData{Version: a.cfg.Version})
}
