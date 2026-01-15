import { useState, useEffect, useRef } from 'react'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Toaster } from '@/components/ui/toaster'
import { useToast } from '@/hooks/use-toast'
import { useSessionStore, useThemeStore } from '@/stores'
import { RuleListEditor } from '@/components/rules'
import { EventsPanel } from '@/components/events'
import type { Rule, RuleSet } from '@/types/rules'
import type { InterceptEvent } from '@/types/events'
import { createEmptyRule, createEmptyRuleSet } from '@/types/rules'
import { 
  Play, 
  Square, 
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
  FolderOpen,
  Trash2,
  Copy,
  Edit3,
  Check,
  X
} from 'lucide-react'

// 规则集记录类型
interface RuleSetRecord {
  id: number
  name: string
  version: string
  rulesJson: string
  isActive: boolean
  createdAt: string
  updatedAt: string
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
          // 规则集持久化 API
          ListRuleSets: () => Promise<{ ruleSets: RuleSetRecord[]; success: boolean; error?: string }>
          GetRuleSet: (id: number) => Promise<{ ruleSet: RuleSetRecord; success: boolean; error?: string }>
          SaveRuleSet: (id: number, name: string, rulesJson: string) => Promise<{ ruleSet: RuleSetRecord; success: boolean; error?: string }>
          DeleteRuleSet: (id: number) => Promise<{ success: boolean; error?: string }>
          SetActiveRuleSet: (id: number) => Promise<{ success: boolean; error?: string }>
          GetActiveRuleSet: () => Promise<{ ruleSet: RuleSetRecord | null; success: boolean; error?: string }>
          RenameRuleSet: (id: number, newName: string) => Promise<{ success: boolean; error?: string }>
          DuplicateRuleSet: (id: number, newName: string) => Promise<{ ruleSet: RuleSetRecord; success: boolean; error?: string }>
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
    isIntercepting,
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
          toast({
            variant: 'destructive',
            title: '连接失败',
            description: result?.error,
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

  // 启用/停用拦截
  const handleToggleInterception = async () => {
    if (!currentSessionId) return
    
    try {
      if (isIntercepting) {
        const result = await window.go?.gui?.App?.DisableInterception(currentSessionId)
        if (result?.success) {
          setIntercepting(false)
          toast({
            variant: 'success',
            title: '拦截已停止',
          })
        } else {
          toast({
            variant: 'destructive',
            title: '停止失败',
            description: result?.error,
          })
        }
      } else {
        const result = await window.go?.gui?.App?.EnableInterception(currentSessionId)
        if (result?.success) {
          setIntercepting(true)
          toast({
            variant: 'success',
            title: '拦截已启用',
          })
        } else {
          toast({
            variant: 'destructive',
            title: '启用失败',
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
          >
            <RefreshCw className="w-4 h-4" />
          </Button>
          <Button 
            onClick={handleToggleInterception}
            variant={isIntercepting ? "destructive" : "secondary"}
            disabled={!isConnected}
          >
            {isIntercepting ? <Square className="w-4 h-4 mr-2" /> : <Play className="w-4 h-4 mr-2" />}
            {isIntercepting ? '停止拦截' : '启用拦截'}
          </Button>
          <Button variant="ghost" size="icon" onClick={toggleTheme}>
            {isDark ? <Sun className="w-4 h-4" /> : <Moon className="w-4 h-4" />}
          </Button>
        </div>
      </div>

      {/* 主内容区 */}
      <div className="flex-1 flex overflow-hidden min-h-0">
        {/* 左侧面板 - 状态 */}
        <div className="w-64 border-r flex flex-col shrink-0">
          <div className="p-4 border-b">
            <h2 className="font-semibold mb-2">会话状态</h2>
            <div className="space-y-1 text-sm">
              <div className="flex justify-between">
                <span className="text-muted-foreground">连接:</span>
                <span className={isConnected ? 'text-green-500' : 'text-red-500'}>
                  {isConnected ? '● 已连接' : '○ 未连接'}
                </span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">拦截:</span>
                <span className={isIntercepting ? 'text-green-500' : 'text-muted-foreground'}>
                  {isIntercepting ? '● 运行中' : '○ 已停止'}
                </span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">目标:</span>
                <span>{attachedTargets.size} / {targets.length}</span>
              </div>
            </div>
          </div>
          
          <ScrollArea className="flex-1">
            <div className="p-4">
              <h3 className="font-medium mb-2 text-sm">最近事件</h3>
              {events.length === 0 ? (
                <p className="text-sm text-muted-foreground">暂无事件</p>
              ) : (
                <div className="space-y-1">
                  {events.slice(0, 10).map((evt, i) => (
                    <div key={i} className="text-xs p-1.5 rounded bg-muted truncate">
                      <span className="text-muted-foreground">[{evt.type}]</span> {evt.target?.slice(0, 8)}
                    </div>
                  ))}
                </div>
              )}
            </div>
          </ScrollArea>
        </div>

        {/* 右侧主区域 */}
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
              <RulesPanel sessionId={currentSessionId} />
            </TabsContent>

            <TabsContent value="events" className="flex-1 overflow-hidden m-0 min-h-0 data-[state=active]:flex data-[state=active]:flex-col">
              <div className="h-full overflow-auto p-4">
                <EventsPanel events={events as InterceptEvent[]} onClear={clearEvents} />
              </div>
            </TabsContent>
          </Tabs>
        </div>
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

// Rules 面板组件（可视化编辑器 + 规则集管理）
function RulesPanel({ sessionId }: { sessionId: string | null }) {
  const [ruleSet, setRuleSet] = useState<RuleSet>(createEmptyRuleSet())
  const [status, setStatus] = useState<{ type: 'success' | 'error' | null; message: string }>({ type: null, message: '' })
  const [showJson, setShowJson] = useState(false)
  const fileInputRef = useRef<HTMLInputElement>(null)
  
  // 新增：规则集管理状态
  const [ruleSets, setRuleSets] = useState<RuleSetRecord[]>([])
  const [currentRuleSetId, setCurrentRuleSetId] = useState<number>(0)
  const [currentRuleSetName, setCurrentRuleSetName] = useState<string>('默认规则集')
  const [isLoading, setIsLoading] = useState(false)
  const [showRuleSetManager, setShowRuleSetManager] = useState(false)
  const [editingName, setEditingName] = useState<number | null>(null)
  const [newName, setNewName] = useState('')
  const [isInitializing, setIsInitializing] = useState(true) // 新增：初始化状态

  // 组件挂载时加载规则集列表和激活的规则集
  useEffect(() => {
    loadRuleSets()
      .catch(e => {
        console.error('Failed to load rule sets on mount:', e)
        // 如果加载失败，至少确保有一个空的规则集
        setRuleSet(createEmptyRuleSet())
      })
      .finally(() => {
        setIsInitializing(false)
      })
  }, [])

  // 加载规则集列表
  const loadRuleSets = async () => {
    try {
      // 检查 window.go 是否存在
      if (!window.go?.gui?.App?.ListRuleSets) {
        console.warn('Wails bindings not ready yet')
        return
      }
      
      const result = await window.go.gui.App.ListRuleSets()
      if (result?.success) {
        setRuleSets(result.ruleSets || [])
        // 查找激活的规则集
        const activeResult = await window.go.gui.App.GetActiveRuleSet()
        if (activeResult?.success && activeResult.ruleSet) {
          loadRuleSetData(activeResult.ruleSet)
        } else if (result.ruleSets && result.ruleSets.length > 0) {
          // 如果没有激活的，加载第一个
          loadRuleSetData(result.ruleSets[0])
        } else {
          // 如果没有任何规则集，创建一个默认的
          setRuleSet(createEmptyRuleSet())
        }
      }
    } catch (e) {
      console.error('Load rule sets error:', e)
      setRuleSet(createEmptyRuleSet())
    }
  }

  // 加载规则集数据到编辑器
  const loadRuleSetData = (record: RuleSetRecord) => {
    try {
      if (!record.rulesJson) {
        setRuleSet(createEmptyRuleSet())
        setCurrentRuleSetId(record.id)
        setCurrentRuleSetName(record.name)
        return
      }
      
      const parsed = JSON.parse(record.rulesJson)
      // 兼容两种格式：数组或 { version, rules } 对象
      if (Array.isArray(parsed)) {
        setRuleSet({ version: record.version || '2.0', rules: parsed })
      } else if (parsed.rules && Array.isArray(parsed.rules)) {
        setRuleSet({ version: parsed.version || '2.0', rules: parsed.rules })
      } else {
        console.error('Invalid rules format:', parsed)
        setRuleSet(createEmptyRuleSet())
      }
      
      setCurrentRuleSetId(record.id)
      setCurrentRuleSetName(record.name)
    } catch (e) {
      console.error('Parse rules error:', e)
      setRuleSet(createEmptyRuleSet())
    }
  }

  // 选择规则集
  const handleSelectRuleSet = async (record: RuleSetRecord) => {
    loadRuleSetData(record)
    // 设置为激活
    await window.go?.gui?.App?.SetActiveRuleSet(record.id)
    setShowRuleSetManager(false)
    showStatusMessage('success', `已切换到规则集: ${record.name}`)
  }

  // 创建新规则集
  const handleCreateRuleSet = async () => {
    const name = `规则集 ${new Date().toLocaleString()}`
    try {
      const emptyRuleSet = { version: '2.0', rules: [] }
      const result = await window.go?.gui?.App?.SaveRuleSet(0, name, JSON.stringify(emptyRuleSet))
      if (result?.success && result.ruleSet) {
        await loadRuleSets()
        loadRuleSetData(result.ruleSet)
        await window.go?.gui?.App?.SetActiveRuleSet(result.ruleSet.id)
        showStatusMessage('success', '新规则集已创建')
      }
    } catch (e) {
      showStatusMessage('error', '创建失败: ' + String(e))
    }
  }

  // 删除规则集
  const handleDeleteRuleSet = async (id: number) => {
    if (ruleSets.length <= 1) {
      showStatusMessage('error', '至少保留一个规则集')
      return
    }
    try {
      const result = await window.go?.gui?.App?.DeleteRuleSet(id)
      if (result?.success) {
        await loadRuleSets()
        // 如果删除的是当前规则集，切换到第一个
        if (id === currentRuleSetId) {
          const remaining = ruleSets.filter(r => r.id !== id)
          if (remaining.length > 0) {
            loadRuleSetData(remaining[0])
            await window.go?.gui?.App?.SetActiveRuleSet(remaining[0].id)
          }
        }
        showStatusMessage('success', '规则集已删除')
      }
    } catch (e) {
      showStatusMessage('error', '删除失败: ' + String(e))
    }
  }

  // 重命名规则集
  const handleRenameRuleSet = async (id: number) => {
    if (!newName.trim()) return
    try {
      const result = await window.go?.gui?.App?.RenameRuleSet(id, newName.trim())
      if (result?.success) {
        await loadRuleSets()
        if (id === currentRuleSetId) {
          setCurrentRuleSetName(newName.trim())
        }
        setEditingName(null)
        setNewName('')
        showStatusMessage('success', '已重命名')
      }
    } catch (e) {
      showStatusMessage('error', '重命名失败: ' + String(e))
    }
  }

  // 复制规则集
  const handleDuplicateRuleSet = async (id: number, originalName: string) => {
    try {
      const result = await window.go?.gui?.App?.DuplicateRuleSet(id, `${originalName} (副本)`)
      if (result?.success) {
        await loadRuleSets()
        showStatusMessage('success', '规则集已复制')
      }
    } catch (e) {
      showStatusMessage('error', '复制失败: ' + String(e))
    }
  }

  // 添加新规则
  const handleAddRule = () => {
    setRuleSet({
      ...ruleSet,
      rules: [...ruleSet.rules, createEmptyRule()]
    })
  }

  // 更新规则列表
  const handleRulesChange = (rules: Rule[]) => {
    setRuleSet({ ...ruleSet, rules })
  }

  // 显示状态消息
  const showStatusMessage = (type: 'success' | 'error', message: string) => {
    setStatus({ type, message })
    setTimeout(() => setStatus({ type: null, message: '' }), 3000)
  }

  // 保存并应用规则（持久化 + 加载到会话）
  const handleSaveAndApply = async () => {
    setIsLoading(true)
    try {
      const rulesJson = JSON.stringify(ruleSet)
      
      // 1. 保存到数据库
      const saveResult = await window.go?.gui?.App?.SaveRuleSet(
        currentRuleSetId,
        currentRuleSetName,
        rulesJson
      )
      
      if (!saveResult?.success) {
        showStatusMessage('error', saveResult?.error || '保存失败')
        return
      }
      
      // 更新当前规则集ID（新建时会返回新ID）
      if (saveResult.ruleSet) {
        setCurrentRuleSetId(saveResult.ruleSet.id)
        // 设置为激活规则集
        await window.go?.gui?.App?.SetActiveRuleSet(saveResult.ruleSet.id)
      }
      
      // 2. 如果有会话，加载到会话
      if (sessionId) {
        const loadResult = await window.go?.gui?.App?.LoadRules(sessionId, rulesJson)
        if (loadResult?.success) {
          showStatusMessage('success', `已保存并应用 ${ruleSet.rules.length} 条规则`)
        } else {
          showStatusMessage('success', `已保存 ${ruleSet.rules.length} 条规则（未连接会话）`)
        }
      } else {
        showStatusMessage('success', `已保存 ${ruleSet.rules.length} 条规则`)
      }
      
      // 刷新规则集列表
      await loadRuleSets()
    } catch (e) {
      showStatusMessage('error', String(e))
    } finally {
      setIsLoading(false)
    }
  }

  // 导出 JSON
  const handleExport = () => {
    const json = JSON.stringify(ruleSet, null, 2)
    const blob = new Blob([json], { type: 'application/json' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `${currentRuleSetName}.json`
    a.click()
    URL.revokeObjectURL(url)
  }

  // 导入 JSON
  const handleImport = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (!file) return
    
    const reader = new FileReader()
    reader.onload = (event) => {
      try {
        const json = event.target?.result as string
        const imported = JSON.parse(json) as RuleSet
        if (imported.version && Array.isArray(imported.rules)) {
          setRuleSet(imported)
          showStatusMessage('success', `导入成功，共 ${imported.rules.length} 条规则（请点保存以持久化）`)
        } else {
          showStatusMessage('error', 'JSON 格式不正确')
        }
      } catch {
        showStatusMessage('error', 'JSON 解析失败')
      }
    }
    reader.readAsText(file)
    e.target.value = ''
  }

  return (
    <div className="flex-1 flex flex-col p-4 min-h-0">
      {/* 初始化加载状态 */}
      {isInitializing ? (
        <div className="flex items-center justify-center h-full text-muted-foreground">
          <div className="text-center">
            <div className="text-lg mb-2">加载中...</div>
            <div className="text-sm">正在初始化规则编辑器</div>
          </div>
        </div>
      ) : (
        <>
      {/* 工具栏 - 第一行：规则集选择 */}
      <div className="flex items-center gap-2 mb-2 shrink-0">
        <Button
          variant="outline"
          size="sm"
          onClick={() => setShowRuleSetManager(!showRuleSetManager)}
          className="gap-1"
        >
          <FolderOpen className="w-4 h-4" />
          {currentRuleSetName || '选择规则集'}
        </Button>
        <span className="text-xs text-muted-foreground">
          {ruleSets.length} 个规则集
        </span>
      </div>

      {/* 规则集管理面板 */}
      {showRuleSetManager && (
        <div className="mb-4 p-3 border rounded-lg bg-muted/30 shrink-0">
          <div className="flex items-center justify-between mb-2">
            <span className="text-sm font-medium">规则集管理</span>
            <Button size="sm" variant="outline" onClick={handleCreateRuleSet}>
              <Plus className="w-3 h-3 mr-1" />
              新建
            </Button>
          </div>
          <ScrollArea className="h-40">
            <div className="space-y-1">
              {ruleSets.map((rs) => (
                <div
                  key={rs.id}
                  className={`flex items-center gap-2 p-2 rounded text-sm hover:bg-muted transition-colors ${
                    rs.id === currentRuleSetId ? 'bg-primary/10 border border-primary/30' : ''
                  }`}
                >
                  {editingName === rs.id ? (
                    <div className="flex-1 flex items-center gap-1">
                      <Input
                        value={newName}
                        onChange={(e) => setNewName(e.target.value)}
                        className="h-6 text-sm"
                        autoFocus
                        onKeyDown={(e) => {
                          if (e.key === 'Enter') handleRenameRuleSet(rs.id)
                          if (e.key === 'Escape') { setEditingName(null); setNewName('') }
                        }}
                      />
                      <Button size="sm" variant="ghost" className="h-6 w-6 p-0" onClick={() => handleRenameRuleSet(rs.id)}>
                        <Check className="w-3 h-3" />
                      </Button>
                      <Button size="sm" variant="ghost" className="h-6 w-6 p-0" onClick={() => { setEditingName(null); setNewName('') }}>
                        <X className="w-3 h-3" />
                      </Button>
                    </div>
                  ) : (
                    <>
                      <span
                        className="flex-1 cursor-pointer truncate"
                        onClick={() => handleSelectRuleSet(rs)}
                      >
                        {rs.name}
                        {rs.isActive && <span className="ml-1 text-xs text-primary">(激活)</span>}
                      </span>
                      <span className="text-xs text-muted-foreground">
                        {(() => {
                          try {
                            const parsed = JSON.parse(rs.rulesJson || '[]')
                            const count = Array.isArray(parsed) ? parsed.length : (parsed.rules?.length || 0)
                            return `${count} 规则`
                          } catch {
                            return '0 规则'
                          }
                        })()}
                      </span>
                      <Button
                        size="sm"
                        variant="ghost"
                        className="h-6 w-6 p-0"
                        onClick={() => { setEditingName(rs.id); setNewName(rs.name) }}
                      >
                        <Edit3 className="w-3 h-3" />
                      </Button>
                      <Button
                        size="sm"
                        variant="ghost"
                        className="h-6 w-6 p-0"
                        onClick={() => handleDuplicateRuleSet(rs.id, rs.name)}
                      >
                        <Copy className="w-3 h-3" />
                      </Button>
                      <Button
                        size="sm"
                        variant="ghost"
                        className="h-6 w-6 p-0 text-destructive hover:text-destructive"
                        onClick={() => handleDeleteRuleSet(rs.id)}
                        disabled={ruleSets.length <= 1}
                      >
                        <Trash2 className="w-3 h-3" />
                      </Button>
                    </>
                  )}
                </div>
              ))}
            </div>
          </ScrollArea>
        </div>
      )}

      {/* 工具栏 - 第二行：规则操作 */}
      <div className="flex items-center justify-between mb-4 gap-2 shrink-0">
        <div className="flex items-center gap-2">
          <Button onClick={handleAddRule} size="sm">
            <Plus className="w-4 h-4 mr-1" />
            添加规则
          </Button>
          <Button variant="outline" size="sm" onClick={() => setShowJson(!showJson)}>
            <FileJson className="w-4 h-4 mr-1" />
            {showJson ? '可视化' : 'JSON'}
          </Button>
        </div>
        
        <div className="flex items-center gap-2">
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
          <Button 
            size="sm" 
            onClick={handleSaveAndApply} 
            disabled={isLoading}
          >
            <Save className="w-4 h-4 mr-1" />
            {isLoading ? '保存中...' : '保存并应用'}
          </Button>
        </div>
      </div>

      {/* 状态提示 */}
      {status.type && (
        <div className={`p-2 rounded text-sm mb-4 shrink-0 ${
          status.type === 'success' ? 'bg-green-500/10 text-green-500' : 'bg-red-500/10 text-red-500'
        }`}>
          {status.message}
        </div>
      )}

      {/* 规则编辑区 */}
      <div className="flex-1 min-h-0 overflow-auto">
        {showJson ? (
          <textarea
            value={JSON.stringify(ruleSet, null, 2)}
            onChange={(e) => {
              try {
                setRuleSet(JSON.parse(e.target.value))
              } catch {}
            }}
            className="w-full h-full p-3 rounded-md border bg-background font-mono text-sm resize-none focus:outline-none focus:ring-2 focus:ring-ring"
          />
        ) : (
          <RuleListEditor
            rules={ruleSet.rules}
            onChange={handleRulesChange}
          />
        )}
      </div>

      {/* 规则计数 */}
      <div className="text-xs text-muted-foreground mt-2 pt-2 border-t shrink-0">
        共 {ruleSet.rules.length} 条规则 · 版本 {ruleSet.version} · 规则集: {currentRuleSetName}
      </div>
        </>
      )}
    </div>
  )
}

export default App
