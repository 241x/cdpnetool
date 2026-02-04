import { Input } from '@/components/ui/input'
import { Select } from '@/components/ui/select'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { Badge } from '@/components/ui/badge'
import { X, Plus, Trash2, GripVertical, AlertCircle } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { Action, ActionType, Stage, JSONPatchOp, BodyEncoding } from '@/types/rules'
import {
  createEmptyAction,
  isTerminalAction,
  getActionsForStage,
  getActionTypeLabel
} from '@/types/rules'

interface ActionEditorProps {
  action: Action
  onChange: (action: Action) => void
  onRemove: () => void
  stage: Stage
}

// 获取行为类型选项
function getActionTypeOptions(stage: Stage): { value: ActionType; label: string }[] {
  const actions = getActionsForStage(stage)
  return actions.map(type => ({
    value: type,
    label: getActionTypeLabel(type)
  }))
}

export function ActionEditor({ action, onChange, onRemove, stage }: ActionEditorProps) {
  const { t } = useTranslation()
  const handleTypeChange = (newType: ActionType) => {
    onChange(createEmptyAction(newType, stage))
  }

  const isTerminal = isTerminalAction(action)

  return (
    <div className={`p-3 rounded-lg border bg-card ${isTerminal ? 'border-destructive/50 bg-destructive/5' : ''}`}>
      <div className="flex items-start gap-2">
        <GripVertical className="w-4 h-4 text-muted-foreground mt-2 cursor-grab shrink-0" />
        
        {/* 行为类型选择 */}
        <div className="flex-1 space-y-3">
          <div className="flex items-center gap-2">
            <Select
              value={action.type}
              onChange={(e) => handleTypeChange(e.target.value as ActionType)}
              options={getActionTypeOptions(stage)}
              className="w-40"
            />
            {isTerminal && (
              <Badge variant="destructive" className="text-xs">
                <AlertCircle className="w-3 h-3 mr-1" />
                {t('rules.terminalAction')}
              </Badge>
            )}
          </div>

          {/* 根据行为类型渲染字段 */}
          {renderActionFields(action, onChange)}
        </div>

        {/* 删除按钮 */}
        <Button variant="ghost" size="icon" onClick={onRemove} className="shrink-0">
          <X className="w-4 h-4" />
        </Button>
      </div>
    </div>
  )
}

// 渲染行为字段
function renderActionFields(action: Action, onChange: (action: Action) => void) {
  const { t } = useTranslation()
  const updateField = <K extends keyof Action>(key: K, value: Action[K]) => {
    onChange({ ...action, [key]: value })
  }

  switch (action.type) {
    case 'setUrl':
      return (
        <Input
          value={(action.value as string) || ''}
          onChange={(e) => updateField('value', e.target.value)}
          placeholder={t('rules.newUrl')}
        />
      )

    case 'setMethod':
      return (
        <Select
          value={(action.value as string) || ''}
          onChange={(e) => updateField('value', e.target.value)}
          options={[
            { value: 'GET', label: 'GET' },
            { value: 'POST', label: 'POST' },
            { value: 'PUT', label: 'PUT' },
            { value: 'DELETE', label: 'DELETE' },
            { value: 'PATCH', label: 'PATCH' },
            { value: 'HEAD', label: 'HEAD' },
            { value: 'OPTIONS', label: 'OPTIONS' },
          ]}
          className="w-32"
        />
      )

    case 'setHeader':
    case 'setQueryParam':
    case 'setCookie':
    case 'setFormField':
      return (
        <div className="flex items-center gap-2">
          <Input
            value={action.name || ''}
            onChange={(e) => updateField('name', e.target.value)}
            placeholder={getNamePlaceholder(action.type)}
            className="flex-1"
          />
          <Input
            value={(action.value as string) || ''}
            onChange={(e) => updateField('value', e.target.value)}
            placeholder={t('rules.headerValue')}
            className="flex-1"
          />
        </div>
      )

    case 'removeHeader':
    case 'removeQueryParam':
    case 'removeCookie':
    case 'removeFormField':
      return (
        <Input
          value={action.name || ''}
          onChange={(e) => updateField('name', e.target.value)}
          placeholder={getNamePlaceholder(action.type)}
          className="w-60"
        />
      )

    case 'setBody':
    case 'appendBody':
      return (
        <div className="space-y-2">
          <div className="flex items-center gap-2">
            <Select
              value={action.encoding || 'text'}
              onChange={(e) => updateField('encoding', e.target.value as BodyEncoding)}
              options={[
                { value: 'text', label: t('rules.textEncoding') },
                { value: 'base64', label: t('rules.base64Encoding') },
              ]}
              className="w-28"
            />
          </div>
          <Textarea
            value={(action.value as string) || ''}
            onChange={(e) => updateField('value', e.target.value)}
            placeholder={
              action.type === 'appendBody'
                ? (action.encoding === 'base64' ? t('rules.appendBase64Content') : t('rules.appendContent'))
                : (action.encoding === 'base64' ? t('rules.base64Content') : t('rules.bodyContent'))
            }
            rows={4}
            className="font-mono text-sm"
          />
        </div>
      )

    case 'replaceBodyText':
      return (
        <div className="space-y-2">
          <div className="flex items-center gap-2">
            <Input
              value={action.search || ''}
              onChange={(e) => updateField('search', e.target.value)}
              placeholder={t('rules.searchText')}
              className="flex-1"
            />
            <Input
              value={action.replace || ''}
              onChange={(e) => updateField('replace', e.target.value)}
              placeholder={t('rules.replaceWith')}
              className="flex-1"
            />
          </div>
          <label className="flex items-center gap-2 text-sm cursor-pointer">
            <input
              type="checkbox"
              checked={action.replaceAll || false}
              onChange={(e) => updateField('replaceAll', e.target.checked)}
              className="rounded"
            />
            {t('rules.replaceAll')}
          </label>
        </div>
      )

    case 'patchBodyJson':
      return (
        <JSONPatchEditor
          patches={action.patches || []}
          onChange={(patches) => updateField('patches', patches)}
        />
      )

    case 'setStatus':
      return (
        <Input
          type="number"
          value={(action.value as number) || 200}
          onChange={(e) => updateField('value', parseInt(e.target.value) || 200)}
          placeholder={t('rules.statusCode')}
          min={100}
          max={599}
          className="w-24"
        />
      )

    case 'block':
      return (
        <div className="space-y-3">
          <div className="flex items-center gap-2">
            <Input
              type="number"
              value={action.statusCode || 200}
              onChange={(e) => onChange({ ...action, statusCode: parseInt(e.target.value) || 200 })}
              placeholder={t('rules.statusCode')}
              min={100}
              max={599}
              className="w-24"
            />
            <Select
              value={action.bodyEncoding || 'text'}
              onChange={(e) => onChange({ ...action, bodyEncoding: e.target.value as BodyEncoding })}
              options={[
                { value: 'text', label: t('rules.textEncoding') },
                { value: 'base64', label: t('rules.base64Encoding') },
              ]}
              className="w-28"
            />
          </div>
          <KeyValueEditor
            title={t('rules.responseHeaders')}
            data={action.headers || {}}
            onChange={(headers) => onChange({ ...action, headers })}
          />
          <Textarea
            value={action.body || ''}
            onChange={(e) => onChange({ ...action, body: e.target.value })}
            placeholder={action.bodyEncoding === 'base64' ? t('rules.base64ResponseBody') : t('rules.responseBody')}
            rows={4}
            className="font-mono text-sm"
          />
        </div>
      )

    default:
      return null
  }
}

// 获取 name 字段占位符
function getNamePlaceholder(type: ActionType): string {
  switch (type) {
    case 'setHeader':
    case 'removeHeader':
      return 'Header 名'
    case 'setQueryParam':
    case 'removeQueryParam':
      return '参数名'
    case 'setCookie':
    case 'removeCookie':
      return 'Cookie 名'
    case 'setFormField':
    case 'removeFormField':
      return '字段名'
    default:
      return '名称'
  }
}

interface KeyValueEditorProps {
  title: string
  data: Record<string, string>
  onChange: (data: Record<string, string>) => void
}

// Key-Value 编辑器
function KeyValueEditor({ title, data, onChange }: KeyValueEditorProps) {
  const { t } = useTranslation()
  const entries = Object.entries(data)

  const addEntry = () => {
    onChange({ ...data, '': '' })
  }

  const updateEntry = (oldKey: string, newKey: string, value: string) => {
    const newData = { ...data }
    if (oldKey !== newKey) {
      delete newData[oldKey]
    }
    newData[newKey] = value
    onChange(newData)
  }

  const removeEntry = (key: string) => {
    const newData = { ...data }
    delete newData[key]
    onChange(newData)
  }

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <label className="text-sm font-medium">{title}</label>
        <Button variant="outline" size="sm" onClick={addEntry}>
          <Plus className="w-4 h-4 mr-1" />
          {t('common.add')}
        </Button>
      </div>

      {entries.length === 0 ? (
        <div className="text-sm text-muted-foreground p-2 border rounded border-dashed text-center">
          {t('rules.kvNoConfig')}
        </div>
      ) : (
        <div className="space-y-2">
          {entries.map(([key, value], index) => (
            <div key={index} className="flex items-center gap-2">
              <Input
                value={key}
                onChange={(e) => updateEntry(key, e.target.value, value)}
                placeholder={t('rules.kvKey')}
                className="flex-1"
              />
              <Input
                value={value}
                onChange={(e) => updateEntry(key, key, e.target.value)}
                placeholder={t('rules.kvValue')}
                className="flex-1"
              />
              <Button variant="ghost" size="icon" onClick={() => removeEntry(key)}>
                <X className="w-4 h-4" />
              </Button>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

interface JSONPatchEditorProps {
  patches: JSONPatchOp[]
  onChange: (patches: JSONPatchOp[]) => void
}

// JSON Patch 编辑器
function JSONPatchEditor({ patches, onChange }: JSONPatchEditorProps) {
  const { t } = useTranslation()
  const opOptions = [
    { value: 'add', label: t('rules.patchOpAdd') },
    { value: 'remove', label: t('rules.patchOpRemove') },
    { value: 'replace', label: t('rules.patchOpReplace') },
    { value: 'move', label: t('rules.patchOpMove') },
    { value: 'copy', label: t('rules.patchOpCopy') },
  ]

  const addPatch = () => {
    onChange([...patches, { op: 'add', path: '', value: '' }])
  }

  const updatePatch = (index: number, patch: JSONPatchOp) => {
    const newPatches = [...patches]
    newPatches[index] = patch
    onChange(newPatches)
  }

  const removePatch = (index: number) => {
    onChange(patches.filter((_, i) => i !== index))
  }

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <label className="text-sm font-medium">{t('rules.jsonPatchOps')}</label>
        <Button variant="outline" size="sm" onClick={addPatch}>
          <Plus className="w-4 h-4 mr-1" />
          {t('rules.addOperation')}
        </Button>
      </div>

      {patches.length === 0 ? (
        <div className="text-sm text-muted-foreground p-2 border rounded border-dashed text-center">
          {t('rules.noJsonPatch')}
        </div>
      ) : (
        <div className="space-y-2">
          {patches.map((patch, index) => (
            <div key={index} className="flex items-center gap-2 p-2 border rounded bg-muted/30">
              <Select
                value={patch.op}
                onChange={(e) => updatePatch(index, { ...patch, op: e.target.value as JSONPatchOp['op'] })}
                options={opOptions}
                className="w-24"
              />
              <Input
                value={patch.path}
                onChange={(e) => updatePatch(index, { ...patch, path: e.target.value })}
                placeholder={t('rules.jsonPath')}
                className="w-36"
              />
              {(patch.op === 'move' || patch.op === 'copy') && (
                <Input
                  value={patch.from || ''}
                  onChange={(e) => updatePatch(index, { ...patch, from: e.target.value })}
                  placeholder={t('rules.jsonSourcePath')}
                  className="w-32"
                />
              )}
              {(patch.op === 'add' || patch.op === 'replace') && (
                <Input
                  value={typeof patch.value === 'string' ? patch.value : JSON.stringify(patch.value)}
                  onChange={(e) => {
                    let val: any = e.target.value
                    try { val = JSON.parse(e.target.value) } catch { }
                    updatePatch(index, { ...patch, value: val })
                  }}
                  placeholder={t('rules.jsonValue')}
                  className="flex-1"
                />
              )}
              <Button variant="ghost" size="icon" onClick={() => removePatch(index)}>
                <Trash2 className="w-4 h-4" />
              </Button>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

interface ActionsEditorProps {
  actions: Action[]
  onChange: (actions: Action[]) => void
  stage: Stage
  onStageChange?: (stage: Stage) => void
}

// 行为列表编辑器
export function ActionsEditor({ actions, onChange, stage, onStageChange }: ActionsEditorProps) {
  const { t } = useTranslation()
  const addAction = () => {
    const defaultType = stage === 'request' ? 'setHeader' : 'setHeader'
    onChange([...actions, createEmptyAction(defaultType, stage)])
  }

  const updateAction = (index: number, action: Action) => {
    const newActions = [...actions]
    newActions[index] = action
    onChange(newActions)
  }

  const removeAction = (index: number) => {
    onChange(actions.filter((_, i) => i !== index))
  }

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <div className="flex items-center bg-muted rounded-md p-0.5">
            <button
              className={`px-3 py-1 text-xs font-medium rounded transition-all ${
                stage === 'request' 
                  ? 'bg-background text-foreground shadow-sm' 
                  : 'text-muted-foreground hover:text-foreground'
              }`}
              onClick={() => onStageChange?.('request')}
            >
              {t('rules.requestStage')}
            </button>
            <button
              className={`px-3 py-1 text-xs font-medium rounded transition-all ${
                stage === 'response' 
                  ? 'bg-background text-foreground shadow-sm' 
                  : 'text-muted-foreground hover:text-foreground'
              }`}
              onClick={() => onStageChange?.('response')}
            >
              {t('rules.responseStage')}
            </button>
          </div>
          <span className="text-xs text-muted-foreground">
            {stage === 'request' ? t('rules.requestStageDesc') : t('rules.responseStageDesc')}
          </span>
        </div>
        <Button variant="outline" size="sm" onClick={addAction}>
          <Plus className="w-4 h-4 mr-1" />
          {t('rules.addAction')}
        </Button>
      </div>

      {actions.length === 0 ? (
        <div className="text-sm text-muted-foreground p-4 border rounded-lg border-dashed text-center">
          {t('rules.noActions')}
        </div>
      ) : (
        <div className="space-y-2">
          {actions.map((action, index) => (
            <ActionEditor
              key={index}
              action={action}
              onChange={(a) => updateAction(index, a)}
              onRemove={() => removeAction(index)}
              stage={stage}
            />
          ))}
        </div>
      )}
    </div>
  )
}
