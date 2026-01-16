import { useState, useEffect, useRef } from 'react'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Switch } from '@/components/ui/switch'
import { Toaster } from '@/components/ui/toaster'
import { useToast } from '@/hooks/use-toast'
import { useSessionStore, useThemeStore } from '@/stores'
import { RuleListEditor } from '@/components/rules'
import { EventsPanel } from '@/components/events'
import type { Rule, Config } from '@/types/rules'
import type { InterceptEvent } from '@/types/events'
import { createEmptyConfig } from '@/types/rules'
import { 
  RefreshCw, 
  Moon, 
  Sun,
  Link2,
  Link2Off,
  FileJson,
  Activity,
  Plus,
  Download,
  Upload,
  Save,
  Chrome,
  Trash2,
  ChevronDown,
  ChevronRight
} from 'lucide-react'

// 配置记录类型
interface ConfigRecord {
  id: number
  name: string
  description: string
  version: string
  rulesJson: string
  isActive: boolean
  createdAt: string
  updatedAt: string
}

interface OperationResult {
  success: boolean
  error?: string
}

// Wails 生成的绑定（需要在 wails dev 后生成）
declare global {
  interface Window {
    go: {
      gui: {
        App: {
          StartSession: (url: string) => Promise<{ sessionId: string; success: boolean; error?: string }>
          StopSession: (id: string) => Promise<{ success: boolean; error?: string }>
          ListTargets: (id: string) => Promise<{ targets: any[]; success: boolean; error?: string }>
          AttachTarget: (sid: string, tid: string) => Promise<{ success: boolean; error?: string }>
          DetachTarget: (sid: string, tid: string) => Promise<{ success: boolean; error?: string }>
          EnableInterception: (id: string) => Promise<{ success: boolean; error?: string }>
          DisableInterception: (id: string) => Promise<{ success: boolean; error?: string }>
          LoadRules: (id: string, json: string) => Promise<{ success: boolean; error?: string }>
          GetRuleStats: (id: string) => Promise<{ stats: any; success: boolean; error?: string }>
          ApproveRequest: (itemId: string, mutationsJson: string) => Promise<{ success: boolean; error?: string }>
          ApproveResponse: (itemId: string, mutationsJson: string) => Promise<{ success: boolean; error?: string }>
          Reject: (itemId: string) => Promise<{ success: boolean; error?: string }>
          LaunchBrowser: (headless: boolean) => Promise<{ devToolsUrl: string; success: boolean; error?: string }>
          CloseBrowser: () => Promise<{ success: boolean; error?: string }>
          GetBrowserStatus: () => Promise<{ devToolsUrl: string; success: boolean; error?: string }>
          // 配置持久化 API
          ListConfigs: () => Promise<{ configs: ConfigRecord[]; success: boolean; error?: string }>
          GetConfig: (id: number) => Promise<{ config: ConfigRecord; success: boolean; error?: string }>
          SaveConfig: (id: number, name: string, description: string, rulesJson: string) => Promise<{ config: ConfigRecord; success: boolean; error?: string }>
          DeleteConfig: (id: number) => Promise<{ success: boolean; error?: string }>
          SetActiveConfig: (id: number) => Promise<{ success: boolean; error?: string }>
          GetActiveConfig: () => Promise<{ config: ConfigRecord | null; success: boolean; error?: string }>
          RenameConfig: (id: number, newName: string) => Promise<{ success: boolean; error?: string }>
          SetDirty: (dirty: boolean) => Promise<void>
          ExportConfig: (name: string, json: string) => Promise<OperationResult>
          // 创建新配置/规则 API（生成 UUID）
          CreateNewConfig: (name: string) => Promise<{ config: ConfigRecord; configJson: string; success: boolean; error?: string }>
          GenerateNewRule: (name: string) => Promise<{ ruleJson: string; success: boolean; error?: string }>
        }
      }
    }
  }
}

function App() {
  const { 
    devToolsURL, 
    setDevToolsURL, 
    currentSessionId, 
    setCurrentSession,
    isConnected,
    setConnected,
    setIntercepting,
    targets,
    setTargets,
    attachedTargets,
    toggleAttachedTarget,
    events,
    addEvent,
    clearEvents,
  } = useSessionStore()
  
  const { isDark, toggle: toggleTheme } = useThemeStore()
  const { toast } = useToast()
  const [isLoading, setIsLoading] = useState(false)
  const [isLaunchingBrowser, setIsLaunchingBrowser] = useState(false)

  // 启动浏览器
  const handleLaunchBrowser = async () => {
    setIsLaunchingBrowser(true)
    try {
      const result = await window.go?.gui?.App?.LaunchBrowser(false)
      if (result?.success) {
        setDevToolsURL(result.devToolsUrl)
        toast({
          variant: 'success',
          title: '浏览器已启动',
          description: `DevTools URL: ${result.devToolsUrl}`,
        })
      } else {
        toast({
          variant: 'destructive',
          title: '启动失败',
          description: result?.error || '无法启动浏览器',
        })
      }
    } catch (e) {
      toast({
        variant: 'destructive',
        title: '启动错误',
        description: String(e),
      })
    } finally {
      setIsLaunchingBrowser(false)
    }
  }

  // 连接/断开会话
  const handleConnect = async () => {
    if (isConnected && currentSessionId) {
      // 断开
      try {
        const result = await window.go?.gui?.App?.StopSession(currentSessionId)
        if (result?.success) {
          setConnected(false)
          setCurrentSession(null)
          setIntercepting(false)
          setTargets([])
          toast({
            variant: 'success',
            title: '已断开连接',
          })
        } else {
          toast({
            variant: 'destructive',
            title: '断开失败',
            description: result?.error,
          })
        }
      } catch (e) {
        toast({
          variant: 'destructive',
          title: '断开错误',
          description: String(e),
        })
      }
    } else {
      // 连接
      setIsLoading(true)
      try {
        const result = await window.go?.gui?.App?.StartSession(devToolsURL)
        if (result?.success) {
          setCurrentSession(result.sessionId)
          setConnected(true)
          toast({
            variant: 'success',
            title: '连接成功',
            description: `会话 ID: ${result.sessionId.slice(0, 8)}...`,
          })
          // 自动获取目标列表
          await refreshTargets(result.sessionId)
        } else {
          // 优化连接失败提示
          let errorMessage = result?.error || '连接失败'
          if (errorMessage.includes('connection refused') || errorMessage.includes('dial tcp') || errorMessage.includes('ECONNREFUSED')) {
            errorMessage = '无法连接到浏览器，请先点击「启动浏览器」按钮启动一个浏览器实例'
          } else if (errorMessage.includes('invalid') || errorMessage.includes('Invalid')) {
            errorMessage = 'DevTools URL 格式无效，请检查 URL 是否正确'
          }
          toast({
            variant: 'destructive',
            title: '连接失败',
            description: errorMessage,
          })
        }
      } catch (e) {
        toast({
          variant: 'destructive',
          title: '连接错误',
          description: String(e),
        })
      } finally {
        setIsLoading(false)
      }
    }
  }

  // 刷新目标列表
  const refreshTargets = async (sessionId?: string) => {
    const sid = sessionId || currentSessionId
    if (!sid) return
    
    try {
      const result = await window.go?.gui?.App?.ListTargets(sid)
      if (result?.success) {
        setTargets(result.targets || [])
      }
    } catch (e) {
      console.error('List targets error:', e)
    }
  }

  // 附加/移除目标
  const handleToggleTarget = async (targetId: string) => {
    if (!currentSessionId) return
    
    const isAttached = attachedTargets.has(targetId)
    try {
      if (isAttached) {
        const result = await window.go?.gui?.App?.DetachTarget(currentSessionId, targetId)
        if (result?.success) {
          toggleAttachedTarget(targetId)
          toast({
            variant: 'success',
            title: '已移除目标',
          })
        } else {
          toast({
            variant: 'destructive',
            title: '移除失败',
            description: result?.error,
          })
        }
      } else {
        const result = await window.go?.gui?.App?.AttachTarget(currentSessionId, targetId)
        if (result?.success) {
          toggleAttachedTarget(targetId)
          toast({
            variant: 'success',
            title: '已附加目标',
          })
        } else {
          toast({
            variant: 'destructive',
            title: '附加失败',
            description: result?.error,
          })
        }
      }
    } catch (e) {
      toast({
        variant: 'destructive',
        title: '操作错误',
        description: String(e),
      })
    }
  }

  // 监听 Wails 事件
  useEffect(() => {
    // @ts-ignore
    if (window.runtime?.EventsOn) {
      // @ts-ignore
      window.runtime.EventsOn('intercept-event', (event: InterceptEvent) => {
        // 后端已提供完整事件数据，生成 id 用于前端 key
        const enrichedEvent: InterceptEvent = {
          ...event,
          id: event.id || `${event.timestamp}_${Math.random().toString(36).slice(2)}`,
        }
        addEvent(enrichedEvent)
      })
    }
  }, [addEvent])

  return (
    <div className="h-screen flex flex-col bg-background text-foreground">
      {/* 顶部工具栏 */}
      <div className="h-14 border-b flex items-center px-4 gap-4 shrink-0">
        <div className="flex items-center gap-2 flex-1">
          <Button
            onClick={handleLaunchBrowser}
            variant="outline"
            disabled={isLaunchingBrowser || isConnected}
            title="启动新浏览器实例"
          >
            <Chrome className="w-4 h-4 mr-2" />
            {isLaunchingBrowser ? '启动中...' : '启动浏览器'}
          </Button>
          <Input
            value={devToolsURL}
            onChange={(e) => setDevToolsURL(e.target.value)}
            placeholder="DevTools URL (e.g., http://localhost:9222)"
            className="w-80"
            disabled={isConnected}
          />
          <Button 
            onClick={handleConnect}
            variant={isConnected ? "destructive" : "default"}
            disabled={isLoading}
          >
            {isConnected ? <Link2Off className="w-4 h-4 mr-2" /> : <Link2 className="w-4 h-4 mr-2" />}
            {isLoading ? '连接中...' : isConnected ? '断开' : '连接'}
          </Button>
        </div>
        
        <div className="flex items-center gap-2">
          <Button 
            variant="outline" 
            size="icon"
            onClick={() => refreshTargets()}
            disabled={!isConnected}
            title="刷新目标列表"
          >
            <RefreshCw className="w-4 h-4" />
          </Button>
          {/* 状态指示器 */}
          <div className="flex items-center gap-2 text-sm">
            <span className={`flex items-center gap-1 ${isConnected ? 'text-green-500' : 'text-muted-foreground'}`}>
              <span className={`w-2 h-2 rounded-full ${isConnected ? 'bg-green-500' : 'bg-muted-foreground'}`} />
              {isConnected ? '已连接' : '未连接'}
            </span>
            {isConnected && (
              <span className="text-muted-foreground">
                · 目标 {attachedTargets.size}/{targets.length}
              </span>
            )}
          </div>
          <Button variant="ghost" size="icon" onClick={toggleTheme}>
            {isDark ? <Sun className="w-4 h-4" /> : <Moon className="w-4 h-4" />}
          </Button>
        </div>
      </div>

      {/* 主内容区 */}
      <div className="flex-1 flex flex-col overflow-hidden min-h-0">
        <Tabs defaultValue="targets" className="flex-1 flex flex-col min-h-0">
          <div className="border-b px-4">
            <TabsList className="h-10">
              <TabsTrigger value="targets" className="gap-2">
                <Link2 className="w-4 h-4" />
                Targets
              </TabsTrigger>
              <TabsTrigger value="rules" className="gap-2">
                <FileJson className="w-4 h-4" />
                Rules
              </TabsTrigger>
              <TabsTrigger value="events" className="gap-2">
                <Activity className="w-4 h-4" />
                Events
              </TabsTrigger>
            </TabsList>
          </div>

          <TabsContent value="targets" className="flex-1 overflow-hidden m-0 min-h-0 data-[state=active]:flex data-[state=active]:flex-col">
            <div className="h-full overflow-auto p-4">
              <TargetsPanel 
                targets={targets}
                attachedTargets={attachedTargets}
                onToggle={handleToggleTarget}
                isConnected={isConnected}
              />
            </div>
          </TabsContent>

          <TabsContent value="rules" className="flex-1 overflow-hidden m-0 min-h-0 data-[state=active]:flex data-[state=active]:flex-col">
            <RulesPanel 
              sessionId={currentSessionId}
              isConnected={isConnected}
              attachedTargets={attachedTargets}
              setIntercepting={setIntercepting}
            />
          </TabsContent>

          <TabsContent value="events" className="flex-1 overflow-hidden m-0 min-h-0 data-[state=active]:flex data-[state=active]:flex-col">
            <div className="h-full overflow-auto p-4">
              <EventsPanel events={events as InterceptEvent[]} onClear={clearEvents} />
            </div>
          </TabsContent>
        </Tabs>
      </div>

      {/* 底部状态栏 */}
      <div className="h-6 border-t px-4 flex items-center text-xs text-muted-foreground shrink-0">
        <span>cdpnetool v1.0.0</span>
        <span className="mx-2">|</span>
        <span>Session: {currentSessionId?.slice(0, 8) || '-'}</span>
      </div>
      
      {/* Toast 通知 */}
      <Toaster />
    </div>
  )
}

// Targets 面板组件
function TargetsPanel({ 
  targets, 
  attachedTargets, 
  onToggle,
  isConnected 
}: { 
  targets: any[]
  attachedTargets: Set<string>
  onToggle: (id: string) => void
  isConnected: boolean
}) {
  if (!isConnected) {
    return (
      <div className="flex items-center justify-center h-full text-muted-foreground">
        请先连接到浏览器
      </div>
    )
  }

  if (targets.length === 0) {
    return (
      <div className="flex items-center justify-center h-full text-muted-foreground">
        没有找到页面目标，点击刷新按钮重试
      </div>
    )
  }

  return (
    <div className="space-y-2">
      {targets.map((target) => (
        <div 
          key={target.id}
          className="flex items-center gap-3 p-3 rounded-lg border hover:bg-muted/50 transition-colors"
        >
          <div className="flex-1 min-w-0">
            <div className="font-medium truncate">{target.title || '(无标题)'}</div>
            <div className="text-sm text-muted-foreground truncate">{target.url}</div>
          </div>
          <Button
            variant={attachedTargets.has(target.id) ? "default" : "outline"}
            size="sm"
            onClick={() => onToggle(target.id)}
          >
            {attachedTargets.has(target.id) ? '已附加' : '附加'}
          </Button>
        </div>
      ))}
    </div>
  )
}

// Rules 面板组件（可视化编辑器 + 配置管理）
interface RulesPanelProps {
  sessionId: string | null
  isConnected: boolean
  attachedTargets: Set<string>
  setIntercepting: (intercepting: boolean) => void
}

function RulesPanel({ sessionId, isConnected, attachedTargets, setIntercepting }: RulesPanelProps) {
  const { toast } = useToast()
  const [ruleSet, setRuleSet] = useState<Config>(createEmptyConfig())
  const [showJson, setShowJson] = useState(false)
  const fileInputRef = useRef<HTMLInputElement>(null)
  
  // 配置管理状态
  const [ruleSets, setRuleSets] = useState<ConfigRecord[]>([])
  const [currentRuleSetId, setCurrentRuleSetId] = useState<number>(0)
  const [currentRuleSetName, setCurrentRuleSetName] = useState<string>('默认配置')
  const [currentDescription, setCurrentDescription] = useState<string>('')
  const [activeConfigId, setActiveConfigId] = useState<number | null>(null)
  const [isLoading, setIsLoading] = useState(false)
  const [editingName, setEditingName] = useState<number | null>(null)
  const [newName, setNewName] = useState('')
  const [isInitializing, setIsInitializing] = useState(true)
  const [isDirty, setIsDirty] = useState(false)
  const [configInfoExpanded, setConfigInfoExpanded] = useState(false) // 配置信息栏展开状态
  const [jsonEditorContent, setJsonEditorContent] = useState('') // JSON 编辑器内容
  const [jsonError, setJsonError] = useState<string | null>(null) // JSON 解析错误
  // 确认对话框（支持三选项：取消/不保存/保存）
  const [confirmDialog, setConfirmDialog] = useState<{
    show: boolean
    title: string
    message: string
    onConfirm: () => void           // 主要操作（如“确定”或“不保存”）
    onSave?: () => Promise<void>    // 可选的保存操作
    confirmText?: string            // 确认按钮文字
    showSaveOption?: boolean        // 是否显示保存选项
  } | null>(null)

  // 组件挂载时加载配置列表
  useEffect(() => {
    loadRuleSets()
      .catch(e => {
        console.error('Failed to load rule sets on mount:', e)
        setRuleSet(createEmptyConfig())
      })
      .finally(() => {
        setIsInitializing(false)
      })
  }, [])

  // 加载配置列表
  const loadRuleSets = async () => {
    try {
      if (!window.go?.gui?.App?.ListConfigs) {
        console.warn('Wails bindings not ready yet')
        return
      }
      
      const result = await window.go.gui.App.ListConfigs()
      if (result?.success) {
        setRuleSets(result.configs || [])
        // 加载第一个配置到编辑器，但不自动设置为激活状态
        // 用户需要手动启用配置
        if (result.configs && result.configs.length > 0) {
          loadRuleSetData(result.configs[0])
        } else {
          setRuleSet(createEmptyConfig())
        }
      }
    } catch (e) {
      console.error('Load rule sets error:', e)
      setRuleSet(createEmptyConfig())
    }
  }

  // 更新 Dirty 状态并通知后端
  const updateDirty = (dirty: boolean) => {
    setIsDirty(dirty)
    window.go?.gui?.App?.SetDirty(dirty)
  }

  // 处理规则变更
  const handleRulesChange = (rules: Rule[]) => {
    const newConfig = { ...ruleSet, rules }
    setRuleSet(newConfig)
    setJsonEditorContent(JSON.stringify(newConfig, null, 2))  // 同步 JSON 编辑器
    setJsonError(null)
    updateDirty(true)
  }

  // 快捷键支持
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key === 's') {
        e.preventDefault()
        handleSave()
      }
    }
    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [ruleSet, currentRuleSetId, currentRuleSetName, sessionId, isLoading])

  // 加载配置数据到编辑器
  const loadRuleSetData = (record: ConfigRecord) => {
    try {
      let config: Config
      
      if (!record.rulesJson) {
        // 空配置，创建完整结构（保留 UUID 如果有的话）
        config = {
          version: record.version || '1.0',
          description: record.description || '',
          settings: {},
          rules: []
        }
      } else {
        const parsed = JSON.parse(record.rulesJson)
        // 兼容两种格式：数组或完整对象
        if (Array.isArray(parsed)) {
          // 旧格式（纯规则数组），包装成完整结构
          config = {
            version: record.version || '1.0',
            description: record.description || '',
            settings: {},
            rules: parsed
          }
        } else if (parsed.rules && Array.isArray(parsed.rules)) {
          // 完整格式，保留所有字段（包括原始 UUID）
          config = {
            id: parsed.id,  // 保留配置 JSON 中的原始 ID（UUID）
            name: parsed.name,
            version: parsed.version || '1.0',
            description: parsed.description ?? record.description ?? '',
            settings: parsed.settings || {},
            rules: parsed.rules
          }
        } else {
          console.error('Invalid rules format:', parsed)
          config = {
            version: '1.0',
            description: '',
            settings: {},
            rules: []
          }
        }
      }
      
      setRuleSet(config)
      setCurrentRuleSetId(record.id)  // 数据库 ID 用于数据库操作
      setCurrentRuleSetName(record.name)
      setCurrentDescription(record.description || '')
      setJsonEditorContent(JSON.stringify(config, null, 2))  // 同步 JSON 编辑器
      setJsonError(null)
      updateDirty(false)
    } catch (e) {
      console.error('Parse rules error:', e)
      const emptyConfig = {
        version: '1.0',
        description: '',
        settings: {},
        rules: []
      }
      setRuleSet(emptyConfig)
      setJsonEditorContent(JSON.stringify(emptyConfig, null, 2))
      setJsonError(null)
      updateDirty(false)
    }
  }

  // 选择配置
  const handleSelectRuleSet = async (record: ConfigRecord) => {
    if (isDirty) {
      setConfirmDialog({
        show: true,
        title: '未保存的更改',
        message: '当前配置有未保存的更改，切换配置将丢失这些更改。',
        confirmText: '不保存',
        showSaveOption: true,
        onConfirm: () => {
          loadRuleSetData(record)
          window.go?.gui?.App?.SetActiveConfig(record.id)
          toast({ variant: 'success', title: `已切换到配置: ${record.name}` })
          setConfirmDialog(null)
        },
        onSave: async () => {
          await handleSave()
          loadRuleSetData(record)
          await window.go?.gui?.App?.SetActiveConfig(record.id)
          toast({ variant: 'success', title: `已切换到配置: ${record.name}` })
          setConfirmDialog(null)
        }
      })
      return
    }
    loadRuleSetData(record)
    await window.go?.gui?.App?.SetActiveConfig(record.id)
    toast({ variant: 'success', title: `已切换到配置: ${record.name}` })
  }

  // 创建新配置（调用后端生成标准配置）
  const handleCreateRuleSet = async () => {
    try {
      const result = await window.go?.gui?.App?.CreateNewConfig('新配置')
      if (result?.success && result.config && result.configJson) {
        await loadRuleSets()
        // 直接使用后端返回的完整 JSON，而不是数据库记录
        const newConfig = JSON.parse(result.configJson) as Config
        setRuleSet(newConfig)
        setCurrentRuleSetId(result.config.id)
        setCurrentRuleSetName(result.config.name)
        setCurrentDescription(result.config.description || '')
        setJsonEditorContent(result.configJson)  // 同步 JSON 编辑器
        setJsonError(null)
        await window.go?.gui?.App?.SetActiveConfig(result.config.id)
        updateDirty(false)
        toast({ variant: 'success', title: '新配置已创建' })
      }
    } catch (e) {
      toast({ variant: 'destructive', title: '创建失败', description: String(e) })
    }
  }

  // 删除当前配置
  const handleDeleteCurrentConfig = async () => {
    setConfirmDialog({
      show: true,
      title: '删除配置',
      message: `确定要删除配置「${currentRuleSetName}」吗？此操作不可撤销。`,
      onConfirm: async () => {
        await handleDeleteConfig(currentRuleSetId)
        setConfirmDialog(null)
      }
    })
  }

  // 删除配置
  const handleDeleteConfig = async (id: number) => {
    try {
      const result = await window.go?.gui?.App?.DeleteConfig(id)
      if (result?.success) {
        await loadRuleSets()
        // 如果删除的是当前配置，切换到第一个或重置为空状态
        if (id === currentRuleSetId) {
          const remaining = ruleSets.filter(r => r.id !== id)
          if (remaining.length > 0) {
            loadRuleSetData(remaining[0])
            await window.go?.gui?.App?.SetActiveConfig(remaining[0].id)
          } else {
            // 配置列表已空，重置为空状态
            setRuleSet(createEmptyConfig())
            setCurrentRuleSetId(0)
            setCurrentRuleSetName('')
            setCurrentDescription('')
            setActiveConfigId(null)
            updateDirty(false)
          }
        }
        toast({ variant: 'success', title: '配置已删除' })
      }
    } catch (e) {
      toast({ variant: 'destructive', title: '删除失败', description: String(e) })
    }
  }

  // 重命名配置
  const handleRenameConfig = async (id: number) => {
    if (!newName.trim()) return
    try {
      const result = await window.go?.gui?.App?.RenameConfig(id, newName.trim())
      if (result?.success) {
        await loadRuleSets()
        if (id === currentRuleSetId) {
          setCurrentRuleSetName(newName.trim())
        }
        setEditingName(null)
        setNewName('')
        toast({ variant: 'success', title: '已重命名' })
      }
    } catch (e) {
      toast({ variant: 'destructive', title: '重命名失败', description: String(e) })
    }
  }

  // 启用/禁用配置
  const handleToggleConfig = async (config: ConfigRecord, enabled: boolean) => {
    if (enabled) {
      // 启用配置
      if (!isConnected) {
        toast({ variant: 'destructive', title: '请先连接到浏览器' })
        return
      }
      if (attachedTargets.size === 0) {
        toast({ variant: 'destructive', title: '请先在 Targets 标签页附加至少一个目标' })
        return
      }
      
      try {
        // 加载规则到会话 - 需要确保是完整的 Config 格式
        let configJson: string
        if (config.rulesJson) {
          const parsed = JSON.parse(config.rulesJson)
          if (Array.isArray(parsed)) {
            // 如果是数组格式，包装成完整的 Config
            configJson = JSON.stringify({ version: config.version || '1.0', rules: parsed })
          } else if (parsed.version && parsed.rules) {
            // 已经是完整格式
            configJson = config.rulesJson
          } else {
            configJson = JSON.stringify({ version: '1.0', rules: [] })
          }
        } else {
          configJson = JSON.stringify({ version: '1.0', rules: [] })
        }
        const loadResult = await window.go?.gui?.App?.LoadRules(sessionId!, configJson)
        if (!loadResult?.success) {
          toast({ variant: 'destructive', title: '加载规则失败', description: loadResult?.error })
          return
        }
        
        // 启用拦截
        const enableResult = await window.go?.gui?.App?.EnableInterception(sessionId!)
        if (!enableResult?.success) {
          toast({ variant: 'destructive', title: '启用拦截失败', description: enableResult?.error })
          return
        }
        
        // 设置为激活状态
        await window.go?.gui?.App?.SetActiveConfig(config.id)
        setActiveConfigId(config.id)
        setIntercepting(true)
        await loadRuleSets()
        
        toast({ variant: 'success', title: `配置「${config.name}」已启用` })
      } catch (e) {
        toast({ variant: 'destructive', title: '启用失败', description: String(e) })
      }
    } else {
      // 禁用配置
      try {
        if (sessionId) {
          await window.go?.gui?.App?.DisableInterception(sessionId)
        }
        setActiveConfigId(null)
        setIntercepting(false)
        toast({ variant: 'success', title: '拦截已停止' })
      } catch (e) {
        toast({ variant: 'destructive', title: '停止失败', description: String(e) })
      }
    }
  }

  // 计算配置中的规则数量
  const getRuleCount = (config: ConfigRecord) => {
    try {
      const parsed = JSON.parse(config.rulesJson || '[]')
      return Array.isArray(parsed) ? parsed.length : (parsed.rules?.length || 0)
    } catch {
      return 0
    }
  }

  // 右键菜单 - 已移除

  // 添加新规则（调用后端生成 UUID）
  const handleAddRule = async () => {
    try {
      const result = await window.go?.gui?.App?.GenerateNewRule('新规则')
      if (result?.success) {
        const newRule = JSON.parse(result.ruleJson) as Rule
        setRuleSet({
          ...ruleSet,
          rules: [...ruleSet.rules, newRule]
        })
        updateDirty(true)
      }
    } catch (e) {
      // 回退到前端生成
      const fallbackRule: Rule = {
        id: crypto.randomUUID(),
        name: '新规则',
        enabled: true,
        priority: 0,
        stage: 'request',
        match: {},
        actions: []
      }
      setRuleSet({
        ...ruleSet,
        rules: [...ruleSet.rules, fallbackRule]
      })
      updateDirty(true)
    }
  }

  // 保存配置
  const handleSave = async () => {
    // 如果在 JSON 模式且有解析错误，阻止保存
    if (showJson && jsonError) {
      toast({ variant: 'destructive', title: '无法保存', description: 'JSON 格式错误，请修正后再保存' })
      return
    }
    
    setIsLoading(true)
    try {
      const rulesJson = JSON.stringify(ruleSet)
      
      const saveResult = await window.go?.gui?.App?.SaveConfig(
        currentRuleSetId,
        currentRuleSetName,
        currentDescription,
        rulesJson
      )
      
      if (!saveResult?.success) {
        toast({ variant: 'destructive', title: '保存失败', description: saveResult?.error })
        return
      }
      
      if (saveResult.config) {
        setCurrentRuleSetId(saveResult.config.id)
      }
      
      updateDirty(false)
      await loadRuleSets()
      
      // 如果当前配置是激活状态，重新加载规则到会话
      if (currentRuleSetId === activeConfigId && sessionId) {
        await window.go?.gui?.App?.LoadRules(sessionId, rulesJson)
        toast({ variant: 'success', title: `已保存并更新 ${ruleSet.rules.length} 条规则` })
      } else {
        toast({ variant: 'success', title: `已保存 ${ruleSet.rules.length} 条规则` })
      }
    } catch (e) {
      toast({ variant: 'destructive', title: '保存失败', description: String(e) })
    } finally {
      setIsLoading(false)
    }
  }

  // 导出 JSON (原生对话框)
  const handleExport = async () => {
    const json = JSON.stringify(ruleSet, null, 2)
    const result = await window.go?.gui?.App?.ExportConfig(currentRuleSetName || "ruleset", json)
    if (result && !result.success) {
      toast({ variant: 'destructive', title: '导出失败', description: result.error })
    } else if (result && result.success) {
      toast({ variant: 'success', title: '配置导出成功' })
    }
  }

  // 导入 JSON
  const handleImport = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (!file) return
    
    const reader = new FileReader()
    reader.onload = (event) => {
      try {
        const json = event.target?.result as string
        const imported = JSON.parse(json) as Config
        if (imported.version && Array.isArray(imported.rules)) {
          setRuleSet(imported)
          updateDirty(true)
          toast({ variant: 'success', title: `导入成功，共 ${imported.rules.length} 条规则（请点保存以持久化）` })
        } else {
          toast({ variant: 'destructive', title: 'JSON 格式不正确' })
        }
      } catch {
        toast({ variant: 'destructive', title: 'JSON 解析失败' })
      }
    }
    reader.readAsText(file)
    e.target.value = ''
  }

  return (
    <div className="flex-1 flex min-h-0">
      {/* 初始化加载状态 */}
      {isInitializing ? (
        <div className="flex items-center justify-center w-full text-muted-foreground">
          <div className="text-center">
            <div className="text-lg mb-2">加载中...</div>
            <div className="text-sm">正在初始化配置编辑器</div>
          </div>
        </div>
      ) : (
        <>
          {/* 左侧配置列表 */}
          <div className="w-60 border-r flex flex-col shrink-0">
            <div className="p-3 border-b flex items-center justify-between">
              <span className="font-medium text-sm">配置列表</span>
              <Button size="sm" variant="ghost" onClick={handleCreateRuleSet} title="新建配置">
                <Plus className="w-4 h-4" />
              </Button>
            </div>
            <ScrollArea className="flex-1">
              <div className="p-2 space-y-1">
                {ruleSets.map((config) => (
                  <div
                    key={config.id}
                    className={`flex items-center gap-2 p-2 rounded-md cursor-pointer transition-colors ${
                      config.id === currentRuleSetId 
                        ? 'bg-primary/10 border border-primary/30' 
                        : 'hover:bg-muted'
                    }`}
                    onClick={() => handleSelectRuleSet(config)}
                  >
                    <Switch
                      checked={config.id === activeConfigId}
                      onCheckedChange={(checked) => handleToggleConfig(config, checked)}
                      disabled={!isConnected && config.id !== activeConfigId}
                    />
                    <div className="flex-1 min-w-0">
                      {editingName === config.id ? (
                        <Input
                          value={newName}
                          onChange={(e) => setNewName(e.target.value)}
                          className="h-6 text-sm"
                          autoFocus
                          onClick={(e) => e.stopPropagation()}
                          onKeyDown={(e) => {
                            if (e.key === 'Enter') handleRenameConfig(config.id)
                            if (e.key === 'Escape') { setEditingName(null); setNewName('') }
                          }}
                          onBlur={() => { setEditingName(null); setNewName('') }}
                        />
                      ) : (
                        <>
                          <div className="text-sm font-medium truncate">{config.name}</div>
                          <div className="text-xs text-muted-foreground">
                            {getRuleCount(config)} 条规则
                            {config.id === activeConfigId && <span className="ml-1 text-green-500">· 运行中</span>}
                          </div>
                        </>
                      )}
                    </div>
                  </div>
                ))}
              </div>
            </ScrollArea>
          </div>

          {/* 右侧配置详情 */}
          <div className="flex-1 flex flex-col min-h-0 p-4">
            {/* 空状态 */}
            {ruleSets.length === 0 ? (
              <div className="flex-1 flex items-center justify-center text-muted-foreground">
                <div className="text-center">
                  <div className="text-lg mb-2">暂无配置</div>
                  <div className="text-sm mb-4">点击左侧「+」按钮创建第一个配置</div>
                </div>
              </div>
            ) : (
              <>
            {/* 配置信息栏（可折叠） */}
            <div className="mb-4 pb-3 border-b shrink-0">
              {/* 折叠头部 */}
              <div className="flex items-center gap-2">
                <button
                  onClick={() => setConfigInfoExpanded(!configInfoExpanded)}
                  className="flex items-center gap-1 text-sm font-medium hover:text-primary transition-colors"
                >
                  {configInfoExpanded ? <ChevronDown className="w-4 h-4" /> : <ChevronRight className="w-4 h-4" />}
                  <span className="truncate max-w-48">{currentRuleSetName}</span>
                </button>
                {isDirty && <span className="w-2 h-2 rounded-full bg-primary animate-pulse" title="有未保存更改" />}
                <div className="flex-1" />
                <input
                  ref={fileInputRef}
                  type="file"
                  accept=".json"
                  onChange={handleImport}
                  className="hidden"
                />
                <Button variant="outline" size="sm" onClick={() => fileInputRef.current?.click()}>
                  <Upload className="w-4 h-4 mr-1" />
                  导入
                </Button>
                <Button variant="outline" size="sm" onClick={handleExport}>
                  <Download className="w-4 h-4 mr-1" />
                  导出
                </Button>
                <Button size="sm" onClick={handleSave} disabled={isLoading}>
                  <Save className="w-4 h-4 mr-1" />
                  {isLoading ? '保存中...' : '保存'}
                </Button>
                <Button variant="destructive" size="sm" onClick={handleDeleteCurrentConfig}>
                  <Trash2 className="w-4 h-4 mr-1" />
                  删除
                </Button>
              </div>
              
              {/* 展开内容 */}
              {configInfoExpanded && (
                <div className="mt-3 space-y-3 pl-5">
                  <div className="flex items-center gap-2">
                    <span className="text-sm text-muted-foreground whitespace-nowrap w-16">名称:</span>
                    <Input
                      value={currentRuleSetName}
                      onChange={(e) => {
                        setCurrentRuleSetName(e.target.value)
                        updateDirty(true)
                      }}
                      className="flex-1 h-8 max-w-xs"
                    />
                  </div>
                  <div className="flex items-start gap-2">
                    <span className="text-sm text-muted-foreground whitespace-nowrap w-16 pt-2">描述:</span>
                    <Textarea
                      value={currentDescription}
                      onChange={(e) => {
                        setCurrentDescription(e.target.value)
                        updateDirty(true)
                      }}
                      placeholder="配置描述（可选）"
                      className="flex-1 min-h-[60px] max-w-md"
                    />
                  </div>
                </div>
              )}
            </div>

            {/* 规则工具栏 */}
            <div className="flex items-center gap-2 mb-4 shrink-0">
              <Button onClick={handleAddRule} size="sm">
                <Plus className="w-4 h-4 mr-1" />
                添加规则
              </Button>
              <Button variant="outline" size="sm" onClick={() => {
                if (!showJson) {
                  // 切换到 JSON 模式时，同步最新的 ruleSet
                  setJsonEditorContent(JSON.stringify(ruleSet, null, 2))
                  setJsonError(null)
                }
                setShowJson(!showJson)
              }}>
                <FileJson className="w-4 h-4 mr-1" />
                {showJson ? '可视化' : 'JSON'}
              </Button>
              <div className="flex-1" />
              <span className="text-xs text-muted-foreground">
                共 {ruleSet.rules.length} 条规则
              </span>
            </div>

            {/* 规则编辑区 */}
            <div className="flex-1 min-h-0 overflow-auto flex flex-col">
              {showJson ? (
                <div className="flex-1 flex flex-col min-h-0">
                  <textarea
                    value={jsonEditorContent}
                    onChange={(e) => {
                      setJsonEditorContent(e.target.value)
                      // 尝试解析，更新错误状态
                      try {
                        const parsed = JSON.parse(e.target.value)
                        if (parsed.rules && Array.isArray(parsed.rules)) {
                          setRuleSet(parsed)
                          setJsonError(null)
                        } else {
                          setJsonError('配置格式错误：缺少 rules 数组')
                        }
                      } catch (err) {
                        setJsonError(`JSON 解析错误：${err instanceof Error ? err.message : String(err)}`)
                      }
                      updateDirty(true)
                    }}
                    className={`flex-1 w-full p-3 rounded-md border bg-background font-mono text-sm resize-none focus:outline-none focus:ring-2 focus:ring-ring ${
                      jsonError ? 'border-destructive' : ''
                    }`}
                    spellCheck={false}
                  />
                  {jsonError && (
                    <div className="mt-2 p-2 text-sm text-destructive bg-destructive/10 rounded-md">
                      {jsonError}
                    </div>
                  )}
                </div>
              ) : (
                <RuleListEditor
                  rules={ruleSet.rules}
                  onChange={handleRulesChange}
                />
              )}
            </div>
              </>
            )}
          </div>
        </>
      )}

      {/* 自定义确认对话框 */}
      {confirmDialog?.show && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
          <div className="bg-background border rounded-lg shadow-lg p-6 max-w-md w-full mx-4">
            <h3 className="text-lg font-semibold mb-2">{confirmDialog.title}</h3>
            <p className="text-muted-foreground mb-6">{confirmDialog.message}</p>
            <div className="flex justify-end gap-2">
              <Button variant="outline" onClick={() => setConfirmDialog(null)}>
                取消
              </Button>
              {confirmDialog.showSaveOption && confirmDialog.onSave && (
                <Button variant="default" onClick={confirmDialog.onSave}>
                  保存
                </Button>
              )}
              <Button variant="destructive" onClick={confirmDialog.onConfirm}>
                {confirmDialog.confirmText || '确定'}
              </Button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

export default App
