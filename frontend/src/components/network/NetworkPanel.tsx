import { useState, useMemo } from 'react'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { CopyButton } from '@/components/ui/copy-button'
import { 
  Search, 
  X,
  Trash2,
  Activity,
  Play,
  Square,
  ChevronDown,
  ChevronUp,
  ChevronRight
} from 'lucide-react'
import type { NetworkEvent, Response as TrafficResponse, Request as TrafficRequest } from '@/types/events'
import { useTranslation } from 'react-i18next'

interface NetworkPanelProps {
  events: NetworkEvent[]
  isCapturing: boolean
  onToggleCapture: (enabled: boolean) => void
  onClear?: () => void
  isConnected: boolean
  attachedTargetId: string | null
}

export function NetworkPanel({ 
  events, 
  isCapturing, 
  onToggleCapture, 
  onClear,
  isConnected,
  attachedTargetId
}: NetworkPanelProps) {
  const { t } = useTranslation()
  const [search, setSearch] = useState('')
  const [expandedEvent, setExpandedEvent] = useState<string | null>(null)

  const filteredEvents = useMemo(() => {
    if (!search) return events
    const searchLower = search.toLowerCase()
    return events.filter(evt => 
      evt.request.url.toLowerCase().includes(searchLower) ||
      evt.request.method.toLowerCase().includes(searchLower)
    )
  }, [events, search])

  return (
    <div className="h-full flex flex-col">
      {/* 工具栏 */}
      <div className="flex items-center gap-4 mb-4 shrink-0">
        <div className="flex items-center gap-2">
          <Button
            size="sm"
            variant={isCapturing ? "destructive" : "default"}
            onClick={() => onToggleCapture(!isCapturing)}
            disabled={!isConnected || !attachedTargetId}
            className="h-8"
          >
            {isCapturing ? (
              <><Square className="w-3 h-3 mr-1.5 fill-current" /> {t('network.stopCapture')}</>
            ) : (
              <><Play className="w-3 h-3 mr-1.5 fill-current" /> {t('network.startCapture')}</>
            )}
          </Button>
          <Button variant="outline" size="sm" onClick={onClear} className="h-8">
            <Trash2 className="w-4 h-4 mr-1.5" />
            {t('events.clear')}
          </Button>
        </div>

        <div className="relative flex-1 max-w-sm">
          <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
          <Input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder={t('network.searchPlaceholder')}
            className="h-8 pl-9 pr-8"
          />
          {search && (
            <button 
              onClick={() => setSearch('')}
              className="absolute right-2.5 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
            >
              <X className="w-4 h-4" />
            </button>
          )}
        </div>
      </div>

      <div className="text-sm text-muted-foreground mb-3">
        {isCapturing && <span className="inline-flex items-center gap-1.5 text-red-500 animate-pulse mr-2">
          <span className="w-1.5 h-1.5 rounded-full bg-red-500" /> {t('rules.running')}
        </span>}
        {t('rules.ruleCount', { count: filteredEvents.length })}
      </div>

      <div className="flex-1 overflow-auto">
        <div className="space-y-2 pr-4">
          {filteredEvents.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-20 text-muted-foreground">
              <Activity className="w-10 h-10 mb-4 opacity-20" />
              <p>{isConnected ? (isCapturing ? t('network.waiting') : t('network.clickToStart')) : t('targets.connectFirst')}</p>
            </div>
          ) : (
            filteredEvents.map((evt) => (
              <TrafficItem
                key={evt.id}
                event={evt}
                isExpanded={expandedEvent === evt.id}
                onToggleExpand={() => setExpandedEvent(expandedEvent === evt.id ? null : evt.id)}
              />
            ))
          )}
        </div>
      </div>
    </div>
  )
}

function TrafficItem({ event, isExpanded, onToggleExpand }: { event: NetworkEvent, isExpanded: boolean, onToggleExpand: () => void }) {
  const { request, response, timestamp } = event
  
  const statusColor = response ? (
    response.statusCode >= 400 ? 'text-red-500' : 
    response.statusCode >= 300 ? 'text-yellow-500' : 'text-green-500'
  ) : 'text-muted-foreground'

  const formatTime = (ts: number) => {
    return new Date(ts).toLocaleTimeString(undefined, { 
      hour: '2-digit', 
      minute: '2-digit', 
      second: '2-digit',
      hour12: false 
    })
  }

  return (
    <div className="border rounded-lg bg-card overflow-hidden">
      {/* 头部 */}
      <div 
        className="flex items-center gap-2 p-2.5 cursor-pointer hover:bg-muted/50 transition-colors"
        onClick={onToggleExpand}
      >
        {/* Method */}
        <span className="font-mono text-xs font-medium px-1.5 py-0.5 rounded bg-muted">
          {request.method}
        </span>

        {/* Status Code */}
        <span className={`font-mono text-xs font-bold ${statusColor}`}>
          {response?.statusCode || 'Pending'}
        </span>

        {/* URL */}
        <span className="flex-1 text-sm truncate text-muted-foreground font-mono">
          {request.url}
        </span>

        {/* 时间 */}
        <span className="text-xs text-muted-foreground shrink-0">
          {formatTime(timestamp)}
        </span>

        {isExpanded ? <ChevronUp className="w-4 h-4" /> : <ChevronDown className="w-4 h-4" />}
      </div>

      {/* 展开详情 */}
      {isExpanded && <TrafficDetailView request={request} response={response} />}
    </div>
  )
}

function TrafficDetailView({ request, response }: { request: TrafficRequest, response?: TrafficResponse }) {
  const { t } = useTranslation()
  
  // 状态管理：默认展开常规和响应标头，请求标头默认收起
  const [collapsed, setCollapsed] = useState({
    general: false,
    responseHeaders: true,
    requestHeaders: true,
  })

  const toggleSection = (key: keyof typeof collapsed) => {
    setCollapsed(prev => ({ ...prev, [key]: !prev[key] }))
  }
  
  // 解码 Base64 编码的请求体
  const formattedRequestBody = useMemo(() => {
    if (!request.body) return null
    try {
      const binaryString = atob(request.body)
      const bytes = new Uint8Array(binaryString.length)
      for (let i = 0; i < binaryString.length; i++) {
        bytes[i] = binaryString.charCodeAt(i)
      }
      return new TextDecoder('utf-8').decode(bytes)
    } catch {
      return request.body
    }
  }, [request.body])

  // 解码 Response Body
  const formattedResponseBody = useMemo((): { isPreviewable: boolean; content?: string; size?: string; type?: string } | null => {
    if (!response?.body) return null
    
    const contentType = response.headers?.['content-type'] || ''
    
    // 简单判断是否为文本类型
    const isTextContent = (ct: string): boolean => {
      const textTypes = [
        'text/',
        'application/json',
        'application/xml',
        'application/javascript',
        'application/x-www-form-urlencoded'
      ]
      return textTypes.some(type => ct.toLowerCase().includes(type))
    }
    
    // 非文本类型
    if (!isTextContent(contentType)) {
      const size = response.body.length
      const formatBytes = (bytes: number): string => {
        if (bytes < 1024) return bytes + ' B'
        if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(2) + ' KB'
        return (bytes / (1024 * 1024)).toFixed(2) + ' MB'
      }
      return {
        isPreviewable: false,
        size: formatBytes(size),
        type: contentType || 'application/octet-stream'
      }
    }
    
    // 文本类型：解码 base64
    try {
      const binaryString = atob(response.body)
      const bytes = new Uint8Array(binaryString.length)
      for (let i = 0; i < binaryString.length; i++) {
        bytes[i] = binaryString.charCodeAt(i)
      }
      const decoded = new TextDecoder('utf-8').decode(bytes)
      return { isPreviewable: true, content: decoded }
    } catch {
      return { isPreviewable: true, content: response.body }
    }
  }, [response?.body, response?.headers])

  return (
    <div className="border-t bg-card">
      <Tabs defaultValue="headers" className="w-full">
        <TabsList className="w-full justify-start rounded-none border-b bg-muted/30 h-9 p-0">
          <TabsTrigger 
            value="headers" 
            className="rounded-none border-b-2 border-transparent data-[state=active]:border-primary data-[state=active]:bg-background h-9 px-4 text-xs font-medium"
          >
            {t('events.tabs.headers')}
          </TabsTrigger>
          <TabsTrigger 
            value="payload" 
            className="rounded-none border-b-2 border-transparent data-[state=active]:border-primary data-[state=active]:bg-background h-9 px-4 text-xs font-medium"
          >
            {t('events.tabs.payload')}
          </TabsTrigger>
          <TabsTrigger 
            value="response" 
            className="rounded-none border-b-2 border-transparent data-[state=active]:border-primary data-[state=active]:bg-background h-9 px-4 text-xs font-medium"
          >
            {t('events.tabs.response')}
          </TabsTrigger>
        </TabsList>

        <TabsContent value="headers" className="m-0">
          <div className="max-h-[300px] overflow-auto">
            <div className="p-4 space-y-2">
              {/* 常规 (General) */}
              <section className="border-b border-muted/30 pb-2">
                <button 
                  onClick={() => toggleSection('general')}
                  className="w-full flex items-center gap-1 text-[11px] font-bold text-muted-foreground uppercase hover:text-foreground transition-colors"
                >
                  {collapsed.general ? <ChevronRight className="w-3 h-3" /> : <ChevronDown className="w-3 h-3" />} {t('events.sections.general')}
                </button>
                {!collapsed.general && (
                  <div className="mt-2 ml-4 space-y-1.5 text-xs font-mono">
                    <div className="flex gap-2">
                      <span className="text-muted-foreground min-w-[140px] shrink-0">{t('events.fields.requestUrl')}:</span>
                      <span className="break-all selectable">{request.url}</span>
                    </div>
                    <div className="flex gap-2">
                      <span className="text-muted-foreground min-w-[140px] shrink-0">{t('events.fields.requestMethod')}:</span>
                      <span className="selectable">{request.method}</span>
                    </div>
                    {request.resourceType && (
                      <div className="flex gap-2">
                        <span className="text-muted-foreground min-w-[140px] shrink-0">{t('events.fields.resourceType')}:</span>
                        <span className="font-semibold text-blue-600 dark:text-blue-400 selectable">{request.resourceType}</span>
                      </div>
                    )}
                    {response && response.statusCode !== undefined && response.statusCode !== null && (
                      <div className="flex gap-2">
                        <span className="text-muted-foreground min-w-[140px] shrink-0">{t('events.fields.statusCode')}:</span>
                        <span className={response.statusCode >= 400 ? 'text-red-500 selectable' : 'text-green-500 font-bold selectable'}>
                          {response.statusCode}
                        </span>
                      </div>
                    )}
                  </div>
                )}
              </section>

              {/* 响应标头 */}
              <section className="border-b border-muted/30 pb-2">
                <button 
                  onClick={() => toggleSection('responseHeaders')}
                  className="w-full flex items-center gap-1 text-[11px] font-bold text-muted-foreground uppercase hover:text-foreground transition-colors"
                >
                  {collapsed.responseHeaders ? <ChevronRight className="w-3 h-3" /> : <ChevronDown className="w-3 h-3" />} {t('events.sections.responseHeaders')}
                </button>
                {!collapsed.responseHeaders && (
                  <div className="mt-2 ml-4 space-y-1 text-xs font-mono border-l-2 pl-3">
                    {response?.headers && Object.keys(response.headers).length > 0 ? (
                      Object.entries(response.headers).map(([k, v]) => (
                        <div key={k} className="flex gap-2 py-0.5 border-b border-muted/30 last:border-0">
                          <span className="text-primary font-bold shrink-0 selectable">{k}:</span>
                          <span className="break-all selectable">{v}</span>
                        </div>
                      ))
                    ) : (
                      <div className="text-muted-foreground italic">{t('events.common.noData')}</div>
                    )}
                  </div>
                )}
              </section>

              {/* 请求标头 */}
              <section className="border-b border-muted/30 pb-2">
                <button 
                  onClick={() => toggleSection('requestHeaders')}
                  className="w-full flex items-center gap-1 text-[11px] font-bold text-muted-foreground uppercase hover:text-foreground transition-colors"
                >
                  {collapsed.requestHeaders ? <ChevronRight className="w-3 h-3" /> : <ChevronDown className="w-3 h-3" />} {t('events.sections.requestHeaders')}
                </button>
                {!collapsed.requestHeaders && (
                  <div className="mt-2 ml-4 space-y-1 text-xs font-mono border-l-2 pl-3">
                    {request.headers && Object.keys(request.headers).length > 0 ? (
                      Object.entries(request.headers).map(([k, v]) => (
                        <div key={k} className="flex gap-2 py-0.5 border-b border-muted/30 last:border-0">
                          <span className="text-primary font-bold shrink-0 selectable">{k}:</span>
                          <span className="break-all selectable">{v}</span>
                        </div>
                      ))
                    ) : (
                      <div className="text-muted-foreground italic">{t('events.common.noData')}</div>
                    )}
                  </div>
                )}
              </section>
            </div>
          </div>
        </TabsContent>

        <TabsContent value="payload" className="m-0">
          <div className="p-4">
            {request.body ? (
              <>
                <div className="flex items-center justify-between mb-2">
                  <div className="text-[11px] font-bold text-muted-foreground uppercase">{t('events.payload.title')}</div>
                  <CopyButton content={formattedRequestBody || ''} />
                </div>
                <div className="max-h-[300px] overflow-auto">
                  <pre className="text-xs font-mono p-4 bg-muted/50 rounded-lg border break-all leading-relaxed" style={{ whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}>
                    {formattedRequestBody}
                  </pre>
                </div>
              </>
            ) : (
              <div className="flex flex-col items-center justify-center py-12 text-muted-foreground">
                <div className="text-xs italic">{t('events.payload.noData')}</div>
              </div>
            )}
          </div>
        </TabsContent>

        <TabsContent value="response" className="m-0">
          <div className="p-4">
            {response?.body ? (
              <>
                <div className="flex items-center justify-between mb-2">
                  <div className="text-[11px] font-bold text-muted-foreground uppercase">{t('events.response.title')}</div>
                  {formattedResponseBody && 'isPreviewable' in formattedResponseBody && formattedResponseBody.isPreviewable && formattedResponseBody.content && (
                    <CopyButton content={formattedResponseBody.content} />
                  )}
                </div>
                {formattedResponseBody && 'isPreviewable' in formattedResponseBody ? (
                  formattedResponseBody.isPreviewable && formattedResponseBody.content ? (
                    <div className="max-h-[300px] overflow-auto">
                      <pre className="text-xs font-mono p-4 bg-muted/50 rounded-lg border break-all leading-relaxed" style={{ whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}>
                        {formattedResponseBody.content}
                      </pre>
                    </div>
                  ) : !formattedResponseBody.isPreviewable ? (
                    <div className="flex flex-col items-center justify-center py-12 text-muted-foreground">
                      <div className="text-sm mb-2">{t('events.response.cannotPreview')}</div>
                      <div className="text-xs">{t('events.response.type')}: {formattedResponseBody.type}</div>
                      <div className="text-xs">{t('events.response.size')}: {formattedResponseBody.size}</div>
                    </div>
                  ) : null
                ) : null}
              </>
            ) : (
              <div className="flex flex-col items-center justify-center py-12 text-muted-foreground">
                <div className="text-xs italic">{t('events.response.noData')}</div>
              </div>
            )}
          </div>
        </TabsContent>
      </Tabs>
    </div>
  )
}
