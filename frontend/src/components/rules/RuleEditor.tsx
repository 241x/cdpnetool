import { useState, useMemo } from 'react'
import { Input } from '@/components/ui/input'
import { Select } from '@/components/ui/select'
import { Button } from '@/components/ui/button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Badge } from '@/components/ui/badge'
import { ChevronDown, ChevronUp, Trash2, GripVertical, Power, PowerOff } from 'lucide-react'
import { ConditionGroup } from './ConditionEditor'
import { ActionsEditor } from './ActionEditor'
import type { Rule, Stage, Match, Action, Condition } from '@/types/rules'
import { ACTION_TYPE_LABELS, isTerminalAction } from '@/types/rules'
import { useTranslation } from 'react-i18next'

interface RuleEditorProps {
  rule: Rule
  onChange: (rule: Rule) => void
  onRemove: () => void
  isExpanded?: boolean
  onToggleExpand?: () => void
}

export function RuleEditor({
  rule,
  onChange,
  onRemove,
  isExpanded = true,
  onToggleExpand
}: RuleEditorProps) {
  const { t } = useTranslation()
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
  const conditionCount = useMemo(
    () => (rule.match.allOf?.length || 0) + (rule.match.anyOf?.length || 0),
    [rule.match.allOf, rule.match.anyOf]
  )

  // 获取行为摘要
  const actionsSummary = useMemo(() => {
    if (rule.actions.length === 0) return 'No actions'
    const types = rule.actions.map(a => ACTION_TYPE_LABELS[a.type])
    if (types.length <= 2) return types.join(', ')
    return `${types[0]} & ${types.length - 1} more`
  }, [rule.actions])

  // 检查是否有终结性行为
  const hasTerminalAction = useMemo(
    () => rule.actions.some(isTerminalAction),
    [rule.actions]
  )

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
          title={rule.enabled ? t('rules.enabled') : t('rules.disabled')}
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
              {rule.stage === 'request' ? t('rules.requestStage') : t('rules.responseStage')}
            </Badge>
            <Badge variant="secondary" className="text-xs">
              {t('rules.priority')} {rule.priority}
            </Badge>
            {hasTerminalAction && (
              <Badge variant="destructive" className="text-xs">
                {t('rules.terminal')}
              </Badge>
            )}
          </div>
          <div className="text-xs text-muted-foreground mt-0.5">
            {conditionCount} {t('rules.conditions')} · {actionsSummary}
          </div>
        </div>

        <div className="flex items-center gap-1">
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
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
            <div className="space-y-1">
              <label className="text-sm font-medium">ID</label>
              <Input
                value={rule.id}
                onChange={(e) => onChange({ ...rule, id: e.target.value })}
                placeholder="ID"
              />
            </div>
            <div className="space-y-1">
              <label className="text-sm font-medium">Name</label>
              <Input
                value={rule.name}
                onChange={(e) => onChange({ ...rule, name: e.target.value })}
                placeholder="Name"
              />
            </div>
            <div className="space-y-1">
              <label className="text-sm font-medium">{t('rules.stage')}</label>
              <Select
                value={rule.stage}
                onChange={(e) => onChange({ ...rule, stage: e.target.value as Stage, actions: [] })}
                options={[
                  { value: 'request', label: t('rules.requestStage') },
                  { value: 'response', label: t('rules.responseStage') },
                ]}
              />
            </div>
            <div className="space-y-1">
              <label className="text-sm font-medium">{t('rules.priority')}</label>
              <Input
                type="number"
                value={rule.priority}
                onChange={(e) => onChange({ ...rule, priority: parseInt(e.target.value) || 0 })}
                placeholder="0"
              />
            </div>
          </div>

          {/* Match 和 Actions 编辑区 */}
          <Tabs value={activeTab} onValueChange={(v) => setActiveTab(v as 'match' | 'actions')}>
            <TabsList>
              <TabsTrigger value="match">{t('rules.conditions')}</TabsTrigger>
              <TabsTrigger value="actions">{t('rules.actions')} ({rule.actions.length})</TabsTrigger>
            </TabsList>

            <TabsContent value="match" className="space-y-4 pt-4">
              <ConditionGroup
                title={t('rules.allOf')}
                description={t('rules.allOfDesc')}
                conditions={rule.match.allOf || []}
                onChange={(conditions) => updateMatch('allOf', conditions)}
              />

              <ConditionGroup
                title={t('rules.anyOf')}
                description={t('rules.anyOfDesc')}
                conditions={rule.match.anyOf || []}
                onChange={(conditions) => updateMatch('anyOf', conditions)}
              />
            </TabsContent>

            <TabsContent value="actions" className="space-y-4 pt-4">
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

interface RuleListEditorProps {
  rules: Rule[]
  onChange: (rules: Rule[]) => void
}

export function RuleListEditor({ rules, onChange }: RuleListEditorProps) {
  const { t } = useTranslation()
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

  // 按优先级从大到小排序展示
  const sortedRules = [...rules].sort((a, b) => b.priority - a.priority)

  return (
    <div className="space-y-3">
      {sortedRules.map((rule) => {
        const originalIndex = rules.findIndex(r => r.id === rule.id)
        return (
          <RuleEditor
            key={rule.id}
            rule={rule}
            onChange={(r) => updateRule(originalIndex, r)}
            onRemove={() => removeRule(originalIndex)}
            isExpanded={expandedRules.has(rule.id)}
            onToggleExpand={() => toggleExpand(rule.id)}
          />
        )
      })}

      {rules.length === 0 && (
        <div className="text-center text-muted-foreground p-8 border rounded-lg border-dashed">
          {t('rules.noRules')}
        </div>
      )}
    </div>
  )
}