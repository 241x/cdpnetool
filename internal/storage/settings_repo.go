package storage

import (
	"time"

	"gorm.io/gorm"
)

// SettingsRepo 设置仓库
type SettingsRepo struct{}

// NewSettingsRepo 创建设置仓库实例
func NewSettingsRepo() *SettingsRepo {
	return &SettingsRepo{}
}

// Get 获取设置值
func (r *SettingsRepo) Get(key string) (string, error) {
	var setting Setting
	result := DB().Where("key = ?", key).First(&setting)
	if result.Error != nil {
		return "", result.Error
	}
	return setting.Value, nil
}

// GetWithDefault 获取设置值，不存在时返回默认值
func (r *SettingsRepo) GetWithDefault(key, defaultValue string) string {
	val, err := r.Get(key)
	if err != nil {
		return defaultValue
	}
	return val
}

// Set 设置值（存在则更新，不存在则创建）
func (r *SettingsRepo) Set(key, value string) error {
	setting := Setting{
		Key:       key,
		Value:     value,
		UpdatedAt: time.Now(),
	}
	return DB().Save(&setting).Error
}

// Delete 删除设置
func (r *SettingsRepo) Delete(key string) error {
	return DB().Delete(&Setting{}, "key = ?", key).Error
}

// GetAll 获取所有设置
func (r *SettingsRepo) GetAll() (map[string]string, error) {
	var settings []Setting
	if err := DB().Find(&settings).Error; err != nil {
		return nil, err
	}

	result := make(map[string]string)
	for _, s := range settings {
		result[s.Key] = s.Value
	}
	return result, nil
}

// SetMultiple 批量设置
func (r *SettingsRepo) SetMultiple(kvs map[string]string) error {
	return DB().Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		for key, value := range kvs {
			setting := Setting{
				Key:       key,
				Value:     value,
				UpdatedAt: now,
			}
			if err := tx.Save(&setting).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// 便捷方法

// GetDevToolsURL 获取 DevTools URL
func (r *SettingsRepo) GetDevToolsURL() string {
	return r.GetWithDefault(SettingKeyDevToolsURL, "http://localhost:9222")
}

// SetDevToolsURL 设置 DevTools URL
func (r *SettingsRepo) SetDevToolsURL(url string) error {
	return r.Set(SettingKeyDevToolsURL, url)
}

// GetTheme 获取主题
func (r *SettingsRepo) GetTheme() string {
	return r.GetWithDefault(SettingKeyTheme, "system")
}

// SetTheme 设置主题
func (r *SettingsRepo) SetTheme(theme string) error {
	return r.Set(SettingKeyTheme, theme)
}

// GetLastRuleSetID 获取上次使用的规则集 ID
func (r *SettingsRepo) GetLastRuleSetID() string {
	return r.GetWithDefault(SettingKeyLastRuleSetID, "")
}

// SetLastRuleSetID 设置上次使用的规则集 ID
func (r *SettingsRepo) SetLastRuleSetID(id string) error {
	return r.Set(SettingKeyLastRuleSetID, id)
}
