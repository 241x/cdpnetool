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
  CheckCircle
} from 'lucide-react'
import type { 
  MatchedEventWithId, 
  FinalResultType 
} from '@/types/events'
import { 
  FINAL_RESULT_LABELS, 
  FINAL_RESULT_COLORS 
} from '@/types/events'

interface EventsPanelProps {
  matchedEvents: MatchedEventWithId[]
  onClearMatched?: () => void
}

export function EventsPanel({ 
  matchedEvents, 
  onClearMatched, 
}: EventsPanelProps) {
  const totalMatched = matchedEvents.length

  return (
    <div className="h-full flex flex-col">
      <div className="flex items-center justify-between mb-2">
        <div className="flex items-center gap-2 px-3 py-1.5 text-sm font-medium">
          <CheckCircle className="w-4 h-4 text-primary" />
          匹配请求
          {totalMatched > 0 && (
            <Badge variant="secondary" className="ml-1 text-xs">{totalMatched}</Badge>
          )}
        </div>
      </div>

      <div className="flex-1 flex flex-col overflow-hidden">
        <MatchedEventsList events={matchedEvents} onClear={onClearMatched} />
      </div>
    </div>
  )
}

interface MatchedEventsListProps {
  events: MatchedEventWithId[]
  onClear?: () => void
}

// 匹配事件列表
function MatchedEventsList({ events, onClear }: MatchedEventsListProps) {
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
        <p>暂无匹配事件</p>
        <p className="text-sm mt-1">匹配规则的请求将在此显示</p>
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
            placeholder="搜索 URL、方法、规则名..."
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
            <option value="all">全部 ({resultCounts.all})</option>
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
            清除
          </Button>
        )}
      </div>

      <div className="text-sm text-muted-foreground mb-3">
        共 {filteredEvents.length} 条 {search && '（搜索结果）'}
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
  const { networkEvent } = event
  const { request, response, matchedRules, finalResult } = networkEvent

  // 状态管理：默认全部折叠
  const [collapsed, setCollapsed] = useState({
    general: false,
    rules: false,
    responseHeaders: false,
    requestHeaders: false,
  })

  const toggleSection = (key: keyof typeof collapsed) => {
    setCollapsed(prev => ({ ...prev, [key]: !prev[key] }))
  }

  const formattedRequestBody = useMemo(() => {
    if (!request.body) return null
    try {
      const parsed = JSON.parse(request.body)
      return JSON.stringify(parsed, null, 2)
    } catch {
      return request.body
    }
  }, [request.body])

  const formattedResponseBody = useMemo(() => {
    if (!response?.body) return null
    try {
      const parsed = JSON.parse(response.body)
      return JSON.stringify(parsed, null, 2)
    } catch {
      return response.body
    }
  }, [response?.body])

  return (
    <div className="border-t bg-card">
      <Tabs defaultValue="headers" className="w-full">
        <TabsList className="w-full justify-start rounded-none border-b bg-muted/30 h-9 p-0">
          <TabsTrigger 
            value="headers" 
            className="rounded-none border-b-2 border-transparent data-[state=active]:border-primary data-[state=active]:bg-background h-9 px-4 text-xs font-medium"
          >
            标头
          </TabsTrigger>
          <TabsTrigger 
            value="payload" 
            className="rounded-none border-b-2 border-transparent data-[state=active]:border-primary data-[state=active]:bg-background h-9 px-4 text-xs font-medium"
          >
            负载
          </TabsTrigger>
          <TabsTrigger 
            value="response" 
            className="rounded-none border-b-2 border-transparent data-[state=active]:border-primary data-[state=active]:bg-background h-9 px-4 text-xs font-medium"
          >
            响应
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
                  {collapsed.general ? <ChevronDown className="w-3 h-3" /> : <ChevronRight className="w-3 h-3" />} 常规
                </button>
                {collapsed.general && (
                  <div className="mt-2 ml-4 space-y-1.5 text-xs font-mono">
                    <div className="flex gap-2">
                      <span className="text-muted-foreground min-w-[140px] shrink-0">Request URL:</span>
                      <span className="break-all">{request.url}</span>
                    </div>
                    <div className="flex gap-2">
                      <span className="text-muted-foreground min-w-[140px] shrink-0">Request Method:</span>
                      <span>{request.method}</span>
                    </div>
                    {response && response.statusCode !== undefined && response.statusCode !== null && (
                      <div className="flex gap-2">
                        <span className="text-muted-foreground min-w-[140px] shrink-0">Status Code:</span>
                        <span className={response.statusCode >= 400 ? 'text-red-500' : 'text-green-500 font-bold'}>
                          {response.statusCode}
                        </span>
                      </div>
                    )}
                    {finalResult && (
                      <div className="flex gap-2">
                        <span className="text-muted-foreground min-w-[140px] shrink-0">Final Result:</span>
                        <span className="font-bold">{FINAL_RESULT_LABELS[finalResult as FinalResultType] || finalResult}</span>
                      </div>
                    )}
                    {networkEvent.target && (
                      <div className="flex gap-2">
                        <span className="text-muted-foreground min-w-[140px] shrink-0">Target ID:</span>
                        <span className="text-muted-foreground truncate">{networkEvent.target}</span>
                      </div>
                    )}
                  </div>
                )}
              </section>

              {/* 匹配规则 */}
              {matchedRules && matchedRules.length > 0 && (
                <section className="border-b border-muted/30 pb-2">
                  <button 
                    onClick={() => toggleSection('rules')}
                    className="w-full flex items-center gap-1 text-[11px] font-bold text-muted-foreground uppercase hover:text-foreground transition-colors"
                  >
                    {collapsed.rules ? <ChevronDown className="w-3 h-3" /> : <ChevronRight className="w-3 h-3" />} 匹配规则
                  </button>
                  {collapsed.rules && (
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

              {/* 响应标头 */}
              <section className="border-b border-muted/30 pb-2">
                <button 
                  onClick={() => toggleSection('responseHeaders')}
                  className="w-full flex items-center gap-1 text-[11px] font-bold text-muted-foreground uppercase hover:text-foreground transition-colors"
                >
                  {collapsed.responseHeaders ? <ChevronDown className="w-3 h-3" /> : <ChevronRight className="w-3 h-3" />} 响应标头
                </button>
                {collapsed.responseHeaders && (
                  <div className="mt-2 ml-4 space-y-1 text-xs font-mono border-l-2 pl-3">
                    {response?.headers && Object.keys(response.headers).length > 0 ? (
                      Object.entries(response.headers).map(([k, v]) => (
                        <div key={k} className="flex gap-2 py-0.5 border-b border-muted/30 last:border-0">
                          <span className="text-primary font-bold shrink-0">{k}:</span>
                          <span className="break-all">{v}</span>
                        </div>
                      ))
                    ) : (
                      <div className="text-muted-foreground italic">无标头数据</div>
                    )}
                  </div>
                )}
              </section>

              {/* 请求标头 */}
              <section className="pb-2">
                <button 
                  onClick={() => toggleSection('requestHeaders')}
                  className="w-full flex items-center gap-1 text-[11px] font-bold text-muted-foreground uppercase hover:text-foreground transition-colors"
                >
                  {collapsed.requestHeaders ? <ChevronDown className="w-3 h-3" /> : <ChevronRight className="w-3 h-3" />} 请求标头
                </button>
                {collapsed.requestHeaders && (
                  <div className="mt-2 ml-4 space-y-1 text-xs font-mono border-l-2 pl-3">
                    {request.headers && Object.keys(request.headers).length > 0 ? (
                      Object.entries(request.headers).map(([k, v]) => (
                        <div key={k} className="flex gap-2 py-0.5 border-b border-muted/30 last:border-0">
                          <span className="text-primary font-bold shrink-0">{k}:</span>
                          <span className="break-all">{v}</span>
                        </div>
                      ))
                    ) : (
                      <div className="text-muted-foreground italic">无标头数据</div>
                    )}
                  </div>
                )}
              </section>
            </div>
          </ScrollArea>
        </TabsContent>

        <TabsContent value="payload" className="m-0">
          <ScrollArea className="max-h-[500px]">
            <div className="p-4">
              {request.body ? (
                <>
                  <div className="flex items-center justify-between mb-2">
                    <span className="text-[11px] font-bold text-muted-foreground uppercase">Request Payload</span>
                    {request.body.trim().startsWith('{') && <Badge variant="outline" className="text-[10px]">JSON</Badge>}
                  </div>
                  <pre className="text-xs font-mono p-4 bg-muted/50 rounded-lg border overflow-auto whitespace-pre-wrap leading-relaxed">
                    {formattedRequestBody}
                  </pre>
                </>
              ) : (
                <div className="flex flex-col items-center justify-center py-12 text-muted-foreground">
                  <div className="text-xs italic">此请求没有负载数据</div>
                </div>
              )}
            </div>
          </ScrollArea>
        </TabsContent>

        <TabsContent value="response" className="m-0">
          <ScrollArea className="max-h-[500px]">
            <div className="p-4">
              {response?.body ? (
                <>
                  <div className="flex items-center justify-between mb-2">
                    <span className="text-[11px] font-bold text-muted-foreground uppercase">Response Body</span>
                    {response.body.trim().startsWith('{') && <Badge variant="outline" className="text-[10px]">JSON</Badge>}
                  </div>
                  <pre className="text-xs font-mono p-4 bg-muted/50 rounded-lg border overflow-auto whitespace-pre-wrap leading-relaxed">
                    {formattedResponseBody}
                  </pre>
                </>
              ) : (
                <div className="flex flex-col items-center justify-center py-12 text-muted-foreground">
                  <div className="text-xs italic">此请求没有响应体数据</div>
                </div>
              )}
            </div>
          </ScrollArea>
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
  const colors = FINAL_RESULT_COLORS[event.networkEvent.finalResult!] || FINAL_RESULT_COLORS.passed
  const formatTime = (ts: number) => {
    return new Date(ts).toLocaleTimeString('zh-CN', { 
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
          {event.networkEvent.matchedRules?.length || 0} 规则
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
