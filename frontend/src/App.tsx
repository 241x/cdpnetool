import { useState, useEffect } from 'react'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Toaster } from '@/components/ui/toaster'
import { StatusIndicator } from '@/components/ui/status-indicator'
import { useToast } from '@/hooks/use-toast'
import { useSessionStore } from '@/stores'
import { EventsPanel } from '@/components/events'
import { NetworkPanel } from '@/components/network/NetworkPanel'
import { TargetsPanel } from '@/components/targets/TargetsPanel'
import { RulesPanel } from '@/components/rules/RulesPanel'
import { SettingsDialog } from '@/components/settings/SettingsDialog'
import { AboutDialog } from '@/components/about'
import { api } from '@/api'
import { useTranslation } from 'react-i18next'
import { getErrorMessage } from '@/lib/error-handler'
import { 
  Link2,
  Link2Off,
  FileJson,
  Activity,
  Chrome,
  Languages,
  Bug,
  Settings,
  Info
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
    isIntercepting,
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
  
  const { toast } = useToast()
  const [isLoading, setIsLoading] = useState(false)
  const [isLaunchingBrowser, setIsLaunchingBrowser] = useState(false)
  const [appVersion, setAppVersion] = useState('')
  const [activeTab, setActiveTab] = useState('targets')
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [aboutOpen, setAboutOpen] = useState(false)

  // 监听 Tab 切换，离开请求预览时自动停止捕获
  const handleTabChange = async (value: string) => {
    // 离开请求预览页面时，静默停止捕获（不显示提示）
    if (activeTab === 'network' && value !== 'network' && isTrafficCapturing) {
      await handleToggleTrafficCapture(false, true)
    }
    setActiveTab(value)
  }

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
          toast({ variant: 'destructive', title: t('errors.title'), description: getErrorMessage(result, t) })
        }
      } catch (e) {
        toast({ variant: 'destructive', title: t('errors.title'), description: String(e) })
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
          toast({ variant: 'destructive', title: t('errors.title'), description: getErrorMessage(result, t) })
        }
      } catch (e) {
        toast({ variant: 'destructive', title: t('errors.title'), description: String(e) })
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
      description: t('common.languageName'),
    })
  }

  // 切换目标处理
  const handleToggleTarget = async (targetId: string) => {
    const result = await toggleTarget(targetId)
    if (!result.success) {
      toast({ variant: 'destructive', title: t('errors.title'), description: getErrorMessage(result, t) })
    }
  }

  // 切换全量流量捕获
  const handleToggleTrafficCapture = async (enabled: boolean, silent = false) => {
    if (!sessionId) return
    try {
      const result = await api.session.enableTrafficCapture(sessionId, enabled)
      if (result?.success) {
        setTrafficCapturing(enabled)
        if (!silent) {
          toast({ 
            variant: enabled ? 'success' : 'default',
            title: enabled ? t('network.startCapture') : t('network.stopCapture'),
          })
        }
      } else {
        toast({ variant: 'destructive', title: t('errors.title'), description: getErrorMessage(result, t) })
      }
    } catch (e) {
      toast({ variant: 'destructive', title: t('errors.title'), description: String(e) })
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
          <StatusIndicator 
            steps={[
              {
                key: 'browser',
                label: t('status.browserConnected'),
                status: isConnected ? 'completed' : 'current'
              },
              {
                key: 'target',
                label: t('status.targetAttached'),
                status: !isConnected ? 'pending' : (attachedTargetId ? 'completed' : 'current')
              },
              {
                key: 'config',
                label: t('status.configEnabled'),
                status: (!isConnected || !attachedTargetId) ? 'pending' : (isIntercepting ? 'completed' : 'current')
              }
            ]}
          />
          <Button variant="ghost" size="icon" onClick={handleToggleLanguage} title={t('toolbar.toggleLanguage')}>
            <Languages className="w-4 h-4" />
          </Button>
          <Button variant="ghost" size="icon" onClick={() => setSettingsOpen(true)} title={t('toolbar.settings')}>
            <Settings className="w-4 h-4" />
          </Button>
        </div>
      </div>

      {/* 主内容区 */}
      <div className="flex-1 flex flex-col overflow-hidden min-h-0">
        <Tabs defaultValue="targets" value={activeTab} onValueChange={handleTabChange} className="flex-1 flex flex-col min-h-0">
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
                <Bug className="w-4 h-4" />
                {t('common.requestPreview')}
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
            <RulesPanel 
              sessionId={sessionId}
              isConnected={isConnected}
              attachedTargetId={attachedTargetId}
              setIntercepting={setIntercepting}
            />
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
                attachedTargetId={attachedTargetId}
              />
            </div>
          </TabsContent>
        </Tabs>
      </div>
      
      <div className="h-6 border-t px-4 flex items-center text-xs text-muted-foreground shrink-0">
        <Button 
          variant="ghost" 
          size="icon"
          className="h-5 w-5 p-0 mr-2"
          onClick={() => setAboutOpen(true)}
          title={t('settings.about.title')}
        >
          <Info className="w-3.5 h-3.5" />
        </Button>
        <span>cdpnetool v{appVersion}</span>
        <span className="mx-2">|</span>
        <span>Session: {sessionId?.slice(0, 8) || '-'}</span>
      </div>
      
      <Toaster />
      <SettingsDialog open={settingsOpen} onOpenChange={setSettingsOpen} />
      <AboutDialog open={aboutOpen} onOpenChange={setAboutOpen} />
    </div>
  )
}

export default App
