import { useState, useEffect, useRef } from 'react'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Toaster } from '@/components/ui/toaster'
import { useToast } from '@/hooks/use-toast'
import { useSessionStore, useThemeStore } from '@/stores'
import { RuleListEditor } from '@/components/rules'
import { PendingPanel } from '@/components/pending'
import { EventsPanel } from '@/components/events'
import type { Rule, RuleSet } from '@/types/rules'
import type { PendingItem, RewriteMutation } from '@/types/pending'
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
  Clock,
  Plus,
  Download,
  Upload,
  Save,
  Chrome
} from 'lucide-react'

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
  const [pendingItems, setPendingItems] = useState<PendingItem[]>([])

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
      // @ts-ignore
      window.runtime.EventsOn('pending-item', (item: PendingItem) => {
        setPendingItems(prev => [item, ...prev])
      })
    }
  }, [addEvent])

  // 审批 Pending 项
  const handleApprovePending = async (itemId: string, stage: 'request' | 'response', mutations?: RewriteMutation) => {
    try {
      const mutationsJson = mutations ? JSON.stringify(mutations) : ''
      const result = stage === 'request' 
        ? await window.go?.gui?.App?.ApproveRequest(itemId, mutationsJson)
        : await window.go?.gui?.App?.ApproveResponse(itemId, mutationsJson)
      
      if (result?.success) {
        setPendingItems(prev => prev.filter(item => item.id !== itemId))
        toast({
          variant: 'success',
          title: '已通过',
        })
      } else {
        toast({
          variant: 'destructive',
          title: '审批失败',
          description: result?.error,
        })
      }
    } catch (e) {
      toast({
        variant: 'destructive',
        title: '审批错误',
        description: String(e),
      })
    }
  }

  // 拒绝 Pending 项
  const handleRejectPending = async (itemId: string) => {
    try {
      const result = await window.go?.gui?.App?.Reject(itemId)
      if (result?.success) {
        setPendingItems(prev => prev.filter(item => item.id !== itemId))
        toast({
          variant: 'success',
          title: '已拒绝',
        })
      } else {
        toast({
          variant: 'destructive',
          title: '拒绝失败',
          description: result?.error,
        })
      }
    } catch (e) {
      toast({
        variant: 'destructive',
        title: '拒绝错误',
        description: String(e),
      })
    }
  }

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
      <div className="flex-1 flex overflow-hidden">
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
        <div className="flex-1 flex flex-col overflow-hidden">
          <Tabs defaultValue="targets" className="flex-1 flex flex-col">
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
                <TabsTrigger value="pending" className="gap-2">
                  <Clock className="w-4 h-4" />
                  Pending
                </TabsTrigger>
              </TabsList>
            </div>

            <TabsContent value="targets" className="flex-1 p-4 overflow-auto m-0">
              <TargetsPanel 
                targets={targets}
                attachedTargets={attachedTargets}
                onToggle={handleToggleTarget}
                isConnected={isConnected}
              />
            </TabsContent>

            <TabsContent value="rules" className="flex-1 p-4 overflow-auto m-0">
              <RulesPanel sessionId={currentSessionId} />
            </TabsContent>

            <TabsContent value="events" className="flex-1 p-4 overflow-auto m-0">
              <EventsPanel events={events as InterceptEvent[]} onClear={clearEvents} />
            </TabsContent>

            <TabsContent value="pending" className="flex-1 p-4 overflow-auto m-0">
              <PendingPanel
                items={pendingItems}
                onApprove={handleApprovePending}
                onReject={handleRejectPending}
              />
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

// Rules 面板组件（可视化编辑器）
function RulesPanel({ sessionId }: { sessionId: string | null }) {
  const [ruleSet, setRuleSet] = useState<RuleSet>(createEmptyRuleSet())
  const [status, setStatus] = useState<{ type: 'success' | 'error' | null; message: string }>({ type: null, message: '' })
  const [showJson, setShowJson] = useState(false)
  const fileInputRef = useRef<HTMLInputElement>(null)

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

  // 加载规则到后端
  const handleLoadRules = async () => {
    if (!sessionId) return
    
    try {
      const json = JSON.stringify(ruleSet, null, 2)
      const result = await window.go?.gui?.App?.LoadRules(sessionId, json)
      if (result?.success) {
        setStatus({ type: 'success', message: `成功加载 ${ruleSet.rules.length} 条规则` })
      } else {
        setStatus({ type: 'error', message: result?.error || '加载失败' })
      }
    } catch (e) {
      setStatus({ type: 'error', message: String(e) })
    }
    
    // 3秒后清除状态
    setTimeout(() => setStatus({ type: null, message: '' }), 3000)
  }

  // 导出 JSON
  const handleExport = () => {
    const json = JSON.stringify(ruleSet, null, 2)
    const blob = new Blob([json], { type: 'application/json' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = 'rules.json'
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
          setStatus({ type: 'success', message: `导入成功，共 ${imported.rules.length} 条规则` })
        } else {
          setStatus({ type: 'error', message: 'JSON 格式不正确' })
        }
      } catch {
        setStatus({ type: 'error', message: 'JSON 解析失败' })
      }
    }
    reader.readAsText(file)
    e.target.value = '' // 重置输入
  }

  return (
    <div className="h-full flex flex-col">
      {/* 工具栏 */}
      <div className="flex items-center justify-between mb-4 gap-2">
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
          <Button size="sm" onClick={handleLoadRules} disabled={!sessionId || ruleSet.rules.length === 0}>
            <Save className="w-4 h-4 mr-1" />
            应用规则
          </Button>
        </div>
      </div>

      {/* 状态提示 */}
      {status.type && (
        <div className={`p-2 rounded text-sm mb-4 ${
          status.type === 'success' ? 'bg-green-500/10 text-green-500' : 'bg-red-500/10 text-red-500'
        }`}>
          {status.message}
        </div>
      )}

      {/* 规则编辑区 */}
      <div className="flex-1 overflow-hidden">
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
      <div className="text-xs text-muted-foreground mt-2 pt-2 border-t">
        共 {ruleSet.rules.length} 条规则 · 版本 {ruleSet.version}
      </div>
    </div>
  )
}

export default App
