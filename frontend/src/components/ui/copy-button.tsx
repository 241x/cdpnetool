import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { Copy, Check } from 'lucide-react'
import { useTranslation } from 'react-i18next'

interface CopyButtonProps {
  content: string
}

export function CopyButton({ content }: CopyButtonProps) {
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
