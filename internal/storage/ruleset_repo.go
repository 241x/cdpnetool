package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"cdpnetool/pkg/rulespec"

	"gorm.io/gorm"
)

// RuleSetRepo 规则集仓库
type RuleSetRepo struct{}

// NewRuleSetRepo 创建规则集仓库实例
func NewRuleSetRepo() *RuleSetRepo {
	return &RuleSetRepo{}
}

// Create 创建新规则集
func (r *RuleSetRepo) Create(name, version string, rules []rulespec.Rule) (*RuleSetRecord, error) {
	rulesJSON, err := json.Marshal(rules)
	if err != nil {
		return nil, fmt.Errorf("序列化规则失败: %w", err)
	}

	record := &RuleSetRecord{
		Name:      name,
		Version:   version,
		RulesJSON: string(rulesJSON),
		IsActive:  false,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := DB().Create(record).Error; err != nil {
		return nil, err
	}
	return record, nil
}

// Update 更新规则集
func (r *RuleSetRepo) Update(id uint, name, version string, rules []rulespec.Rule) error {
	rulesJSON, err := json.Marshal(rules)
	if err != nil {
		return fmt.Errorf("序列化规则失败: %w", err)
	}

	return DB().Model(&RuleSetRecord{}).Where("id = ?", id).Updates(map[string]interface{}{
		"name":       name,
		"version":    version,
		"rules_json": string(rulesJSON),
		"updated_at": time.Now(),
	}).Error
}

// Delete 删除规则集
func (r *RuleSetRepo) Delete(id uint) error {
	return DB().Delete(&RuleSetRecord{}, id).Error
}

// GetByID 根据 ID 获取规则集
func (r *RuleSetRepo) GetByID(id uint) (*RuleSetRecord, error) {
	var record RuleSetRecord
	if err := DB().First(&record, id).Error; err != nil {
		return nil, err
	}
	return &record, nil
}

// GetByName 根据名称获取规则集
func (r *RuleSetRepo) GetByName(name string) (*RuleSetRecord, error) {
	var record RuleSetRecord
	if err := DB().Where("name = ?", name).First(&record).Error; err != nil {
		return nil, err
	}
	return &record, nil
}

// List 列出所有规则集
func (r *RuleSetRepo) List() ([]RuleSetRecord, error) {
	var records []RuleSetRecord
	if err := DB().Order("updated_at DESC").Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

// SetActive 设置激活的规则集（只能有一个激活）
func (r *RuleSetRepo) SetActive(id uint) error {
	return DB().Transaction(func(tx *gorm.DB) error {
		// 先取消所有激活
		if err := tx.Model(&RuleSetRecord{}).Where("is_active = ?", true).Update("is_active", false).Error; err != nil {
			return err
		}
		// 激活指定规则集
		if err := tx.Model(&RuleSetRecord{}).Where("id = ?", id).Update("is_active", true).Error; err != nil {
			return err
		}
		return nil
	})
}

// GetActive 获取当前激活的规则集
func (r *RuleSetRepo) GetActive() (*RuleSetRecord, error) {
	var record RuleSetRecord
	if err := DB().Where("is_active = ?", true).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &record, nil
}

// ParseRules 从记录中解析规则
func (r *RuleSetRepo) ParseRules(record *RuleSetRecord) ([]rulespec.Rule, error) {
	if record == nil || record.RulesJSON == "" {
		return nil, nil
	}

	var rules []rulespec.Rule
	if err := json.Unmarshal([]byte(record.RulesJSON), &rules); err != nil {
		return nil, fmt.Errorf("解析规则失败: %w", err)
	}
	return rules, nil
}

// ToRuleSet 将记录转换为 RuleSet
func (r *RuleSetRepo) ToRuleSet(record *RuleSetRecord) (*rulespec.RuleSet, error) {
	rules, err := r.ParseRules(record)
	if err != nil {
		return nil, err
	}

	return &rulespec.RuleSet{
		Version: record.Version,
		Rules:   rules,
	}, nil
}

// SaveFromRuleSet 从 RuleSet 保存（更新或创建）
func (r *RuleSetRepo) SaveFromRuleSet(id uint, name string, rs *rulespec.RuleSet) (*RuleSetRecord, error) {
	if id == 0 {
		// 创建新记录
		return r.Create(name, rs.Version, rs.Rules)
	}
	// 更新现有记录
	if err := r.Update(id, name, rs.Version, rs.Rules); err != nil {
		return nil, err
	}
	return r.GetByID(id)
}

// Rename 重命名规则集
func (r *RuleSetRepo) Rename(id uint, newName string) error {
	return DB().Model(&RuleSetRecord{}).Where("id = ?", id).Updates(map[string]interface{}{
		"name":       newName,
		"updated_at": time.Now(),
	}).Error
}

// Duplicate 复制规则集
func (r *RuleSetRepo) Duplicate(id uint, newName string) (*RuleSetRecord, error) {
	original, err := r.GetByID(id)
	if err != nil {
		return nil, err
	}

	record := &RuleSetRecord{
		Name:      newName,
		Version:   original.Version,
		RulesJSON: original.RulesJSON,
		IsActive:  false,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := DB().Create(record).Error; err != nil {
		return nil, err
	}
	return record, nil
}
