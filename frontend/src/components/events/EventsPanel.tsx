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
  ChevronUp,
  Trash2,
  Filter,
  ArrowRight,
  ArrowLeft,
  Copy,
  Check,
  CheckCircle,
  XCircle
} from 'lucide-react'
import type { 
  MatchedEventWithId, 
  UnmatchedEventWithId, 
  FinalResultType 
} from '@/types/events'
import { 
  FINAL_RESULT_LABELS, 
  FINAL_RESULT_COLORS, 
  UNMATCHED_COLORS 
} from '@/types/events'

interface EventsPanelProps {
  matchedEvents: MatchedEventWithId[]
  unmatchedEvents: UnmatchedEventWithId[]
  onClearMatched?: () => void
  onClearUnmatched?: () => void
}

export function EventsPanel({ 
  matchedEvents, 
  unmatchedEvents, 
  onClearMatched, 
  onClearUnmatched 
}: EventsPanelProps) {
  const [activeTab, setActiveTab] = useState<'matched' | 'unmatched'>('matched')

  const totalMatched = matchedEvents.length
  const totalUnmatched = unmatchedEvents.length

  return (
    <div className="h-full flex flex-col">
      <Tabs value={activeTab} onValueChange={(v) => setActiveTab(v as 'matched' | 'unmatched')} className="flex-1 flex flex-col">
        <TabsList className="w-fit mb-4">
          <TabsTrigger value="matched" className="gap-2">
            <CheckCircle className="w-4 h-4" />
            匹配请求
            {totalMatched > 0 && (
              <Badge variant="secondary" className="ml-1 text-xs">{totalMatched}</Badge>
            )}
          </TabsTrigger>
          <TabsTrigger value="unmatched" className="gap-2">
            <XCircle className="w-4 h-4" />
            未匹配请求
            {totalUnmatched > 0 && (
              <Badge variant="secondary" className="ml-1 text-xs">{totalUnmatched}</Badge>
            )}
          </TabsTrigger>
        </TabsList>

        <TabsContent value="matched" className="flex-1 m-0 overflow-hidden">
          <MatchedEventsList events={matchedEvents} onClear={onClearMatched} />
        </TabsContent>

        <TabsContent value="unmatched" className="flex-1 m-0 overflow-hidden">
          <UnmatchedEventsList events={unmatchedEvents} onClear={onClearUnmatched} />
        </TabsContent>
      </Tabs>
    </div>
  )
}

// ========== 匹配事件列表 ==========
interface MatchedEventsListProps {
  events: MatchedEventWithId[]
  onClear?: () => void
}

function MatchedEventsList({ events, onClear }: MatchedEventsListProps) {
  const [search, setSearch] = useState('')
  const [resultFilter, setResultFilter] = useState<FinalResultType | 'all'>('all')
  const [expandedEvent, setExpandedEvent] = useState<string | null>(null)

  const filteredEvents = useMemo(() => {
    return events.filter(evt => {
      if (resultFilter !== 'all' && evt.finalResult !== resultFilter) return false
      if (search) {
        const searchLower = search.toLowerCase()
        return (
          evt.url.toLowerCase().includes(searchLower) ||
          evt.method.toLowerCase().includes(searchLower) ||
          evt.matchedRules.some(r => r.ruleName.toLowerCase().includes(searchLower))
        )
      }
      return true
    })
  }, [events, search, resultFilter])

  const resultCounts = useMemo(() => {
    const counts: Record<string, number> = { all: events.length }
    events.forEach(evt => {
      counts[evt.finalResult] = (counts[evt.finalResult] || 0) + 1
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

// ========== 未匹配事件列表 ==========
interface UnmatchedEventsListProps {
  events: UnmatchedEventWithId[]
  onClear?: () => void
}

function UnmatchedEventsList({ events, onClear }: UnmatchedEventsListProps) {
  const [search, setSearch] = useState('')
  const [expandedEvent, setExpandedEvent] = useState<string | null>(null)

  const filteredEvents = useMemo(() => {
    if (!search) return events
    const searchLower = search.toLowerCase()
    return events.filter(evt => 
      evt.url.toLowerCase().includes(searchLower) ||
      evt.method.toLowerCase().includes(searchLower)
    )
  }, [events, search])

  if (events.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center h-full text-muted-foreground">
        <div className="text-4xl mb-4 opacity-50">📡</div>
        <p>暂无未匹配请求</p>
        <p className="text-sm mt-1">未匹配任何规则的请求将在此显示</p>
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
            placeholder="搜索 URL、方法..."
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
            <UnmatchedEventItem
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

// ========== 匹配事件项 ==========
interface MatchedEventItemProps {
  event: MatchedEventWithId
  isExpanded: boolean
  onToggleExpand: () => void
}

function MatchedEventItem({ event, isExpanded, onToggleExpand }: MatchedEventItemProps) {
  const [copied, setCopied] = useState(false)
  const colors = FINAL_RESULT_COLORS[event.finalResult] || FINAL_RESULT_COLORS.passed

  const handleCopyUrl = async () => {
    await navigator.clipboard.writeText(event.url)
    setCopied(true)
    setTimeout(() => setCopied(false), 1500)
  }

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
          {FINAL_RESULT_LABELS[event.finalResult]}
        </Badge>

        {/* 阶段标签 */}
        <Badge variant="outline" className="text-xs">
          {event.stage === 'request' ? (
            <><ArrowRight className="w-3 h-3 mr-0.5" />REQ</>
          ) : (
            <><ArrowLeft className="w-3 h-3 mr-0.5" />RES</>
          )}
        </Badge>

        {/* Method */}
        <span className="font-mono text-xs font-medium px-1.5 py-0.5 rounded bg-muted">
          {event.method}
        </span>

        {/* URL */}
        <span className="flex-1 text-sm truncate text-muted-foreground font-mono">
          {event.url}
        </span>

        {/* 匹配规则数 */}
        <Badge variant="secondary" className="text-xs">
          {event.matchedRules.length} 规则
        </Badge>

        {/* 时间 */}
        <span className="text-xs text-muted-foreground shrink-0">
          {formatTime(event.timestamp)}
        </span>

        {isExpanded ? <ChevronUp className="w-4 h-4" /> : <ChevronDown className="w-4 h-4" />}
      </div>

      {/* 展开详情 */}
      {isExpanded && (
        <div className="border-t p-3 space-y-4 text-sm">
          {/* 基本信息 */}
          <div>
            <div className="font-medium mb-2 text-xs text-muted-foreground uppercase">基本信息</div>
            <div className="grid grid-cols-3 gap-2 text-xs">
              <div>
                <span className="text-muted-foreground">Target:</span>
                <span className="ml-2 font-mono">{event.target.slice(0, 16)}...</span>
              </div>
              {event.original?.resourceType && (
                <div>
                  <span className="text-muted-foreground">Type:</span>
                  <span className="ml-2 font-mono">{event.original.resourceType}</span>
                </div>
              )}
              {(event.statusCode || 0) > 0 && (
                <div>
                  <span className="text-muted-foreground">Status:</span>
                  <span className={`ml-2 font-mono ${
                    (event.statusCode || 0) >= 400 ? 'text-red-500' : 
                    (event.statusCode || 0) >= 300 ? 'text-yellow-500' : 'text-green-500'
                  }`}>
                    {event.statusCode}
                  </span>
                </div>
              )}
            </div>
          </div>

          {/* URL */}
          <div>
            <div className="flex items-center justify-between mb-1">
              <span className="font-medium text-xs text-muted-foreground uppercase">URL</span>
              <Button variant="ghost" size="sm" onClick={handleCopyUrl} className="h-6 px-2">
                {copied ? <Check className="w-3 h-3" /> : <Copy className="w-3 h-3" />}
              </Button>
            </div>
            <div className="p-2 bg-muted rounded font-mono text-xs break-all">
              {event.url}
            </div>
          </div>

          {/* 匹配的规则 */}
          {event.matchedRules && event.matchedRules.length > 0 && (
            <div>
              <div className="font-medium mb-2 text-xs text-muted-foreground uppercase">匹配规则</div>
              <div className="space-y-1">
                {event.matchedRules.map((rule, idx) => (
                  <div key={idx} className="p-2 bg-muted rounded text-xs flex items-center gap-2">
                    <span className="font-medium">{rule.ruleName || '未知规则'}</span>
                    <span className="text-muted-foreground">→</span>
                    <div className="flex gap-1 flex-wrap">
                      {(rule.actions || []).map((action, i) => (
                        <Badge key={i} variant="secondary" className="text-xs">{action}</Badge>
                      ))}
                    </div>
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* 请求/响应信息（根据 stage 分开展示） */}
          {event.stage === 'request' && event.original && event.modified && (
            <div>
              <div className="font-medium mb-2 text-xs text-muted-foreground uppercase">请求信息</div>
              <div className="grid grid-cols-2 gap-3">
                {/* 原始请求 */}
                <div className="space-y-2">
                  <div className="text-xs font-medium text-muted-foreground">原始</div>
                  
                  {/* URL */}
                  {event.original.url && (
                    <div>
                      <div className="text-xs text-muted-foreground mb-1">URL</div>
                      <div className="p-2 bg-muted rounded font-mono text-xs break-all max-h-16 overflow-auto">
                        {event.original.url}
                      </div>
                    </div>
                  )}

                  {/* Headers */}
                  <div>
                    <div className="text-xs text-muted-foreground mb-1">Headers</div>
                    <div className="p-2 bg-muted rounded font-mono text-xs max-h-32 overflow-auto">
                      {event.original.headers && Object.keys(event.original.headers).length > 0 ? (
                        Object.entries(event.original.headers).map(([key, value]) => (
                          <div key={key} className="truncate">
                            <span className="text-primary">{key}:</span> {value}
                          </div>
                        ))
                      ) : (
                        <span className="text-muted-foreground">（无）</span>
                      )}
                    </div>
                  </div>

                  {/* PostData/Body */}
                  {(event.original.postData || event.original.body) && (
                    <div>
                      <div className="text-xs text-muted-foreground mb-1">Body</div>
                      <div className="p-2 bg-muted rounded font-mono text-xs max-h-32 overflow-auto whitespace-pre-wrap">
                        {event.original.postData || event.original.body || <span className="text-muted-foreground">（空）</span>}
                      </div>
                    </div>
                  )}
                </div>

                {/* 修改后请求 */}
                <div className="space-y-2">
                  <div className="text-xs font-medium text-muted-foreground">修改后</div>
                  
                  {/* URL */}
                  {event.modified.url && (
                    <div>
                      <div className="text-xs text-muted-foreground mb-1">URL</div>
                      <div className="p-2 bg-muted rounded font-mono text-xs break-all max-h-16 overflow-auto">
                        {event.modified.url}
                      </div>
                    </div>
                  )}

                  {/* Headers */}
                  <div>
                    <div className="text-xs text-muted-foreground mb-1">Headers</div>
                    <div className="p-2 bg-muted rounded font-mono text-xs max-h-32 overflow-auto">
                      {event.modified.headers && Object.keys(event.modified.headers).length > 0 ? (
                        Object.entries(event.modified.headers).map(([key, value]) => (
                          <div key={key} className="truncate">
                            <span className="text-primary">{key}:</span> {value}
                          </div>
                        ))
                      ) : (
                        <span className="text-muted-foreground">（无）</span>
                      )}
                    </div>
                  </div>

                  {/* PostData/Body */}
                  {(event.modified.postData || event.modified.body) && (
                    <div>
                      <div className="text-xs text-muted-foreground mb-1">Body</div>
                      <div className="p-2 bg-muted rounded font-mono text-xs max-h-32 overflow-auto whitespace-pre-wrap">
                        {event.modified.postData || event.modified.body || <span className="text-muted-foreground">（空）</span>}
                      </div>
                    </div>
                  )}
                </div>
              </div>
            </div>
          )}

          {/* 响应信息 */}
          {event.stage === 'response' && event.original && event.modified && (
            <div>
              <div className="font-medium mb-2 text-xs text-muted-foreground uppercase">响应信息</div>
              <div className="grid grid-cols-2 gap-3">
                {/* 原始响应 */}
                <div className="space-y-2">
                  <div className="text-xs font-medium text-muted-foreground">原始</div>
                  
                  {/* Status Code */}
                  {(event.original.statusCode || 0) > 0 && (
                    <div>
                      <div className="text-xs text-muted-foreground mb-1">Status Code</div>
                      <div className="p-2 bg-muted rounded font-mono text-xs">
                        <span className={(event.original.statusCode || 0) >= 400 ? 'text-red-500' : (event.original.statusCode || 0) >= 300 ? 'text-yellow-500' : 'text-green-500'}>
                          {event.original.statusCode}
                        </span>
                      </div>
                    </div>
                  )}

                  {/* Headers */}
                  <div>
                    <div className="text-xs text-muted-foreground mb-1">Headers</div>
                    <div className="p-2 bg-muted rounded font-mono text-xs max-h-32 overflow-auto">
                      {event.original.headers && Object.keys(event.original.headers).length > 0 ? (
                        Object.entries(event.original.headers).map(([key, value]) => (
                          <div key={key} className="truncate">
                            <span className="text-primary">{key}:</span> {value}
                          </div>
                        ))
                      ) : (
                        <span className="text-muted-foreground">（无）</span>
                      )}
                    </div>
                  </div>

                  {/* Body */}
                  {event.original.body && (
                    <div>
                      <div className="text-xs text-muted-foreground mb-1">Body</div>
                      <div className="p-2 bg-muted rounded font-mono text-xs max-h-32 overflow-auto whitespace-pre-wrap">
                        {event.original.body || <span className="text-muted-foreground">（空）</span>}
                      </div>
                    </div>
                  )}
                </div>

                {/* 修改后响应 */}
                <div className="space-y-2">
                  <div className="text-xs font-medium text-muted-foreground">修改后</div>
                  
                  {/* Status Code */}
                  {(event.modified.statusCode || 0) > 0 && (
                    <div>
                      <div className="text-xs text-muted-foreground mb-1">Status Code</div>
                      <div className="p-2 bg-muted rounded font-mono text-xs">
                        <span className={(event.modified.statusCode || 0) >= 400 ? 'text-red-500' : (event.modified.statusCode || 0) >= 300 ? 'text-yellow-500' : 'text-green-500'}>
                          {event.modified.statusCode}
                        </span>
                      </div>
                    </div>
                  )}

                  {/* Headers */}
                  <div>
                    <div className="text-xs text-muted-foreground mb-1">Headers</div>
                    <div className="p-2 bg-muted rounded font-mono text-xs max-h-32 overflow-auto">
                      {event.modified.headers && Object.keys(event.modified.headers).length > 0 ? (
                        Object.entries(event.modified.headers).map(([key, value]) => (
                          <div key={key} className="truncate">
                            <span className="text-primary">{key}:</span> {value}
                          </div>
                        ))
                      ) : (
                        <span className="text-muted-foreground">（无）</span>
                      )}
                    </div>
                  </div>

                  {/* Body */}
                  {event.modified.body && (
                    <div>
                      <div className="text-xs text-muted-foreground mb-1">Body</div>
                      <div className="p-2 bg-muted rounded font-mono text-xs max-h-32 overflow-auto whitespace-pre-wrap">
                        {event.modified.body || <span className="text-muted-foreground">（空）</span>}
                      </div>
                    </div>
                  )}
                </div>
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  )
}

// ========== 未匹配事件项 ==========
interface UnmatchedEventItemProps {
  event: UnmatchedEventWithId
  isExpanded: boolean
  onToggleExpand: () => void
}

function UnmatchedEventItem({ event, isExpanded, onToggleExpand }: UnmatchedEventItemProps) {
  const [copied, setCopied] = useState(false)

  const handleCopyUrl = async () => {
    await navigator.clipboard.writeText(event.url)
    setCopied(true)
    setTimeout(() => setCopied(false), 1500)
  }

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
        {/* 未匹配标签 */}
        <Badge variant="outline" className={`${UNMATCHED_COLORS.bg} ${UNMATCHED_COLORS.text} border-0 text-xs`}>
          未匹配
        </Badge>

        {/* 阶段标签 */}
        <Badge variant="outline" className="text-xs">
          {event.stage === 'request' ? (
            <><ArrowRight className="w-3 h-3 mr-0.5" />REQ</>
          ) : (
            <><ArrowLeft className="w-3 h-3 mr-0.5" />RES</>
          )}
        </Badge>

        {/* Method */}
        <span className="font-mono text-xs font-medium px-1.5 py-0.5 rounded bg-muted">
          {event.method}
        </span>

        {/* URL */}
        <span className="flex-1 text-sm truncate text-muted-foreground font-mono">
          {event.url}
        </span>

        {/* Status Code (如果有) */}
        {event.statusCode && (
          <span className={`font-mono text-xs ${
            event.statusCode >= 400 ? 'text-red-500' : 
            event.statusCode >= 300 ? 'text-yellow-500' : 'text-green-500'
          }`}>
            {event.statusCode}
          </span>
        )}

        {/* 时间 */}
        <span className="text-xs text-muted-foreground shrink-0">
          {formatTime(event.timestamp)}
        </span>

        {isExpanded ? <ChevronUp className="w-4 h-4" /> : <ChevronDown className="w-4 h-4" />}
      </div>

      {/* 展开详情 */}
      {isExpanded && (
        <div className="border-t p-3 space-y-3 text-sm">
          {/* 基本信息 */}
          <div className="grid grid-cols-2 gap-2">
            <div>
              <span className="text-muted-foreground">Target:</span>
              <span className="ml-2 font-mono text-xs">{event.target.slice(0, 20)}...</span>
            </div>
            {event.statusCode && (
              <div>
                <span className="text-muted-foreground">Status:</span>
                <span className={`ml-2 font-mono ${
                  event.statusCode >= 400 ? 'text-red-500' : 
                  event.statusCode >= 300 ? 'text-yellow-500' : 'text-green-500'
                }`}>
                  {event.statusCode}
                </span>
              </div>
            )}
          </div>

          {/* URL */}
          <div className="space-y-1">
            <div className="flex items-center justify-between">
              <span className="text-muted-foreground">URL</span>
              <Button variant="ghost" size="sm" onClick={handleCopyUrl} className="h-6 px-2">
                {copied ? <Check className="w-3 h-3" /> : <Copy className="w-3 h-3" />}
              </Button>
            </div>
            <div className="p-2 bg-muted rounded font-mono text-xs break-all">
              {event.url}
            </div>
          </div>

          <div className="p-2 bg-slate-500/10 rounded text-xs text-muted-foreground">
            此请求未匹配任何规则，已直接放行
          </div>
        </div>
      )}
    </div>
  )
}
