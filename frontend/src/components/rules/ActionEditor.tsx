import { Input } from '@/components/ui/input'
import { Select } from '@/components/ui/select'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { Badge } from '@/components/ui/badge'
import { X, Plus, Trash2, GripVertical, AlertCircle } from 'lucide-react'
import type { Action, ActionType, Stage, JSONPatchOp, BodyEncoding } from '@/types/rules'
import {
  ACTION_TYPE_LABELS,
  createEmptyAction,
  isTerminalAction,
  getActionsForStage
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
    label: ACTION_TYPE_LABELS[type]
  }))
}

export function ActionEditor({ action, onChange, onRemove, stage }: ActionEditorProps) {
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
                终结性
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
  const updateField = <K extends keyof Action>(key: K, value: Action[K]) => {
    onChange({ ...action, [key]: value })
  }

  switch (action.type) {
    case 'setUrl':
      return (
        <Input
          value={(action.value as string) || ''}
          onChange={(e) => updateField('value', e.target.value)}
          placeholder="新的 URL..."
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
            className="w-40"
          />
          <Input
            value={(action.value as string) || ''}
            onChange={(e) => updateField('value', e.target.value)}
            placeholder="值..."
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
                { value: 'text', label: '文本' },
                { value: 'base64', label: 'Base64' },
              ]}
              className="w-28"
            />
          </div>
          <Textarea
            value={(action.value as string) || ''}
            onChange={(e) => updateField('value', e.target.value)}
            placeholder={
              action.type === 'appendBody'
                ? (action.encoding === 'base64' ? '追加的 Base64 编码内容...' : '追加的内容...')
                : (action.encoding === 'base64' ? 'Base64 编码内容...' : 'Body 内容...')
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
              placeholder="搜索文本..."
              className="flex-1"
            />
            <Input
              value={action.replace || ''}
              onChange={(e) => updateField('replace', e.target.value)}
              placeholder="替换为..."
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
            替换所有匹配
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
          placeholder="状态码"
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
              placeholder="状态码"
              min={100}
              max={599}
              className="w-24"
            />
            <Select
              value={action.bodyEncoding || 'text'}
              onChange={(e) => onChange({ ...action, bodyEncoding: e.target.value as BodyEncoding })}
              options={[
                { value: 'text', label: '文本' },
                { value: 'base64', label: 'Base64' },
              ]}
              className="w-28"
            />
          </div>
          <KeyValueEditor
            title="响应头"
            data={action.headers || {}}
            onChange={(headers) => onChange({ ...action, headers })}
          />
          <Textarea
            value={action.body || ''}
            onChange={(e) => onChange({ ...action, body: e.target.value })}
            placeholder={action.bodyEncoding === 'base64' ? 'Base64 编码的响应体...' : '响应体内容...'}
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
          添加
        </Button>
      </div>

      {entries.length === 0 ? (
        <div className="text-sm text-muted-foreground p-2 border rounded border-dashed text-center">
          暂无配置
        </div>
      ) : (
        <div className="space-y-2">
          {entries.map(([key, value], index) => (
            <div key={index} className="flex items-center gap-2">
              <Input
                value={key}
                onChange={(e) => updateEntry(key, e.target.value, value)}
                placeholder="键"
                className="flex-1"
              />
              <Input
                value={value}
                onChange={(e) => updateEntry(key, key, e.target.value)}
                placeholder="值"
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
  const opOptions = [
    { value: 'add', label: '添加' },
    { value: 'remove', label: '删除' },
    { value: 'replace', label: '替换' },
    { value: 'move', label: '移动' },
    { value: 'copy', label: '复制' },
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
        <label className="text-sm font-medium">JSON Patch 操作</label>
        <Button variant="outline" size="sm" onClick={addPatch}>
          <Plus className="w-4 h-4 mr-1" />
          添加操作
        </Button>
      </div>

      {patches.length === 0 ? (
        <div className="text-sm text-muted-foreground p-2 border rounded border-dashed text-center">
          暂无 JSON Patch 操作
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
                placeholder="路径 (如 /data/id)"
                className="w-36"
              />
              {(patch.op === 'move' || patch.op === 'copy') && (
                <Input
                  value={patch.from || ''}
                  onChange={(e) => updatePatch(index, { ...patch, from: e.target.value })}
                  placeholder="源路径"
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
                  placeholder="值 (JSON)"
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
}

// 行为列表编辑器
export function ActionsEditor({ actions, onChange, stage }: ActionsEditorProps) {
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
        <div>
          <h4 className="font-medium">执行行为</h4>
          <p className="text-xs text-muted-foreground">按顺序依次执行，终结性行为会中断后续执行</p>
        </div>
        <Button variant="outline" size="sm" onClick={addAction}>
          <Plus className="w-4 h-4 mr-1" />
          添加行为
        </Button>
      </div>

      {actions.length === 0 ? (
        <div className="text-sm text-muted-foreground p-4 border rounded-lg border-dashed text-center">
          暂无行为，点击上方按钮添加
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
