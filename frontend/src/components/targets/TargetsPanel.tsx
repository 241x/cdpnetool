import { Button } from '@/components/ui/button'
import { domain } from '@/../wailsjs/go/models'
import { useTranslation } from 'react-i18next'
import { RefreshCw } from 'lucide-react'

interface TargetsPanelProps {
  targets: domain.TargetInfo[]
  attachedTargetId: string | null
  onToggle: (id: string) => void
  isConnected: boolean
  onRefresh: () => void
}

export function TargetsPanel({ 
  targets, 
  attachedTargetId, 
  onToggle,
  isConnected,
  onRefresh
}: TargetsPanelProps) {
  const { t } = useTranslation()

  const formatUrl = (url: string) => {
    try {
      const urlObj = new URL(url)
      const pathAndQuery = urlObj.pathname + urlObj.search + urlObj.hash
      const displayPath = pathAndQuery.length > 50 ? pathAndQuery.substring(0, 50) + '...' : pathAndQuery
      return {
        host: urlObj.host,
        path: displayPath || '/'
      }
    } catch {
      return {
        host: url.substring(0, 30),
        path: url.length > 30 ? '...' + url.substring(30, 60) : ''
      }
    }
  }

  if (!isConnected) {
    return (
      <div className="flex items-center justify-center h-full text-muted-foreground">
        {t('targets.connectFirst')}
      </div>
    )
  }

  if (targets.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center h-full gap-4">
        <div className="text-muted-foreground">{t('targets.noTargets')}</div>
        <Button variant="outline" onClick={onRefresh}>
          <RefreshCw className="w-4 h-4 mr-2" />
          {t('toolbar.refreshTargets')}
        </Button>
      </div>
    )
  }

  return (
    <div className="space-y-3">
      <div className="flex justify-start">
        <Button variant="outline" size="sm" onClick={onRefresh}>
          <RefreshCw className="w-4 h-4 mr-2" />
          {t('toolbar.refreshTargets')}
        </Button>
      </div>
      <div className="space-y-2">
        {targets.map((target) => {
          const urlObj = new URL(target.url)
          const faviconUrl = `${urlObj.protocol}//${urlObj.host}/favicon.ico`
          const { host, path } = formatUrl(target.url)
          
          return (
            <div 
              key={target.id}
              className="flex items-center gap-3 p-3 rounded-lg border hover:bg-muted/50 transition-colors"
            >
              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-2">
                  <div className="font-medium truncate">{target.title || t('targets.untitled')}</div>
                  <img 
                    src={faviconUrl} 
                    alt="" 
                    className="w-4 h-4 flex-shrink-0"
                    onError={(e) => {
                      e.currentTarget.style.display = 'none'
                    }}
                  />
                </div>
                <div className="text-xs text-muted-foreground" title={target.url}>
                  <span className="font-medium">{host}</span>
                  <span className="text-muted-foreground/70">{path}</span>
                </div>
              </div>
              <Button
                variant={attachedTargetId === target.id ? "default" : "outline"}
                size="sm"
                onClick={() => onToggle(target.id)}
              >
                {attachedTargetId === target.id ? t('targets.attached') : t('targets.attach')}
              </Button>
            </div>
          )
        })}
      </div>
    </div>
  )
}
