import { useState, useMemo } from 'react'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { 
  Search, 
  X,
  Trash2,
  Activity,
  Play,
  Square
} from 'lucide-react'
import type { NetworkEvent, Response as TrafficResponse, Request as TrafficRequest } from '@/types/events'
import { useTranslation } from 'react-i18next'

interface NetworkPanelProps {
  events: NetworkEvent[]
  isCapturing: boolean
  onToggleCapture: (enabled: boolean) => void
  onClear?: () => void
  isConnected: boolean
}

export function NetworkPanel({ 
  events, 
  isCapturing, 
  onToggleCapture, 
  onClear,
  isConnected
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
            disabled={!isConnected}
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

        <div className="text-xs text-muted-foreground">
          {isCapturing && <span className="flex items-center gap-1.5 text-red-500 animate-pulse mr-2">
            <span className="w-1.5 h-1.5 rounded-full bg-red-500" /> {t('rules.running')}
          </span>}
          {t('rules.ruleCount', { count: events.length })}
        </div>
      </div>

      {/* 列表头部 */}
      <div className="grid grid-cols-[80px_80px_1fr_80px_100px] gap-2 px-3 py-2 border-b text-[11px] font-bold text-muted-foreground uppercase bg-muted/30">
        <div>Method</div>
        <div>Status</div>
        <div>URL</div>
        <div>Size</div>
        <div>Time</div>
      </div>

      <ScrollArea className="flex-1">
        <div className="divide-y border-b">
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
      </ScrollArea>
    </div>
  )
}

function TrafficItem({ event, isExpanded, onToggleExpand }: { event: NetworkEvent, isExpanded: boolean, onToggleExpand: () => void }) {
  const { request, response, timestamp } = event
  
  const statusColor = response ? (
    response.statusCode >= 400 ? 'text-red-500' : 
    response.statusCode >= 300 ? 'text-yellow-500' : 'text-green-500'
  ) : 'text-muted-foreground animate-pulse'

  const formatTime = (ts: number) => {
    return new Date(ts).toLocaleTimeString(undefined, { hour12: false })
  }

  return (
    <div className="group">
      <div 
        className={`grid grid-cols-[80px_80px_1fr_80px_100px] gap-2 px-3 py-2 cursor-pointer hover:bg-muted/50 transition-colors text-sm items-center ${isExpanded ? 'bg-muted/30' : ''}`}
        onClick={onToggleExpand}
      >
        <div className="font-mono text-xs font-bold">{request.method}</div>
        <div className={`font-mono text-xs font-bold ${statusColor}`}>
          {response?.statusCode || 'Pending'}
        </div>
        <div className="truncate font-mono text-xs text-muted-foreground group-hover:text-foreground">
          {request.url}
        </div>
        <div className="text-xs text-muted-foreground">
          {response?.body ? `${(response.body.length / 1024).toFixed(1)} KB` : '-'}
        </div>
        <div className="text-[11px] text-muted-foreground font-mono">
          {formatTime(timestamp)}
        </div>
      </div>
      {isExpanded && (
        <div className="p-4 bg-card border-t border-b">
          <TrafficDetailView request={request} response={response} />
        </div>
      )}
    </div>
  )
}

function TrafficDetailView({ request, response }: { request: TrafficRequest, response?: TrafficResponse }) {
  const { t } = useTranslation()
  
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

  return (
    <div className="space-y-6">
      <section>
        <h4 className="text-[11px] font-bold text-muted-foreground uppercase mb-2">{t('events.sections.request')}</h4>
        <div className="space-y-1 text-xs font-mono">
          <div className="flex gap-2">
            <span className="text-muted-foreground w-24 shrink-0">URL:</span>
            <span className="break-all">{request.url}</span>
          </div>
          <div className="flex gap-2 pt-2">
            <span className="text-muted-foreground w-24 shrink-0">{t('events.sections.request')}:</span>
            <div className="space-y-0.5 border-l-2 pl-3">
              {Object.entries(request.headers).map(([k, v]) => (
                <div key={k}><span className="text-primary font-bold">{k}:</span> {v}</div>
              ))}
            </div>
          </div>
          {request.body && (
            <div className="flex gap-2 pt-2">
              <span className="text-muted-foreground w-24 shrink-0">Payload:</span>
              <pre className="p-2 bg-muted rounded w-full overflow-auto max-h-40">{formattedRequestBody}</pre>
            </div>
          )}
        </div>
      </section>

      {response && (
        <section>
          <h4 className="text-[11px] font-bold text-muted-foreground uppercase mb-2">{t('events.sections.response')}</h4>
          <div className="space-y-1 text-xs font-mono">
            <div className="flex gap-2">
              <span className="text-muted-foreground w-24 shrink-0">Status Code:</span>
              <span className="font-bold">{response.statusCode}</span>
            </div>
            <div className="flex gap-2 pt-2">
              <span className="text-muted-foreground w-24 shrink-0">{t('events.sections.response')}:</span>
              <div className="space-y-0.5 border-l-2 pl-3">
                {Object.entries(response.headers).map(([k, v]) => (
                  <div key={k}><span className="text-primary font-bold">{k}:</span> {v}</div>
                ))}
              </div>
            </div>
          </div>
        </section>
      )}
    </div>
  )
}
