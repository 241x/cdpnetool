import { useState, useMemo } from 'react'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { 
  Search, 
  X,
  ChevronDown,
  ChevronRight,
  ChevronUp,
  Trash2,
  Filter,
  Copy,
  Check
} from 'lucide-react'
import type { 
  MatchedEventWithId, 
  FinalResultType 
} from '@/types/events'
import { 
  FINAL_RESULT_LABELS, 
  FINAL_RESULT_COLORS 
} from '@/types/events'
import { useTranslation } from 'react-i18next'

interface EventsPanelProps {
  matchedEvents: MatchedEventWithId[]
  onClearMatched?: () => void
}

export function EventsPanel({ 
  matchedEvents, 
  onClearMatched, 
}: EventsPanelProps) {
  return (
    <div className="h-full flex flex-col">
      <MatchedEventsList events={matchedEvents} onClear={onClearMatched} />
    </div>
  )
}

interface MatchedEventsListProps {
  events: MatchedEventWithId[]
  onClear?: () => void
}

// 匹配事件列表
function MatchedEventsList({ events, onClear }: MatchedEventsListProps) {
  const { t } = useTranslation()
  const [search, setSearch] = useState('')
  const [resultFilter, setResultFilter] = useState<FinalResultType | 'all'>('all')
  const [expandedEvent, setExpandedEvent] = useState<string | null>(null)

  const filteredEvents = useMemo(() => {
    return events.filter(evt => {
      if (resultFilter !== 'all' && evt.networkEvent.finalResult !== resultFilter) return false
      if (search) {
        const searchLower = search.toLowerCase()
        return (
          evt.networkEvent.request.url.toLowerCase().includes(searchLower) ||
          evt.networkEvent.request.method.toLowerCase().includes(searchLower) ||
          evt.networkEvent.matchedRules?.some(r => r.ruleName.toLowerCase().includes(searchLower)) || false
        )
      }
      return true
    })
  }, [events, search, resultFilter])

  const resultCounts = useMemo(() => {
    const counts: Record<string, number> = { all: events.length }
    events.forEach(evt => {
      const result = evt.networkEvent.finalResult || 'passed';
      counts[result] = (counts[result] || 0) + 1
    })
    return counts
  }, [events])

  if (events.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center h-full text-muted-foreground">
        <div className="text-4xl mb-4 opacity-50">✓</div>
        <p>{t('events.noEvents')}</p>
        <p className="text-sm mt-1">{t('events.noEventsSub')}</p>
      </div>
    )
  }

  return (
    <div className="h-full flex flex-col">
      {/* 工具栏 */}
      <div className="flex items-center gap-2 mb-4">
        <div className="relative flex-1 max-w-xs">
          <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
          <Input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder={t('events.searchPlaceholder')}
            className="pl-9 pr-8"
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

        <div className="flex items-center gap-1">
          <Filter className="w-4 h-4 text-muted-foreground" />
          <select
            value={resultFilter}
            onChange={(e) => setResultFilter(e.target.value as FinalResultType | 'all')}
            className="h-9 px-2 rounded-md border bg-background text-sm"
          >
            <option value="all">{t('events.all')} ({resultCounts.all})</option>
            {Object.entries(FINAL_RESULT_LABELS).map(([type, label]) => (
              resultCounts[type] > 0 && (
                <option key={type} value={type}>
                  {label} ({resultCounts[type]})
                </option>
              )
            ))}
          </select>
        </div>

        {onClear && (
          <Button variant="outline" size="sm" onClick={onClear}>
            <Trash2 className="w-4 h-4 mr-1" />
            {t('events.clear')}
          </Button>
        )}
      </div>

      <div className="text-sm text-muted-foreground mb-3">
        {t('rules.ruleCount', { count: filteredEvents.length })} {search && `(${t('common.refresh')})`}
      </div>

      <ScrollArea className="flex-1">
        <div className="space-y-2 pr-4">
          {filteredEvents.map((evt) => (
            <MatchedEventItem
              key={evt.id}
              event={evt}
              isExpanded={expandedEvent === evt.id}
              onToggleExpand={() => setExpandedEvent(expandedEvent === evt.id ? null : evt.id)}
            />
          ))}
        </div>
      </ScrollArea>
    </div>
  )
}

// 事件详情视图（参考 Chrome DevTools 布局）
function EventDetailView({ event }: { event: MatchedEventWithId }) {
  const { t } = useTranslation()
  const { networkEvent } = event
  const { request, response, matchedRules, finalResult } = networkEvent

  // 状态管理：默认展开常规
  const [collapsed, setCollapsed] = useState({
    general: false,
    rules: true,
    responseHeaders: true,
    requestHeaders: true,
  })

  const toggleSection = (key: keyof typeof collapsed) => {
    setCollapsed(prev => ({ ...prev, [key]: !prev[key] }))
  }

  // 解码 Base64 编码的字符串
  const decodeBase64 = (base64Str: string): string => {
    try {
      const binaryString = atob(base64Str)
      const bytes = new Uint8Array(binaryString.length)
      for (let i = 0; i < binaryString.length; i++) {
        bytes[i] = binaryString.charCodeAt(i)
      }
      return new TextDecoder('utf-8').decode(bytes)
    } catch {
      return base64Str
    }
  }

  const isTextContent = (contentType: string): boolean => {
    const textTypes = [
      'text/',
      'application/json',
      'application/xml',
      'application/javascript',
      'application/x-www-form-urlencoded'
    ]
    return textTypes.some(type => contentType.toLowerCase().includes(type))
  }

  const formatBytes = (bytes: number): string => {
    if (bytes < 1024) return bytes + ' B'
    if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(2) + ' KB'
    return (bytes / (1024 * 1024)).toFixed(2) + ' MB'
  }

  const formattedRequestBody = useMemo(() => {
    if (!request.body) return null
    return decodeBase64(request.body)
  }, [request.body])

  const formattedResponseBody = useMemo((): { isPreviewable: boolean; content?: string; size?: string; type?: string } | null => {
    if (!response?.body) return null
    
    const contentType = response.headers?.['content-type'] || ''
    
    // 非文本类型
    if (!isTextContent(contentType)) {
      const size = response.body.length
      return {
        isPreviewable: false,
        size: formatBytes(size),
        type: contentType || 'application/octet-stream'
      }
    }
    
    // 文本类型：解码 base64
    const decoded = decodeBase64(response.body)
    return { isPreviewable: true, content: decoded }
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
          <ScrollArea className="max-h-[500px]">
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
                      <span className="break-all">{request.url}</span>
                    </div>
                    <div className="flex gap-2">
                      <span className="text-muted-foreground min-w-[140px] shrink-0">{t('events.fields.requestMethod')}:</span>
                      <span>{request.method}</span>
                    </div>
                    {request.resourceType && (
                      <div className="flex gap-2">
                        <span className="text-muted-foreground min-w-[140px] shrink-0">{t('events.fields.resourceType')}:</span>
                        <span className="font-semibold text-blue-600 dark:text-blue-400">{request.resourceType}</span>
                      </div>
                    )}
                    {response && response.statusCode !== undefined && response.statusCode !== null && (
                      <div className="flex gap-2">
                        <span className="text-muted-foreground min-w-[140px] shrink-0">{t('events.fields.statusCode')}:</span>
                        <span className={response.statusCode >= 400 ? 'text-red-500' : 'text-green-500 font-bold'}>
                          {response.statusCode}
                        </span>
                      </div>
                    )}
                    {finalResult && (
                      <div className="flex gap-2">
                        <span className="text-muted-foreground min-w-[140px] shrink-0">{t('events.fields.finalResult')}:</span>
                        <span className="font-bold">{FINAL_RESULT_LABELS[finalResult as FinalResultType] || finalResult}</span>
                      </div>
                    )}
                    {networkEvent.target && (
                      <div className="flex gap-2">
                        <span className="text-muted-foreground min-w-[140px] shrink-0">{t('events.fields.targetId')}:</span>
                        <span className="text-muted-foreground truncate">{networkEvent.target}</span>
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
                          <span className="text-primary font-bold shrink-0">{k}:</span>
                          <span className="break-all">{v}</span>
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
                          <span className="text-primary font-bold shrink-0">{k}:</span>
                          <span className="break-all">{v}</span>
                        </div>
                      ))
                    ) : (
                      <div className="text-muted-foreground italic">{t('events.common.noData')}</div>
                    )}
                  </div>
                )}
              </section>

              {/* 匹配规则 */}
              {matchedRules && matchedRules.length > 0 && (
                <section className="pb-2">
                  <button 
                    onClick={() => toggleSection('rules')}
                    className="w-full flex items-center gap-1 text-[11px] font-bold text-muted-foreground uppercase hover:text-foreground transition-colors"
                  >
                    {collapsed.rules ? <ChevronRight className="w-3 h-3" /> : <ChevronDown className="w-3 h-3" />} {t('events.sections.rules')}
                  </button>
                  {!collapsed.rules && (
                    <div className="mt-2 ml-4 space-y-2 text-xs">
                      {matchedRules.map((rule, idx) => (
                        <div key={idx} className="p-2 bg-muted/50 rounded-md border flex items-center gap-3">
                          <span className="font-bold text-primary">{rule.ruleName}</span>
                          <div className="flex gap-1.5 flex-wrap">
                            {rule.actions.map((action, i) => (
                              <Badge key={i} variant="outline" className="text-[10px] py-0 h-4 bg-background">
                                {action}
                              </Badge>
                            ))}
                          </div>
                        </div>
                      ))}
                    </div>
                  )}
                </section>
              )}
            </div>
          </ScrollArea>
        </TabsContent>

        <TabsContent value="payload" className="m-0">
          <div className="p-4">
            {request.body ? (
              <>
                <div className="flex items-center justify-between mb-2">
                  <div className="text-[11px] font-bold text-muted-foreground uppercase">{t('events.payload.title')}</div>
                  <CopyButton content={formattedRequestBody || ''} />
                </div>
                <ScrollArea className="h-[200px]">
                  <pre className="text-xs font-mono p-4 bg-muted/50 rounded-lg border break-all leading-relaxed" style={{ whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}>
                    {formattedRequestBody}
                  </pre>
                </ScrollArea>
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
                    <ScrollArea className="h-[200px]">
                      <pre className="text-xs font-mono p-4 bg-muted/50 rounded-lg border break-all leading-relaxed" style={{ whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}>
                        {formattedResponseBody.content}
                      </pre>
                    </ScrollArea>
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

interface MatchedEventItemProps {
  event: MatchedEventWithId
  isExpanded: boolean
  onToggleExpand: () => void
}

// 匹配事件项
function MatchedEventItem({ event, isExpanded, onToggleExpand }: MatchedEventItemProps) {
  const { t } = useTranslation()
  const colors = FINAL_RESULT_COLORS[event.networkEvent.finalResult!] || FINAL_RESULT_COLORS.passed
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
        {/* 结果标签 */}
        <Badge variant="outline" className={`${colors.bg} ${colors.text} border-0 text-xs`}>
          {FINAL_RESULT_LABELS[event.networkEvent.finalResult!]}
        </Badge>

        {/* Method */}
        <span className="font-mono text-xs font-medium px-1.5 py-0.5 rounded bg-muted">
          {event.networkEvent.request.method}
        </span>

        {/* URL */}
        <span className="flex-1 text-sm truncate text-muted-foreground font-mono">
          {event.networkEvent.request.url}
        </span>

        {/* 匹配规则数 */}
        <Badge variant="secondary" className="text-xs">
          {event.networkEvent.matchedRules?.length || 0} {t('common.rules')}
        </Badge>

        {/* 时间 */}
        <span className="text-xs text-muted-foreground shrink-0">
          {formatTime(event.networkEvent.timestamp)}
        </span>

        {isExpanded ? <ChevronUp className="w-4 h-4" /> : <ChevronDown className="w-4 h-4" />}
      </div>

      {/* 展开详情 */}
      {isExpanded && <EventDetailView event={event} />}
    </div>
  )
}

function CopyButton({ content }: { content: string }) {
  const { t } = useTranslation()
  const [copied, setCopied] = useState(false)

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(content)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    } catch (err) {
      console.error('Failed to copy:', err)
    }
  }

  return (
    <Button
      variant="ghost"
      size="sm"
      onClick={handleCopy}
      className="h-6 px-2"
    >
      {copied ? (
        <>
          <Check className="w-3 h-3 mr-1" />
          <span className="text-xs">{t('events.common.copied')}</span>
        </>
      ) : (
        <>
          <Copy className="w-3 h-3 mr-1" />
          <span className="text-xs">{t('events.common.copy')}</span>
        </>
      )}
    </Button>
  )
}
