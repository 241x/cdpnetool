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
import type { NetworkEvent } from '@/types/events'
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

import type { ApiResponse, SessionData, TargetListData, BrowserData, EmptyData } from '@/types/api'

// 配置记录类型
interface ConfigRecord {
  id: number           // 数据库主键ID
  configId: string     // 配置业务ID
  name: string
  version: string
  configJson: string   // 完整配置 JSON
  isActive: boolean
  createdAt: string
  updatedAt: string
}

// Wails 生成的绑定
declare global {
  interface Window {
    go: {
      gui: {
        App: {
          StartSession: (url: string) => Promise<ApiResponse<SessionData>>
          StopSession: (id: string) => Promise<ApiResponse<EmptyData>>
          ListTargets: (id: string) => Promise<ApiResponse<TargetListData>>
          AttachTarget: (sid: string, tid: string) => Promise<ApiResponse<EmptyData>>
          DetachTarget: (sid: string, tid: string) => Promise<ApiResponse<EmptyData>>
          EnableInterception: (id: string) => Promise<ApiResponse<EmptyData>>
          DisableInterception: (id: string) => Promise<ApiResponse<EmptyData>>
          LoadRules: (id: string, json: string) => Promise<ApiResponse<EmptyData>>
          GetRuleStats: (id: string) => Promise<ApiResponse<{ stats: any }>>
          ApproveRequest: (itemId: string, mutationsJson: string) => Promise<ApiResponse<EmptyData>>
          ApproveResponse: (itemId: string, mutationsJson: string) => Promise<ApiResponse<EmptyData>>
          Reject: (itemId: string) => Promise<ApiResponse<EmptyData>>
          LaunchBrowser: (headless: boolean) => Promise<ApiResponse<BrowserData>>
          CloseBrowser: () => Promise<ApiResponse<EmptyData>>
          GetBrowserStatus: () => Promise<ApiResponse<BrowserData>>
          ListConfigs: () => Promise<ApiResponse<{ configs: ConfigRecord[] }>>
          GetConfig: (id: number) => Promise<ApiResponse<{ config: ConfigRecord }>>
          SaveConfig: (id: number, configJson: string) => Promise<ApiResponse<{ config: ConfigRecord }>>
          DeleteConfig: (id: number) => Promise<ApiResponse<EmptyData>>
          SetActiveConfig: (id: number) => Promise<ApiResponse<EmptyData>>
          GetActiveConfig: (id: number) => Promise<ApiResponse<{ config: ConfigRecord | null }>>
          RenameConfig: (id: number, newName: string) => Promise<ApiResponse<EmptyData>>
          SetDirty: (dirty: boolean) => Promise<void>
          ExportConfig: (name: string, json: string) => Promise<ApiResponse<EmptyData>>
          ImportConfig: (json: string) => Promise<ApiResponse<{ config: ConfigRecord }>>
          CreateNewConfig: (name: string) => Promise<ApiResponse<{ config: ConfigRecord; configJson: string }>>
          GenerateNewRule: (name: string, existingCount: number) => Promise<ApiResponse<{ ruleJson: string }>>
          GetVersion: () => Promise<ApiResponse<{ version: string }>>
        }
      }
    }
  }
}

function App() {
  const { 
    devToolsURL, 
    setDevToolsURL, 
    currentSessionId: sessionId, 
    setCurrentSession,
    isConnected,
    setConnected,
    isIntercepting,
    setIntercepting,
    setActiveConfigId,
    targets,
    setTargets,
    attachedTargetId,
    setAttachedTargetId,
    matchedEvents,
    addInterceptEvent,
    clearMatchedEvents,
    resetSession,
  } = useSessionStore()
  
  const { isDark, toggle: toggleTheme } = useThemeStore()
  const { toast } = useToast()
  const [isLoading, setIsLoading] = useState(false)
  const [isLaunchingBrowser, setIsLaunchingBrowser] = useState(false)
  const [appVersion, setAppVersion] = useState('')

  // 获取版本号
  useEffect(() => {
    const fetchVersion = async () => {
      try {
        const result = await window.go?.gui?.App?.GetVersion()
        if (result?.success && result.data) {
          setAppVersion(result.data.version)
        }
      } catch (e) {
        console.error('获取版本号失败:', e)
      }
    }
    fetchVersion()
  }, [])

  // 启动浏览器
  const handleLaunchBrowser = async () => {
    setIsLaunchingBrowser(true)
    try {
      const result = await window.go?.gui?.App?.LaunchBrowser(false)
      if (result?.success && result.data) {
        setDevToolsURL(result.data.devToolsUrl)
        toast({
          variant: 'success',
          title: '浏览器已启动',
          description: `DevTools URL: ${result.data.devToolsUrl}`,
        })
      } else {
        toast({
          variant: 'destructive',
          title: '启动失败',
          description: result?.message || '无法启动浏览器',
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
    if (isConnected && sessionId) {
      // 断开
      try {
        const result = await window.go?.gui?.App?.StopSession(sessionId)
        if (result?.success) {
          setConnected(false)
          setCurrentSession(null)
          resetSession() // 一键重置拦截状态、激活配置 ID 和附加目标
          toast({
            variant: 'success',
            title: '已断开连接',
          })
        } else {
          toast({
            variant: 'destructive',
            title: '断开失败',
            description: result?.message,
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
        if (result?.success && result.data) {
          setCurrentSession(result.data.sessionId)
          setConnected(true)
          toast({
            variant: 'success',
            title: '连接成功',
            description: `会话 ID: ${result.data.sessionId.slice(0, 8)}...`,
          })
          // 自动获取目标列表
          await refreshTargets(result.data.sessionId)
        } else {
          toast({
            variant: 'destructive',
            title: '连接失败',
            description: result?.message || '连接失败',
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
  const refreshTargets = async (sessionID?: string) => {
    const sid = sessionID || sessionId
    if (!sid) return
    
    try {
      const result = await window.go?.gui?.App?.ListTargets(sid)
      if (result?.success && result.data) {
        setTargets(result.data.targets || [])
      }
    } catch (e) {
      console.error('List targets error:', e)
    }
  }

  // 附加/移除目标
  const handleToggleTarget = async (targetId: string) => {
    if (!sessionId) return
    
    const isCurrentlyAttached = attachedTargetId === targetId
    
    try {
      // 1. 如果正在拦截，切换任何目标状态前必须先停止拦截（保证前后端一致）
      if (isIntercepting) {
        await window.go?.gui?.App?.DisableInterception(sessionId)
        setIntercepting(false)
        setActiveConfigId(null)
        toast({ title: '配置已禁用', description: '修改附着目标时，拦截已自动停止' })
      }

      // 2. 如果点击的是当前已附着的目标 -> 执行“分离”
      if (isCurrentlyAttached) {
        const result = await window.go?.gui?.App?.DetachTarget(sessionId, targetId)
        if (result?.success) {
          setAttachedTargetId(null)
          toast({ variant: 'success', title: '已移除目标' })
        } else {
          toast({ variant: 'destructive', title: '移除失败', description: result?.message })
        }
        return
      }

      // 3. 如果点击的是新目标 -> 先分离旧的（如果有），再附着新的
      if (attachedTargetId) {
        await window.go?.gui?.App?.DetachTarget(sessionId, attachedTargetId)
        // 不需要在这里 setAttachedTargetId(null)，因为后面紧接着会 set 新s
      }

      const result = await window.go?.gui?.App?.AttachTarget(sessionId, targetId)
      if (result?.success) {
        setAttachedTargetId(targetId)
        toast({ variant: 'success', title: '已切换并附加新目标' })
      } else {
        setAttachedTargetId(null) // 这种情况下旧的已断开，新的没上，干脆重置
        toast({ variant: 'destructive', title: '附加新目标失败', description: result?.message })
      }
    } catch (e) {
      toast({ variant: 'destructive', title: '操作错误', description: String(e) })
    }
  }

  // 监听 Wails 事件
  useEffect(() => {
    // @ts-ignore
    if (window.runtime?.EventsOn) {
      // @ts-ignore
      const unsubscribe = window.runtime.EventsOn('intercept-event', (event: NetworkEvent) => {
        console.log('[Events] 收到拦截事件:', event)
        // 事件由 store 处理，自动分发到匹配/未匹配列表
        addInterceptEvent(event)
      })
      console.log('[Events] 已订阅 intercept-event 事件')
      
      // 清理函数：在组件卸载或依赖变化时取消订阅
      return () => {
        console.log('[Events] 取消订阅 intercept-event 事件')
        if (unsubscribe) {
          unsubscribe()
        }
      }
    }
  }, [])

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
                · 目标 {attachedTargetId ? 1 : 0}/1
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

          {/* 目标面板 */}
          <TabsContent value="targets" className="flex-1 overflow-hidden m-0 min-h-0 data-[state=active]:flex data-[state=active]:flex-col">
            <div className="h-full overflow-auto p-4">
              <TargetsPanel 
                targets={targets}
                attachedTargetId={attachedTargetId}
                onToggle={handleToggleTarget}
                isConnected={isConnected}
              />
            </div>
          </TabsContent>

          {/* 规则面板 */}
          <TabsContent value="rules" className="flex-1 overflow-hidden m-0 min-h-0 data-[state=active]:flex data-[state=active]:flex-col">
            <RulesPanel 
              sessionId={sessionId}
              isConnected={isConnected}
              attachedTargetId={attachedTargetId}
              setIntercepting={setIntercepting}
            />
          </TabsContent>

          {/* 事件面板 */}
          <TabsContent value="events" className="flex-1 overflow-hidden m-0 min-h-0 data-[state=active]:flex data-[state=active]:flex-col">
            <div className="h-full overflow-auto p-4">
              <EventsPanel 
                matchedEvents={matchedEvents} 
                onClearMatched={clearMatchedEvents}
              />
            </div>
          </TabsContent>
        </Tabs>
      </div>

      {/* 底部状态栏 */}
      <div className="h-6 border-t px-4 flex items-center text-xs text-muted-foreground shrink-0">
        <span>cdpnetool v{appVersion}</span>
        <span className="mx-2">|</span>
        <span>Session: {sessionId?.slice(0, 8) || '-'}</span>
      </div>
      
      {/* Toast 通知 */}
      <Toaster />
    </div>
  )
}

// Targets 面板组件
function TargetsPanel({ 
  targets, 
  attachedTargetId, 
  onToggle,
  isConnected 
}: { 
  targets: any[]
  attachedTargetId: string | null
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
            variant={attachedTargetId === target.id ? "default" : "outline"}
            size="sm"
            onClick={() => onToggle(target.id)}
          >
            {attachedTargetId === target.id ? '已附加' : '附加'}
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
  attachedTargetId: string | null
  setIntercepting: (intercepting: boolean) => void
}

function RulesPanel({ sessionId, isConnected, attachedTargetId, setIntercepting }: RulesPanelProps) {
  const { toast } = useToast()
  const { activeConfigId, setActiveConfigId } = useSessionStore()
  const [ruleSet, setRuleSet] = useState<Config>(createEmptyConfig())
  const [showJson, setShowJson] = useState(false)
  const fileInputRef = useRef<HTMLInputElement>(null)
  const [ruleSets, setRuleSets] = useState<ConfigRecord[]>([])
  const [currentRuleSetId, setCurrentRuleSetId] = useState<number>(0)
  const [currentRuleSetName, setCurrentRuleSetName] = useState<string>('默认配置')
  const [isLoading, setIsLoading] = useState(false)
  const [editingName, setEditingName] = useState<number | null>(null)
  const [newName, setNewName] = useState('')
  const [isInitializing, setIsInitializing] = useState(true)
  const [isDirty, setIsDirty] = useState(false)
  const [configInfoExpanded, setConfigInfoExpanded] = useState(false) // 配置信息栏展开状态
  const [jsonEditorContent, setJsonEditorContent] = useState('') // JSON 编辑器内容
  const [jsonError, setJsonError] = useState<string | null>(null) // JSON 解析错误
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
      if (result?.success && result.data) {
        setRuleSets(result.data.configs || [])
        // 加载第一个配置到编辑器，但不自动设置为激活状态，用户需要手动启用配置
        if (result.data.configs && result.data.configs.length > 0) {
          loadRuleSetData(result.data.configs[0])
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
      
      if (!record.configJson) {
        // 空配置，创建完整结构
        config = {
          id: record.configId,
          name: record.name,
          version: record.version || '1.0',
          description: '',
          settings: {},
          rules: []
        }
      } else {
        // 从 configJson 解析完整配置
        config = JSON.parse(record.configJson) as Config
      }
      
      setRuleSet(config)
      setCurrentRuleSetId(record.id)  // 数据库 ID 用于数据库操作
      setCurrentRuleSetName(config.name || record.name)
      setJsonEditorContent(JSON.stringify(config, null, 2))  // 同步 JSON 编辑器
      setJsonError(null)
      updateDirty(false)
    } catch (e) {
      console.error('Parse config error:', e)
      const emptyConfig = createEmptyConfig()
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
      if (result?.success && result.data) {
        await loadRuleSets()
        // 直接使用后端返回的完整 JSON，而不是数据库记录
        const newConfig = JSON.parse(result.data.configJson) as Config
        setRuleSet(newConfig)
        setCurrentRuleSetId(result.data.config.id)
        setCurrentRuleSetName(result.data.config.name)
        setJsonEditorContent(result.data.configJson)  // 同步 JSON 编辑器
        setJsonError(null)
        await window.go?.gui?.App?.SetActiveConfig(result.data.config.id)
        updateDirty(false)
        toast({ variant: 'success', title: '新配置已创建' })
      } else {
        toast({ variant: 'destructive', title: '创建失败', description: result?.message })
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
            setActiveConfigId(null)
            updateDirty(false)
          }
        }
        toast({ variant: 'success', title: '配置已删除' })
      } else {
        toast({ variant: 'destructive', title: '删除失败', description: result?.message })
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
      if (!attachedTargetId) {
        toast({ variant: 'destructive', title: '请先在 Targets 标签页附加一个目标' })
        return
      }
      
      try {
        // 加载规则到会话 - 直接使用 configJson
        const configJson = config.configJson || JSON.stringify({ version: '1.0', rules: [] })
        const loadResult = await window.go?.gui?.App?.LoadRules(sessionId!, configJson)
        if (!loadResult?.success) {
          toast({ variant: 'destructive', title: '加载规则失败', description: loadResult?.message })
          return
        }
        
        // 启用拦截
        const enableResult = await window.go?.gui?.App?.EnableInterception(sessionId!)
        if (!enableResult?.success) {
          toast({ variant: 'destructive', title: '启用拦截失败', description: enableResult?.message })
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
      if (!config.configJson) return 0
      const parsed = JSON.parse(config.configJson)
      return parsed.rules?.length || 0
    } catch {
      return 0
    }
  }

  // 添加新规则（调用后端生成 ID）
  const handleAddRule = async () => {
    try {
      const result = await window.go?.gui?.App?.GenerateNewRule('新规则', ruleSet.rules.length)
      if (result?.success && result.data) {
        const newRule = JSON.parse(result.data.ruleJson) as Rule
        setRuleSet({
          ...ruleSet,
          rules: [...ruleSet.rules, newRule]
        })
        updateDirty(true)
      } else {
        toast({ variant: 'destructive', title: '添加失败', description: result?.message })
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
      // 同步名称到 ruleSet
      const configToSave = {
        ...ruleSet,
        name: currentRuleSetName
      }
      const configJson = JSON.stringify(configToSave)
      
      const saveResult = await window.go?.gui?.App?.SaveConfig(
        currentRuleSetId,
        configJson
      )
      
      if (!saveResult?.success) {
        toast({ variant: 'destructive', title: '保存失败', description: saveResult?.message })
        return
      }
      
      if (saveResult.data && saveResult.data.config) {
        setCurrentRuleSetId(saveResult.data.config.id)
      }
      
      updateDirty(false)
      await loadRuleSets()
      
      // 如果当前配置是激活状态，重新加载规则到会话
      if (currentRuleSetId === activeConfigId && sessionId) {
        await window.go?.gui?.App?.LoadRules(sessionId, configJson)
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
      toast({ variant: 'destructive', title: '导出失败', description: result.message })
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
                      value={ruleSet.description || ''}
                      onChange={(e) => {
                        setRuleSet({ ...ruleSet, description: e.target.value })
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
