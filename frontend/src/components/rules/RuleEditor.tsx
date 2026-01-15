import { useState } from 'react'
import { Input } from '@/components/ui/input'
import { Select } from '@/components/ui/select'
import { Button } from '@/components/ui/button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Badge } from '@/components/ui/badge'
import { ChevronDown, ChevronUp, Trash2, Copy, GripVertical, Power, PowerOff } from 'lucide-react'
import { ConditionGroup } from './ConditionEditor'
import { ActionsEditor } from './ActionEditor'
import type { Rule, Stage, Match, Action, Condition } from '@/types/rules'
import { ACTION_TYPE_LABELS, isTerminalAction } from '@/types/rules'

interface RuleEditorProps {
  rule: Rule
  onChange: (rule: Rule) => void
  onRemove: () => void
  onDuplicate: () => void
  isExpanded?: boolean
  onToggleExpand?: () => void
}

export function RuleEditor({
  rule,
  onChange,
  onRemove,
  onDuplicate,
  isExpanded = true,
  onToggleExpand
}: RuleEditorProps) {
  const [activeTab, setActiveTab] = useState<'match' | 'actions'>('match')

  const updateMatch = (key: keyof Match, conditions: Condition[]) => {
    onChange({
      ...rule,
      match: {
        ...rule.match,
        [key]: conditions.length > 0 ? conditions : undefined
      }
    })
  }

  const updateActions = (actions: Action[]) => {
    onChange({ ...rule, actions })
  }

  const toggleEnabled = (e: React.MouseEvent) => {
    e.stopPropagation()
    onChange({ ...rule, enabled: !rule.enabled })
  }

  // 计算条件数量摘要
  const conditionCount =
    (rule.match.allOf?.length || 0) +
    (rule.match.anyOf?.length || 0)

  // 获取行为摘要
  const getActionsSummary = () => {
    if (rule.actions.length === 0) return '无行为'
    const types = rule.actions.map(a => ACTION_TYPE_LABELS[a.type])
    if (types.length <= 2) return types.join(', ')
    return `${types[0]} 等 ${types.length} 个`
  }

  // 检查是否有终结性行为
  const hasTerminalAction = rule.actions.some(isTerminalAction)

  return (
    <div className={`border rounded-lg bg-card overflow-hidden ${!rule.enabled ? 'opacity-60' : ''}`}>
      {/* 折叠头部 */}
      <div
        className="flex items-center gap-3 p-3 bg-muted/50 cursor-pointer hover:bg-muted/70 transition-colors"
        onClick={onToggleExpand}
      >
        <GripVertical className="w-4 h-4 text-muted-foreground cursor-grab" />

        {/* 启用/禁用按钮 */}
        <Button
          variant="ghost"
          size="icon"
          className="h-7 w-7"
          onClick={toggleEnabled}
          title={rule.enabled ? '点击禁用' : '点击启用'}
        >
          {rule.enabled ? (
            <Power className="w-4 h-4 text-green-500" />
          ) : (
            <PowerOff className="w-4 h-4 text-muted-foreground" />
          )}
        </Button>

        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2">
            <span className="font-medium truncate">{rule.name || rule.id}</span>
            <Badge variant="outline" className="text-xs">
              {rule.stage === 'request' ? '请求' : '响应'}
            </Badge>
            <Badge variant="secondary" className="text-xs">
              优先级 {rule.priority}
            </Badge>
            {hasTerminalAction && (
              <Badge variant="destructive" className="text-xs">
                终结
              </Badge>
            )}
          </div>
          <div className="text-xs text-muted-foreground mt-0.5">
            {conditionCount} 个条件 · {getActionsSummary()}
          </div>
        </div>

        <div className="flex items-center gap-1">
          <Button variant="ghost" size="icon" onClick={(e) => { e.stopPropagation(); onDuplicate() }}>
            <Copy className="w-4 h-4" />
          </Button>
          <Button variant="ghost" size="icon" onClick={(e) => { e.stopPropagation(); onRemove() }}>
            <Trash2 className="w-4 h-4" />
          </Button>
          {isExpanded ? <ChevronUp className="w-4 h-4" /> : <ChevronDown className="w-4 h-4" />}
        </div>
      </div>

      {/* 展开内容 */}
      {isExpanded && (
        <div className="p-4 space-y-4">
          {/* 基础信息 */}
          <div className="grid grid-cols-4 gap-4">
            <div className="space-y-1">
              <label className="text-sm font-medium">规则 ID</label>
              <Input
                value={rule.id}
                onChange={(e) => onChange({ ...rule, id: e.target.value })}
                placeholder="唯一标识"
              />
            </div>
            <div className="space-y-1">
              <label className="text-sm font-medium">规则名称</label>
              <Input
                value={rule.name}
                onChange={(e) => onChange({ ...rule, name: e.target.value })}
                placeholder="规则描述"
              />
            </div>
            <div className="space-y-1">
              <label className="text-sm font-medium">执行阶段</label>
              <Select
                value={rule.stage}
                onChange={(e) => onChange({ ...rule, stage: e.target.value as Stage, actions: [] })}
                options={[
                  { value: 'request', label: '请求阶段' },
                  { value: 'response', label: '响应阶段' },
                ]}
              />
            </div>
            <div className="space-y-1">
              <label className="text-sm font-medium">优先级</label>
              <Input
                type="number"
                value={rule.priority}
                onChange={(e) => onChange({ ...rule, priority: parseInt(e.target.value) || 0 })}
                placeholder="数值越大越优先"
              />
            </div>
          </div>

          {/* Match 和 Actions 编辑区 */}
          <Tabs value={activeTab} onValueChange={(v) => setActiveTab(v as 'match' | 'actions')}>
            <TabsList>
              <TabsTrigger value="match">匹配条件</TabsTrigger>
              <TabsTrigger value="actions">执行行为 ({rule.actions.length})</TabsTrigger>
            </TabsList>

            <TabsContent value="match" className="space-y-4 pt-4">
              <ConditionGroup
                title="ALL OF (全部满足)"
                description="所有条件都必须为真才能命中"
                conditions={rule.match.allOf || []}
                onChange={(conditions) => updateMatch('allOf', conditions)}
              />

              <ConditionGroup
                title="ANY OF (任一满足)"
                description="任意一个条件为真即命中"
                conditions={rule.match.anyOf || []}
                onChange={(conditions) => updateMatch('anyOf', conditions)}
              />
            </TabsContent>

            <TabsContent value="actions" className="pt-4">
              <ActionsEditor
                actions={rule.actions}
                onChange={updateActions}
                stage={rule.stage}
              />
            </TabsContent>
          </Tabs>
        </div>
      )}
    </div>
  )
}

// ==================== 规则列表编辑器 ====================

interface RuleListEditorProps {
  rules: Rule[]
  onChange: (rules: Rule[]) => void
}

export function RuleListEditor({ rules, onChange }: RuleListEditorProps) {
  const [expandedRules, setExpandedRules] = useState<Set<string>>(new Set())

  const toggleExpand = (ruleId: string) => {
    const newSet = new Set(expandedRules)
    if (newSet.has(ruleId)) {
      newSet.delete(ruleId)
    } else {
      newSet.add(ruleId)
    }
    setExpandedRules(newSet)
  }

  const updateRule = (index: number, rule: Rule) => {
    const newRules = [...rules]
    newRules[index] = rule
    onChange(newRules)
  }

  const removeRule = (index: number) => {
    onChange(rules.filter((_, i) => i !== index))
  }

  const duplicateRule = (index: number) => {
    const rule = rules[index]
    const newRule: Rule = {
      ...JSON.parse(JSON.stringify(rule)),
      id: `${rule.id}_copy_${Date.now()}`,
      name: `${rule.name} (副本)`
    }
    const newRules = [...rules]
    newRules.splice(index + 1, 0, newRule)
    onChange(newRules)
    setExpandedRules(new Set([...expandedRules, newRule.id]))
  }

  // 按优先级从大到小排序展示
  const sortedRules = [...rules].sort((a, b) => b.priority - a.priority)

  return (
    <div className="space-y-3 pr-4">
      {sortedRules.map((rule) => {
        const originalIndex = rules.findIndex(r => r.id === rule.id)
        return (
          <RuleEditor
            key={rule.id}
            rule={rule}
            onChange={(r) => updateRule(originalIndex, r)}
            onRemove={() => removeRule(originalIndex)}
            onDuplicate={() => duplicateRule(originalIndex)}
            isExpanded={expandedRules.has(rule.id)}
            onToggleExpand={() => toggleExpand(rule.id)}
          />
        )
      })}

      {rules.length === 0 && (
        <div className="text-center text-muted-foreground p-8 border rounded-lg border-dashed">
          暂无规则，点击上方 "添加规则" 按钮创建
        </div>
      )}
    </div>
  )
}