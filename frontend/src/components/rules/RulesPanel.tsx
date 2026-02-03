import { useState, useEffect, useCallback } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Switch } from '@/components/ui/switch'
import { useToast } from '@/hooks/use-toast'
import { useSessionStore } from '@/stores'
import { RuleListEditor } from './RuleEditor'
import { ImportExportDialog } from './ImportExportDialog'
import { useTranslation } from 'react-i18next'
import { 
  FileJson, 
  Plus, 
  Save, 
  Trash2, 
  ChevronDown, 
  ChevronRight,
} from 'lucide-react'
import type { Rule, Config } from '@/types/rules'
import { createEmptyConfig } from '@/types/rules'
import { api } from '@/api'
import { model } from '@/../wailsjs/go/models'

interface RulesPanelProps {
  sessionId: string | null
  isConnected: boolean
  attachedTargetId: string | null
  setIntercepting: (intercepting: boolean) => void
}

export function RulesPanel({ sessionId, isConnected, attachedTargetId, setIntercepting }: RulesPanelProps) {
  const { t } = useTranslation()
  const { toast } = useToast()
  const { activeConfigId, setActiveConfigId } = useSessionStore()
  const [ruleSet, setRuleSet] = useState<Config>(createEmptyConfig())
  const [showJson, setShowJson] = useState(false)
  const [showImportExport, setShowImportExport] = useState(false)
  const [ruleSets, setRuleSets] = useState<model.ConfigRecord[]>([])
  const [currentRuleSetId, setCurrentRuleSetId] = useState<number>(0)
  const [currentRuleSetName, setCurrentRuleSetName] = useState<string>(t('rules.newConfig'))
  const [isLoading, setIsLoading] = useState(false)
  const [editingName, setEditingName] = useState<number | null>(null)
  const [newName, setNewName] = useState('')
  const [isInitializing, setIsInitializing] = useState(true)
  const [isDirty, setIsDirty] = useState(false)
  const [configInfoExpanded, setConfigInfoExpanded] = useState(false)
  const [jsonEditorContent, setJsonEditorContent] = useState('')
  const [jsonError, setJsonError] = useState<string | null>(null)
  const [confirmDialog, setConfirmDialog] = useState<{
    show: boolean
    title: string
    message: string
    onConfirm: () => void
    onSave?: () => Promise<void>
    confirmText?: string
    showSaveOption?: boolean
  } | null>(null)

  useEffect(() => {
    loadRuleSets()
      .catch(e => {
        console.error('Failed to load rule sets on mount:', e)
        setRuleSet(createEmptyConfig())
      })
      .finally(() => {
        setIsInitializing(false)
      })
  }, [])

  const loadRuleSets = async () => {
    try {
      const result = await api.config.list()
      if (result?.success && result.data) {
        const configs = result.data.configs || []
        setRuleSets(configs)
        if (configs.length > 0) {
          loadRuleSetData(configs[0])
        } else {
          setRuleSet(createEmptyConfig())
        }
      }
    } catch (e) {
      console.error('Load rule sets error:', e)
      setRuleSet(createEmptyConfig())
    }
  }

  const updateDirty = (dirty: boolean) => {
    setIsDirty(dirty)
    api.config.setDirty(dirty)
  }

  const updateJsonEditorFromRuleSet = useCallback((config: Config) => {
    setJsonEditorContent(JSON.stringify(config, null, 2))
    setJsonError(null)
  }, [])

  const handleRulesChange = (rules: Rule[]) => {
    const newConfig = { ...ruleSet, rules }
    setRuleSet(newConfig)
    updateDirty(true)
  }

  // 同步 ruleSet 到 JSON 编辑器（仅在显示 JSON 时）
  useEffect(() => {
    if (showJson) {
      updateJsonEditorFromRuleSet(ruleSet)
    }
  }, [ruleSet, showJson, updateJsonEditorFromRuleSet])

  const loadRuleSetData = (record: model.ConfigRecord) => {
    try {
      let config: Config
      if (!record.configJson) {
        config = {
          id: record.configId,
          name: record.name,
          version: record.version || '1.0',
          description: '',
          settings: {},
          rules: []
        }
      } else {
        config = JSON.parse(record.configJson) as Config
      }
      setRuleSet(config)
      setCurrentRuleSetId(record.id)
      setCurrentRuleSetName(config.name || record.name)
      setJsonEditorContent(JSON.stringify(config, null, 2))
      setJsonError(null)
      updateDirty(false)
    } catch (e) {
      console.error('Parse config error:', e)
      const emptyConfig = createEmptyConfig()
      setRuleSet(emptyConfig)
      setJsonEditorContent(JSON.stringify(emptyConfig, null, 2))
      setJsonError(null)
      updateDirty(false)
    }
  }

  const handleSelectRuleSet = async (record: model.ConfigRecord) => {
    if (isDirty) {
      setConfirmDialog({
        show: true,
        title: t('rules.unsavedChanges'),
        message: t('rules.unsavedChanges'), // Simplified for now
        confirmText: t('common.confirm'),
        showSaveOption: true,
        onConfirm: () => {
          loadRuleSetData(record)
          api.config.setActive(record.id)
          toast({ variant: 'success', title: `${t('common.refresh')}: ${record.name}` })
          setConfirmDialog(null)
        },
        onSave: async () => {
          await handleSave()
          loadRuleSetData(record)
          await api.config.setActive(record.id)
          toast({ variant: 'success', title: `${t('common.refresh')}: ${record.name}` })
          setConfirmDialog(null)
        }
      })
      return
    }
    loadRuleSetData(record)
    await api.config.setActive(record.id)
    toast({ variant: 'success', title: `${t('common.refresh')}: ${record.name}` })
  }

  const handleCreateRuleSet = async () => {
    try {
      const result = await api.config.create(t('rules.newConfig'))
      if (result?.success && result.data && result.data.config) {
        await loadRuleSets()
        const newConfig = JSON.parse(result.data.configJson) as Config
        setRuleSet(newConfig)
        setCurrentRuleSetId(result.data.config.id)
        setCurrentRuleSetName(result.data.config.name)
        setJsonEditorContent(result.data.configJson)
        setJsonError(null)
        await api.config.setActive(result.data.config.id)
        updateDirty(false)
        toast({ variant: 'success', title: t('common.add') + ' Success' })
      } else {
        toast({ variant: 'destructive', title: t('common.add') + ' Error', description: result?.message })
      }
    } catch (e) {
      toast({ variant: 'destructive', title: t('common.add') + ' Error', description: String(e) })
    }
  }

  const handleDeleteCurrentConfig = async () => {
    setConfirmDialog({
      show: true,
      title: t('common.delete'),
      message: `${t('common.delete')} 「${currentRuleSetName}」?`,
      onConfirm: async () => {
        await handleDeleteConfig(currentRuleSetId)
        setConfirmDialog(null)
      }
    })
  }

  const handleDeleteConfig = async (id: number) => {
    try {
      const result = await api.config.delete(id)
      if (result?.success) {
        await loadRuleSets()
        if (id === currentRuleSetId) {
          const remaining = ruleSets.filter(r => r.id !== id)
          if (remaining.length > 0) {
            loadRuleSetData(remaining[0])
            await api.config.setActive(remaining[0].id)
          } else {
            setRuleSet(createEmptyConfig())
            setCurrentRuleSetId(0)
            setCurrentRuleSetName('')
            setActiveConfigId(null)
            updateDirty(false)
          }
        }
        toast({ variant: 'success', title: t('common.delete') + ' Success' })
      } else {
        toast({ variant: 'destructive', title: t('common.delete') + ' Error', description: result?.message })
      }
    } catch (e) {
      toast({ variant: 'destructive', title: t('common.delete') + ' Error', description: String(e) })
    }
  }

  const handleRenameConfig = async (id: number) => {
    if (!newName.trim()) return
    try {
      const result = await api.config.rename(id, newName.trim())
      if (result?.success) {
        await loadRuleSets()
        if (id === currentRuleSetId) {
          setCurrentRuleSetName(newName.trim())
        }
        setEditingName(null)
        setNewName('')
        toast({ variant: 'success', title: t('common.rename') + ' Success' })
      }
    } catch (e) {
      toast({ variant: 'destructive', title: t('common.rename') + ' Error', description: String(e) })
    }
  }

  const handleToggleConfig = async (config: model.ConfigRecord, enabled: boolean) => {
    if (enabled) {
      if (!isConnected) {
        toast({ variant: 'destructive', title: t('targets.connectFirst') })
        return
      }
      if (!attachedTargetId) {
        toast({ variant: 'destructive', title: t('targets.title') + ' Required' })
        return
      }
      
      try {
        const configJson = config.configJson || JSON.stringify({ version: '1.0', rules: [] })
        const loadResult = await api.session.loadRules(sessionId!, configJson)
        if (!loadResult?.success) {
          toast({ variant: 'destructive', title: 'Error', description: loadResult?.message })
          return
        }
        
        const enableResult = await api.session.enableInterception(sessionId!)
        if (!enableResult?.success) {
          toast({ variant: 'destructive', title: 'Error', description: enableResult?.message })
          return
        }
        
        await api.config.setActive(config.id)
        setActiveConfigId(config.id)
        setIntercepting(true)
        await loadRuleSets()
        toast({ variant: 'success', title: `Config 「${config.name}」 ${t('rules.running')}` })
      } catch (e) {
        toast({ variant: 'destructive', title: 'Error', description: String(e) })
      }
    } else {
      try {
        if (sessionId) {
          await api.session.disableInterception(sessionId)
        }
        setActiveConfigId(null)
        setIntercepting(false)
        toast({ variant: 'success', title: 'Stopped' })
      } catch (e) {
        toast({ variant: 'destructive', title: 'Error', description: String(e) })
      }
    }
  }

  const getRuleCount = (config: model.ConfigRecord) => {
    try {
      if (!config.configJson) return 0
      const parsed = JSON.parse(config.configJson)
      return parsed.rules?.length || 0
    } catch {
      return 0
    }
  }

  const handleAddRule = async () => {
    try {
      const result = await api.config.generateRule(t('rules.newRule') || 'New Rule', ruleSet.rules.length)
      if (result?.success && result.data) {
        const newRule = JSON.parse(result.data.ruleJson) as Rule
        setRuleSet({
          ...ruleSet,
          rules: [...ruleSet.rules, newRule]
        })
        updateDirty(true)
      } else {
        toast({ variant: 'destructive', title: 'Error', description: result?.message })
      }
    } catch (e) {
      const fallbackRule: Rule = {
        id: crypto.randomUUID(),
        name: 'New Rule',
        enabled: true,
        priority: 0,
        stage: 'request',
        match: {},
        actions: []
      }
      setRuleSet({
        ...ruleSet,
        rules: [...ruleSet.rules, fallbackRule]
      })
      updateDirty(true)
    }
  }

  const handleSave = useCallback(async () => {
    if (showJson && jsonError) {
      toast({ variant: 'destructive', title: 'Error', description: 'JSON Error' })
      return
    }
    
    setIsLoading(true)
    try {
      const configToSave = {
        ...ruleSet,
        name: currentRuleSetName
      }
      const configJson = JSON.stringify(configToSave)
      const saveResult = await api.config.save(currentRuleSetId, configJson)
      
      if (!saveResult?.success) {
        toast({ variant: 'destructive', title: 'Error', description: saveResult?.message })
        return
      }
      
      if (saveResult.data && saveResult.data.config) {
        setCurrentRuleSetId(saveResult.data.config.id)
      }
      
      updateDirty(false)
      await loadRuleSets()
      
      if (currentRuleSetId === activeConfigId && sessionId) {
        await api.session.loadRules(sessionId, configJson)
        toast({ variant: 'success', title: `Saved & Updated` })
      } else {
        toast({ variant: 'success', title: t('common.save') + ' Success' })
      }
    } catch (e) {
      toast({ variant: 'destructive', title: 'Error', description: String(e) })
    } finally {
      setIsLoading(false)
    }
  }, [showJson, jsonError, ruleSet, currentRuleSetName, currentRuleSetId, activeConfigId, sessionId, toast, t])

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key === 's') {
        e.preventDefault()
        handleSave()
      }
    }
    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [handleSave])

  const handleExportConfig = async (config: model.ConfigRecord) => {
    try {
      const result = await api.config.export(config.name || "ruleset", config.configJson || "{}")
      if (result && !result.success) {
        toast({ variant: 'destructive', title: 'Error', description: result.message })
      } else if (result && result.success) {
        toast({ variant: 'success', title: t('common.export') + ' Success' })
      }
    } catch (e) {
      toast({ variant: 'destructive', title: 'Error', description: String(e) })
    }
  }

  const handleImportFile = (file: File) => {
    const reader = new FileReader()
    reader.onload = (event) => {
      try {
        const json = event.target?.result as string
        const imported = JSON.parse(json) as Config
        if (imported.version && Array.isArray(imported.rules)) {
          setRuleSet(imported)
          updateDirty(true)
          toast({ variant: 'success', title: 'Success' })
          setShowImportExport(false)
        } else {
          toast({ variant: 'destructive', title: 'Error' })
        }
      } catch {
        toast({ variant: 'destructive', title: 'Error' })
      }
    }
    reader.readAsText(file)
  }

  return (
    <div className="flex-1 flex min-h-0 h-full">
      {isInitializing ? (
        <div className="flex items-center justify-center w-full text-muted-foreground">
          <div className="text-center">
            <div className="text-lg mb-2">{t('common.loading')}</div>
          </div>
        </div>
      ) : (
        <>
          <div className="w-60 border-r flex flex-col shrink-0">
            <div className="p-3 border-b flex items-center justify-between">
              <span className="font-medium text-sm shrink-0">{t('rules.listTitle')}</span>
              <div className="flex items-center gap-0.5">
                <Button size="sm" variant="ghost" onClick={handleCreateRuleSet} title={t('common.add')} className="h-8 w-8 p-0">
                  <Plus className="w-4 h-4" />
                </Button>
                <Button 
                  size="sm" 
                  variant="ghost" 
                  onClick={() => setShowImportExport(true)} 
                  className="h-8 px-2 text-xs font-medium"
                >
                  {t('common.import')}/{t('common.export')}
                </Button>
              </div>
            </div>
            <ScrollArea className="flex-1">
              <div className="p-2 space-y-1">
                {ruleSets.map((config) => (
                  <div
                    key={config.id}
                    className={`flex items-center gap-2 p-2 rounded-md cursor-pointer transition-colors ${
                      config.id === currentRuleSetId 
                        ? 'bg-primary/10 border border-primary/30' 
                        : 'hover:bg-muted'
                    }`}
                    onClick={() => handleSelectRuleSet(config)}
                  >
                    <Switch
                      checked={config.id === activeConfigId}
                      onCheckedChange={(checked) => handleToggleConfig(config, checked)}
                      disabled={!isConnected && config.id !== activeConfigId}
                    />
                    <div className="flex-1 min-w-0">
                      {editingName === config.id ? (
                        <Input
                          value={newName}
                          onChange={(e) => setNewName(e.target.value)}
                          className="h-6 text-sm"
                          autoFocus
                          onClick={(e) => e.stopPropagation()}
                          onKeyDown={(e) => {
                            if (e.key === 'Enter') handleRenameConfig(config.id)
                            if (e.key === 'Escape') { setEditingName(null); setNewName('') }
                          }}
                          onBlur={() => { setEditingName(null); setNewName('') }}
                        />
                      ) : (
                        <>
                          <div className="text-sm font-medium truncate">{config.name}</div>
                          <div className="text-xs text-muted-foreground">
                            {t('rules.ruleCount', { count: getRuleCount(config) })}
                            {config.id === activeConfigId && <span className="ml-1 text-green-500">· {t('rules.running')}</span>}
                          </div>
                        </>
                      )}
                    </div>
                  </div>
                ))}
              </div>
            </ScrollArea>
          </div>

          <div className="flex-1 flex flex-col min-h-0 p-4">
            {ruleSets.length === 0 ? (
              <div className="flex-1 flex items-center justify-center text-muted-foreground">
                <div className="text-center">
                  <div className="text-lg mb-2">{t('rules.noConfigs')}</div>
                  <div className="text-sm mb-4">{t('rules.createFirst')}</div>
                </div>
              </div>
            ) : (
              <>
                <div className="mb-4 pb-3 border-b shrink-0">
                  <div className="flex items-center gap-2">
                    <button
                      onClick={() => setConfigInfoExpanded(!configInfoExpanded)}
                      className="flex items-center gap-1 text-sm font-medium hover:text-primary transition-colors"
                    >
                      {configInfoExpanded ? <ChevronDown className="w-4 h-4" /> : <ChevronRight className="w-4 h-4" />}
                      <span className="truncate max-w-48">{currentRuleSetName}</span>
                    </button>
                    {isDirty && <span className="w-2 h-2 rounded-full bg-primary animate-pulse" title={t('rules.unsavedChanges')} />}
                    <div className="flex-1" />
                    <Button size="sm" onClick={handleSave} disabled={isLoading}>
                      <Save className="w-4 h-4 mr-1" />
                      {isLoading ? t('common.saving') : t('common.save')}
                    </Button>
                    <Button variant="destructive" size="sm" onClick={handleDeleteCurrentConfig}>
                      <Trash2 className="w-4 h-4 mr-1" />
                      {t('common.delete')}
                    </Button>
                  </div>
                  
                  {configInfoExpanded && (
                    <div className="mt-3 space-y-3 pl-5">
                      <div className="flex items-center gap-2">
                        <span className="text-sm text-muted-foreground whitespace-nowrap w-16">Name:</span>
                        <Input
                          value={currentRuleSetName}
                          onChange={(e) => {
                            setCurrentRuleSetName(e.target.value)
                            updateDirty(true)
                          }}
                          className="flex-1 h-8 max-w-xs"
                        />
                      </div>
                      <div className="flex items-start gap-2">
                        <span className="text-sm text-muted-foreground whitespace-nowrap w-16 pt-2">Desc:</span>
                        <Textarea
                          value={ruleSet.description || ''}
                          onChange={(e) => {
                            setRuleSet({ ...ruleSet, description: e.target.value })
                            updateDirty(true)
                          }}
                          placeholder="Description"
                          className="flex-1 min-h-[60px] max-w-md"
                        />
                      </div>
                    </div>
                  )}
                </div>

                <div className="flex items-center gap-2 mb-4 shrink-0">
                  <Button onClick={handleAddRule} size="sm">
                    <Plus className="w-4 h-4 mr-1" />
                    {t('rules.addRule')}
                  </Button>
                  <Button variant="outline" size="sm" onClick={() => {
                    if (!showJson) {
                      setJsonEditorContent(JSON.stringify(ruleSet, null, 2))
                      setJsonError(null)
                    }
                    setShowJson(!showJson)
                  }}>
                    <FileJson className="w-4 h-4 mr-1" />
                    {showJson ? t('rules.visual') : t('rules.json')}
                  </Button>
                  <div className="flex-1" />
                  <span className="text-xs text-muted-foreground">
                    {t('rules.ruleCount', { count: ruleSet.rules.length })}
                  </span>
                </div>

                <div className="flex-1 min-h-0 flex flex-col">
                  {showJson ? (
                    <div className="flex-1 min-h-0 flex flex-col">
                      <Textarea
                        value={jsonEditorContent}
                        onChange={(e) => {
                          setJsonEditorContent(e.target.value)
                          try {
                            const parsed = JSON.parse(e.target.value)
                            if (parsed.rules && Array.isArray(parsed.rules)) {
                              setRuleSet(parsed)
                              setJsonError(null)
                            } else {
                              setJsonError('Format error')
                            }
                          } catch (err) {
                            setJsonError('JSON Error')
                          }
                          updateDirty(true)
                        }}
                        className={`flex-1 font-mono text-sm resize-none focus-visible:ring-1 ${
                          jsonError ? 'border-destructive' : ''
                        }`}
                        spellCheck={false}
                      />
                      {jsonError && (
                        <div className="p-2 text-sm text-destructive bg-destructive/10 rounded-md shrink-0">
                          {jsonError}
                        </div>
                      )}
                    </div>
                  ) : (
                    <ScrollArea className="flex-1 pr-4">
                      <RuleListEditor
                        rules={ruleSet.rules}
                        onChange={handleRulesChange}
                      />
                    </ScrollArea>
                  )}
                </div>
              </>
            )}
          </div>
        </>
      )}

      {confirmDialog?.show && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
          <div className="bg-background border rounded-lg shadow-lg p-6 max-w-md w-full mx-4">
            <h3 className="text-lg font-semibold mb-2">{confirmDialog.title}</h3>
            <p className="text-muted-foreground mb-6">{confirmDialog.message}</p>
            <div className="flex justify-end gap-2">
              <Button variant="outline" onClick={() => setConfirmDialog(null)}>
                {t('common.cancel')}
              </Button>
              {confirmDialog.showSaveOption && confirmDialog.onSave && (
                <Button variant="default" onClick={confirmDialog.onSave}>
                  {t('common.save')}
                </Button>
              )}
              <Button variant="destructive" onClick={confirmDialog.onConfirm}>
                {confirmDialog.confirmText || t('common.confirm')}
              </Button>
            </div>
          </div>
        </div>
      )}

      <ImportExportDialog
        open={showImportExport}
        onClose={() => setShowImportExport(false)}
        ruleSets={ruleSets}
        onImport={handleImportFile}
        onExport={handleExportConfig}
        getRuleCount={getRuleCount}
      />
    </div>
  )
}
