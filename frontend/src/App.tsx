import { useState, useEffect } from 'react'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Toaster } from '@/components/ui/toaster'
import { useToast } from '@/hooks/use-toast'
import { useSessionStore, useThemeStore } from '@/stores'
import { EventsPanel } from '@/components/events'
import { NetworkPanel } from '@/components/network/NetworkPanel'
import { TargetsPanel } from '@/components/targets/TargetsPanel'
import { RulesPanel } from '@/components/rules/RulesPanel'
import { api } from '@/api'
import { useTranslation } from 'react-i18next'
import { 
  Moon, 
  Sun,
  Link2,
  Link2Off,
  FileJson,
  Activity,
  Chrome,
  Languages
} from 'lucide-react'

function App() {
  const { t } = useTranslation()
  const { 
    devToolsURL, 
    setDevToolsURL, 
    currentSessionId: sessionId, 
    setCurrentSession,
    isConnected,
    setConnected,
    setIntercepting,
    targets,
    attachedTargetId,
    matchedEvents,
    trafficEvents,
    isTrafficCapturing,
    setTrafficCapturing,
    addInterceptEvent,
    addTrafficEvent,
    clearMatchedEvents,
    clearTrafficEvents,
    resetSession,
    refreshTargets,
    toggleTarget,
    language,
    setLanguage
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
        const result = await api.system.getVersion()
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
      const result = await api.browser.launch(false)
      if (result?.success && result.data) {
        setDevToolsURL(result.data.devToolsUrl)
        toast({
          variant: 'success',
          title: t('toolbar.launchBrowser'),
          description: `DevTools URL: ${result.data.devToolsUrl}`,
        })
      } else {
        toast({
          variant: 'destructive',
          title: t('common.loading'),
          description: result?.message || 'Error',
        })
      }
    } catch (e) {
      toast({
        variant: 'destructive',
        title: 'Error',
        description: String(e),
      })
    } finally {
      setIsLaunchingBrowser(false)
    }
  }

  // 连接/断开会话
  const handleConnect = async () => {
    if (isConnected && sessionId) {
      try {
        const result = await api.session.stop(sessionId)
        if (result?.success) {
          setConnected(false)
          setCurrentSession(null)
          resetSession()
          toast({ variant: 'success', title: t('common.disconnect') })
        } else {
          toast({ variant: 'destructive', title: 'Error', description: result?.message })
        }
      } catch (e) {
        toast({ variant: 'destructive', title: 'Error', description: String(e) })
      }
    } else {
      setIsLoading(true)
      try {
        const result = await api.session.start(devToolsURL)
        if (result?.success && result.data) {
          setCurrentSession(result.data.sessionId)
          setConnected(true)
          toast({
            variant: 'success',
            title: t('common.connected'),
            description: `ID: ${result.data.sessionId.slice(0, 8)}...`,
          })
          await refreshTargets()
        } else {
          toast({ variant: 'destructive', title: 'Error', description: result?.message || 'Failed' })
        }
      } catch (e) {
        toast({ variant: 'destructive', title: 'Error', description: String(e) })
      } finally {
        setIsLoading(false)
      }
    }
  }

  // 切换语言
  const handleToggleLanguage = () => {
    const nextLang = language === 'zh' ? 'en' : 'zh'
    setLanguage(nextLang)
    toast({
      title: t('toolbar.toggleLanguage'),
      description: nextLang === 'zh' ? '中文' : 'English',
    })
  }

  // 切换目标处理
  const handleToggleTarget = async (targetId: string) => {
    const result = await toggleTarget(targetId)
    if (!result.success) {
      toast({ variant: 'destructive', title: 'Error', description: result.message })
    }
  }

  // 切换全量流量捕获
  const handleToggleTrafficCapture = async (enabled: boolean) => {
    if (!sessionId) return
    try {
      const result = await api.session.enableTrafficCapture(sessionId, enabled)
      if (result?.success) {
        setTrafficCapturing(enabled)
        toast({ 
          variant: enabled ? 'success' : 'default',
          title: enabled ? 'Start Capture' : 'Stop Capture',
        })
      } else {
        toast({ variant: 'destructive', title: 'Error', description: result?.message })
      }
    } catch (e) {
      toast({ variant: 'destructive', title: 'Error', description: String(e) })
    }
  }

  // 监听 Wails 事件
  useEffect(() => {
    // @ts-ignore
    if (window.runtime?.EventsOn) {
      // @ts-ignore
      const unsubscribeIntercept = window.runtime.EventsOn('intercept-event', addInterceptEvent)
      // @ts-ignore
      const unsubscribeTraffic = window.runtime.EventsOn('traffic-event', addTrafficEvent)
      
      return () => {
        if (unsubscribeIntercept) unsubscribeIntercept()
        if (unsubscribeTraffic) unsubscribeTraffic()
      }
    }
  }, [addInterceptEvent, addTrafficEvent])

  return (
    <div className="h-screen flex flex-col bg-background text-foreground">
      {/* 顶部工具栏 */}
      <div className="h-14 border-b flex items-center px-4 gap-4 shrink-0">
        <div className="flex items-center gap-2 flex-1">
          <Button
            onClick={handleLaunchBrowser}
            variant="outline"
            disabled={isLaunchingBrowser || isConnected}
            title={t('toolbar.launchBrowser')}
          >
            <Chrome className="w-4 h-4 mr-2" />
            {isLaunchingBrowser ? t('toolbar.launching') : t('toolbar.launchBrowser')}
          </Button>
          <Input
            value={devToolsURL}
            onChange={(e) => setDevToolsURL(e.target.value)}
            placeholder={t('toolbar.devtoolsUrlPlaceholder')}
            className="w-80"
            disabled={isConnected}
          />
          <Button 
            onClick={handleConnect}
            variant={isConnected ? "destructive" : "default"}
            disabled={isLoading}
          >
            {isConnected ? <Link2Off className="w-4 h-4 mr-2" /> : <Link2 className="w-4 h-4 mr-2" />}
            {isLoading ? t('common.connecting') : isConnected ? t('common.disconnect') : t('common.connect')}
          </Button>
        </div>
        
        <div className="flex items-center gap-2">
          <div className="flex items-center gap-2 text-sm">
            <span className={`flex items-center gap-1 ${isConnected ? 'text-green-500' : 'text-muted-foreground'}`}>
              <span className={`w-2 h-2 rounded-full ${isConnected ? 'bg-green-500' : 'bg-muted-foreground'}`} />
              {isConnected ? t('common.connected') : t('common.notConnected')}
            </span>
            {isConnected && (
              <span className="text-muted-foreground">
                · {t('common.target')} {attachedTargetId ? 1 : 0}/1
              </span>
            )}
          </div>
          <Button variant="ghost" size="icon" onClick={handleToggleLanguage} title={t('toolbar.toggleLanguage')}>
            <Languages className="w-4 h-4" />
          </Button>
          <Button variant="ghost" size="icon" onClick={toggleTheme} title={t('toolbar.toggleTheme')}>
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
                {t('common.target')}
              </TabsTrigger>
              <TabsTrigger value="rules" className="gap-2">
                <FileJson className="w-4 h-4" />
                {t('common.rules')}
              </TabsTrigger>
              <TabsTrigger value="events" className="gap-2">
                <Activity className="w-4 h-4" />
                {t('common.events')}
              </TabsTrigger>
              <TabsTrigger value="network" className="gap-2">
                <Activity className="w-4 h-4" />
                {t('common.network')}
              </TabsTrigger>
            </TabsList>
          </div>

          <TabsContent value="targets" className="flex-1 overflow-hidden m-0 min-h-0 data-[state=active]:flex data-[state=active]:flex-col">
            <div className="h-full overflow-auto p-4">
              <TargetsPanel 
                targets={targets}
                attachedTargetId={attachedTargetId}
                onToggle={handleToggleTarget}
                isConnected={isConnected}
                onRefresh={() => refreshTargets()}
              />
            </div>
          </TabsContent>

          <TabsContent value="rules" className="flex-1 overflow-hidden m-0 min-h-0 data-[state=active]:flex data-[state=active]:flex-col">
            <div className="h-full overflow-hidden">
              <RulesPanel 
                sessionId={sessionId}
                isConnected={isConnected}
                attachedTargetId={attachedTargetId}
                setIntercepting={setIntercepting}
              />
            </div>
          </TabsContent>

          <TabsContent value="events" className="flex-1 overflow-hidden m-0 min-h-0 data-[state=active]:flex data-[state=active]:flex-col">
            <div className="h-full overflow-auto p-4">
              <EventsPanel 
                matchedEvents={matchedEvents} 
                onClearMatched={clearMatchedEvents}
              />
            </div>
          </TabsContent>

          <TabsContent value="network" className="flex-1 overflow-hidden m-0 min-h-0 data-[state=active]:flex data-[state=active]:flex-col">
            <div className="h-full overflow-auto p-4">
              <NetworkPanel 
                events={trafficEvents}
                isCapturing={isTrafficCapturing}
                onToggleCapture={handleToggleTrafficCapture}
                onClear={clearTrafficEvents}
                isConnected={isConnected}
              />
            </div>
          </TabsContent>
        </Tabs>
      </div>
      
      <div className="h-6 border-t px-4 flex items-center text-xs text-muted-foreground shrink-0">
        <span>cdpnetool v{appVersion}</span>
        <span className="mx-2">|</span>
        <span>Session: {sessionId?.slice(0, 8) || '-'}</span>
      </div>
      
      <Toaster />
    </div>
  )
}

export default App
