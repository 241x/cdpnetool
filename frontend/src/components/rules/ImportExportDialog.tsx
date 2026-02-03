import { useRef } from 'react'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { 
  FileUp, 
  FileDown, 
  X 
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { model } from '@/../wailsjs/go/models'

interface ImportExportDialogProps {
  open: boolean
  onClose: () => void
  ruleSets: model.ConfigRecord[]
  onImport: (file: File) => void
  onExport: (config: model.ConfigRecord) => void
  getRuleCount: (config: model.ConfigRecord) => number
}

export function ImportExportDialog({ 
  open, 
  onClose, 
  ruleSets, 
  onImport, 
  onExport,
  getRuleCount
}: ImportExportDialogProps) {
  const { t } = useTranslation()
  const fileInputRef = useRef<HTMLInputElement>(null)

  if (!open) return null

  const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (file) {
      onImport(file)
      e.target.value = ''
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
      <input
        ref={fileInputRef}
        type="file"
        accept=".json"
        onChange={handleFileChange}
        className="hidden"
      />
      <div className="bg-background border rounded-lg shadow-lg w-full max-w-md mx-4 overflow-hidden flex flex-col h-[400px]">
        <div className="p-4 border-b flex items-center justify-between bg-muted/30">
          <h3 className="text-sm font-bold uppercase tracking-wider text-muted-foreground">
            {t('common.import')}/{t('common.export')}
          </h3>
          <Button 
            variant="ghost" 
            size="sm" 
            onClick={onClose} 
            className="h-8 w-8 p-0 hover:bg-destructive/10 hover:text-destructive transition-colors"
          >
            <X className="w-4 h-4" />
          </Button>
        </div>
        
        <Tabs defaultValue="import" className="flex-1 flex flex-col min-h-0">
          <TabsList className="flex w-full rounded-none border-b bg-muted/20 p-0 h-10">
            <TabsTrigger 
              value="import" 
              className="flex-1 rounded-none border-b-2 border-transparent data-[state=active]:border-primary data-[state=active]:bg-background data-[state=active]:shadow-none h-10"
            >
              {t('common.import')}
            </TabsTrigger>
            <TabsTrigger 
              value="export" 
              className="flex-1 rounded-none border-b-2 border-transparent data-[state=active]:border-primary data-[state=active]:bg-background data-[state=active]:shadow-none h-10"
            >
              {t('common.export')}
            </TabsTrigger>
          </TabsList>
          
          <TabsContent value="import" className="m-0 p-4 flex-1 flex flex-col data-[state=inactive]:hidden">
            <div 
              className="flex-1 w-full border-2 border-dashed rounded-xl flex flex-col items-center justify-center gap-3 cursor-pointer hover:border-primary hover:bg-primary/5 transition-all group"
              onClick={() => fileInputRef.current?.click()}
            >
              <FileUp className="w-10 h-10 text-muted-foreground group-hover:text-primary group-hover:scale-110 transition-all" />
              <div className="text-center">
                <p className="text-sm font-semibold">{t('rules.importPlaceholder')}</p>
                <p className="text-xs text-muted-foreground mt-1">{t('rules.importDesc')}</p>
              </div>
            </div>
          </TabsContent>
          
          <TabsContent value="export" className="m-0 flex-1 flex flex-col min-h-0 data-[state=inactive]:hidden">
            <ScrollArea className="flex-1 p-4">
              <div className="space-y-2">
                {ruleSets.map(config => (
                  <div key={config.id} className="flex items-center justify-between p-2 rounded-lg border bg-card hover:bg-muted/30 transition-colors group">
                    <div className="min-w-0 flex-1 mr-4 pl-1">
                      <div className="text-sm font-medium truncate group-hover:text-primary transition-colors">{config.name}</div>
                      <div className="text-[10px] text-muted-foreground uppercase">{t('rules.ruleCount', { count: getRuleCount(config) })}</div>
                    </div>
                    <Button 
                      size="sm" 
                      variant="outline" 
                      className="h-7 w-7 p-0 shrink-0 hover:bg-primary hover:text-primary-foreground" 
                      onClick={() => onExport(config)}
                    >
                      <FileDown className="w-4 h-4" />
                    </Button>
                  </div>
                ))}
                {ruleSets.length === 0 && (
                  <div className="py-20 text-center text-muted-foreground text-sm italic">
                    {t('rules.noConfigs')}
                  </div>
                )}
              </div>
            </ScrollArea>
          </TabsContent>
        </Tabs>
        
        <div className="p-4 border-t bg-muted/5 flex justify-end">
          <Button variant="outline" size="sm" onClick={onClose}>
            {t('common.cancel')}
          </Button>
        </div>
      </div>
    </div>
  )
}
