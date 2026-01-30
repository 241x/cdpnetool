// 规则配置类型定义 (v2)
// 与后端 pkg/rulespec/types.go 保持一致

export type RuleID = string

// 生命周期阶段
export type Stage = 'request' | 'response'

// V2 细粒度条件类型（25种）
export type ConditionType =
  // URL 条件
  | 'urlEquals'
  | 'urlPrefix'
  | 'urlSuffix'
  | 'urlContains'
  | 'urlRegex'
  // Method 和 ResourceType
  | 'method'
  | 'resourceType'
  // Header 条件
  | 'headerExists'
  | 'headerNotExists'
  | 'headerEquals'
  | 'headerContains'
  | 'headerRegex'
  // Query 条件
  | 'queryExists'
  | 'queryNotExists'
  | 'queryEquals'
  | 'queryContains'
  | 'queryRegex'
  // Cookie 条件
  | 'cookieExists'
  | 'cookieNotExists'
  | 'cookieEquals'
  | 'cookieContains'
  | 'cookieRegex'
  // Body 条件
  | 'bodyContains'
  | 'bodyRegex'
  | 'bodyJsonPath'

// 条件定义
export interface Condition {
  type: ConditionType
  value?: string         // urlEquals, urlPrefix, urlSuffix, urlContains, *Equals, *Contains, bodyContains
  values?: string[]      // method, resourceType
  pattern?: string       // urlRegex, *Regex
  name?: string          // header*, query*, cookie*
  path?: string          // bodyJsonPath
}

export interface Match {
  allOf?: Condition[]    // AND 逻辑
  anyOf?: Condition[]    // OR 逻辑
}

// V2 细粒度行为类型（15种）
export type ActionType =
  // 请求阶段专用
  | 'setUrl'
  | 'setMethod'
  | 'setQueryParam'
  | 'removeQueryParam'
  | 'setCookie'
  | 'removeCookie'
  | 'setFormField'
  | 'removeFormField'
  | 'block'
  // 响应阶段专用
  | 'setStatus'
  // 通用
  | 'setHeader'
  | 'removeHeader'
  | 'setBody'
  | 'appendBody'
  | 'replaceBodyText'
  | 'patchBodyJson'

// Body 编码方式
export type BodyEncoding = 'text' | 'base64'

// JSON Patch 操作
export interface JSONPatchOp {
  op: 'add' | 'remove' | 'replace' | 'move' | 'copy' | 'test'
  path: string
  value?: any
  from?: string
}

// 行为定义
export interface Action {
  type: ActionType
  value?: string | number       // setUrl, setMethod, setStatus, setBody, setHeader, setQueryParam, setCookie, setFormField
  name?: string                 // setHeader, removeHeader, setQueryParam, removeQueryParam, setCookie, removeCookie, setFormField, removeFormField
  encoding?: BodyEncoding       // setBody
  search?: string               // replaceBodyText
  replace?: string              // replaceBodyText
  replaceAll?: boolean          // replaceBodyText
  patches?: JSONPatchOp[]       // patchBodyJson
  statusCode?: number           // block
  headers?: Record<string, string>  // block
  body?: string                 // block
  bodyEncoding?: BodyEncoding   // block
}

export interface Rule {
  id: RuleID
  name: string
  enabled: boolean
  priority: number
  stage: Stage
  match: Match
  actions: Action[]
}

// 配置版本常量
export const DEFAULT_CONFIG_VERSION = '1.0'

// 配置结构（完整版）
export interface Config {
  id?: string                     // 配置 ID（新建时可省略）
  name?: string                   // 配置名称（在 UI 中单独管理）
  version: string                 // 配置格式版本
  description: string             // 配置描述
  settings: Record<string, any>   // 预留设置项
  rules: Rule[]                   // 规则列表
}

// 创建空配置（完整字段）
export function createEmptyConfig(): Config {
  return {
    version: DEFAULT_CONFIG_VERSION,
    description: '',
    settings: {},
    rules: []
  }
}

export const RESOURCE_TYPES = [
  'document',
  'script',
  'stylesheet',
  'image',
  'media',
  'font',
  'xhr',
  'fetch',
  'websocket',
  'other'
] as const

export type ResourceType = typeof RESOURCE_TYPES[number]

// HTTP 方法常量
export const HTTP_METHODS = ['GET', 'POST', 'PUT', 'DELETE', 'PATCH', 'HEAD', 'OPTIONS'] as const

export const CONDITION_GROUPS = {
  url: ['urlEquals', 'urlPrefix', 'urlSuffix', 'urlContains', 'urlRegex'],
  method: ['method'],
  resourceType: ['resourceType'],
  header: ['headerExists', 'headerNotExists', 'headerEquals', 'headerContains', 'headerRegex'],
  query: ['queryExists', 'queryNotExists', 'queryEquals', 'queryContains', 'queryRegex'],
  cookie: ['cookieExists', 'cookieNotExists', 'cookieEquals', 'cookieContains', 'cookieRegex'],
  body: ['bodyContains', 'bodyRegex', 'bodyJsonPath']
} as const

// 条件类型标签
export const CONDITION_TYPE_LABELS: Record<ConditionType, string> = {
  urlEquals: 'URL 精确匹配',
  urlPrefix: 'URL 前缀匹配',
  urlSuffix: 'URL 后缀匹配',
  urlContains: 'URL 包含',
  urlRegex: 'URL 正则匹配',
  method: 'HTTP 方法',
  resourceType: '资源类型',
  headerExists: 'Header 存在',
  headerNotExists: 'Header 不存在',
  headerEquals: 'Header 精确匹配',
  headerContains: 'Header 包含',
  headerRegex: 'Header 正则匹配',
  queryExists: 'Query 参数存在',
  queryNotExists: 'Query 参数不存在',
  queryEquals: 'Query 参数精确匹配',
  queryContains: 'Query 参数包含',
  queryRegex: 'Query 参数正则匹配',
  cookieExists: 'Cookie 存在',
  cookieNotExists: 'Cookie 不存在',
  cookieEquals: 'Cookie 精确匹配',
  cookieContains: 'Cookie 包含',
  cookieRegex: 'Cookie 正则匹配',
  bodyContains: 'Body 包含',
  bodyRegex: 'Body 正则匹配',
  bodyJsonPath: 'JSON Path 匹配'
}

// 条件类型简短标签（用于选择器）
export const CONDITION_TYPE_SHORT_LABELS: Record<ConditionType, string> = {
  urlEquals: 'URL =',
  urlPrefix: 'URL 前缀',
  urlSuffix: 'URL 后缀',
  urlContains: 'URL 含',
  urlRegex: 'URL 正则',
  method: '方法',
  resourceType: '资源类型',
  headerExists: 'Header 存在',
  headerNotExists: 'Header 不存在',
  headerEquals: 'Header =',
  headerContains: 'Header 含',
  headerRegex: 'Header 正则',
  queryExists: 'Query 存在',
  queryNotExists: 'Query 不存在',
  queryEquals: 'Query =',
  queryContains: 'Query 含',
  queryRegex: 'Query 正则',
  cookieExists: 'Cookie 存在',
  cookieNotExists: 'Cookie 不存在',
  cookieEquals: 'Cookie =',
  cookieContains: 'Cookie 含',
  cookieRegex: 'Cookie 正则',
  bodyContains: 'Body 含',
  bodyRegex: 'Body 正则',
  bodyJsonPath: 'JSON Path'
}

// 请求阶段可用行为
export const REQUEST_ACTIONS: ActionType[] = [
  'setUrl', 'setMethod', 'setHeader', 'removeHeader',
  'setQueryParam', 'removeQueryParam', 'setCookie', 'removeCookie',
  'setBody', 'appendBody', 'replaceBodyText', 'patchBodyJson',
  'setFormField', 'removeFormField', 'block'
]

// 响应阶段可用行为
export const RESPONSE_ACTIONS: ActionType[] = [
  'setStatus', 'setHeader', 'removeHeader',
  'setBody', 'appendBody', 'replaceBodyText', 'patchBodyJson'
]

// 行为类型标签
export const ACTION_TYPE_LABELS: Record<ActionType, string> = {
  setUrl: '设置 URL',
  setMethod: '设置 Method',
  setHeader: '设置 Header',
  removeHeader: '移除 Header',
  setQueryParam: '设置 Query 参数',
  removeQueryParam: '移除 Query 参数',
  setCookie: '设置 Cookie',
  removeCookie: '移除 Cookie',
  setBody: '替换 Body',
  appendBody: '追加 Body',
  replaceBodyText: '文本替换 Body',
  patchBodyJson: 'JSON Patch',
  setFormField: '设置表单字段',
  removeFormField: '移除表单字段',
  setStatus: '设置状态码',
  block: '拦截请求'
}

// 终结性行为
export const TERMINAL_ACTIONS: ActionType[] = ['block']

// 创建空条件
export function createEmptyCondition(type: ConditionType = 'urlPrefix'): Condition {
  const base: Condition = { type }

  // 根据类型设置默认字段
  if (type === 'method') {
    return { ...base, values: ['GET'] }
  }
  if (type === 'resourceType') {
    return { ...base, values: ['xhr', 'fetch'] }
  }
  if (type.endsWith('Regex')) {
    return { ...base, pattern: '' }
  }
  if (type.startsWith('header') || type.startsWith('query') || type.startsWith('cookie')) {
    if (type.endsWith('Exists') || type.endsWith('NotExists')) {
      return { ...base, name: '' }
    }
    if (type.endsWith('Regex')) {
      return { ...base, name: '', pattern: '' }
    }
    return { ...base, name: '', value: '' }
  }
  if (type === 'bodyJsonPath') {
    return { ...base, path: '', value: '' }
  }

  return { ...base, value: '' }
}

// 创建空行为
export function createEmptyAction(type: ActionType = 'setHeader', _stage: Stage = 'request'): Action {
  switch (type) {
    case 'setUrl':
    case 'setMethod':
      return { type, value: '' }
    case 'setHeader':
    case 'setQueryParam':
    case 'setCookie':
    case 'setFormField':
      return { type, name: '', value: '' }
    case 'removeHeader':
    case 'removeQueryParam':
    case 'removeCookie':
    case 'removeFormField':
      return { type, name: '' }
    case 'setBody':
    case 'appendBody':
      return { type, value: '', encoding: 'text' }
    case 'replaceBodyText':
      return { type, search: '', replace: '', replaceAll: false }
    case 'patchBodyJson':
      return { type, patches: [] }
    case 'setStatus':
      return { type, value: 200 }
    case 'block':
      return { type, statusCode: 200, headers: { 'Content-Type': 'application/json' }, body: '{}' }
    default:
      return { type }
  }
}

// 创建空规则
export function createEmptyRule(stage: Stage = 'request'): Rule {
  return {
    id: `rule_${Date.now()}`,
    name: '新规则',
    enabled: true,
    priority: 100,
    stage,
    match: {
      allOf: [createEmptyCondition('urlPrefix')]
    },
    actions: []
  }
}

// 判断行为是否为终结性
export function isTerminalAction(action: Action): boolean {
  return TERMINAL_ACTIONS.includes(action.type)
}

// 判断行为是否适用于指定阶段
export function isActionValidForStage(actionType: ActionType, stage: Stage): boolean {
  if (stage === 'request') {
    return REQUEST_ACTIONS.includes(actionType)
  }
  return RESPONSE_ACTIONS.includes(actionType)
}

// 获取阶段可用的行为类型
export function getActionsForStage(stage: Stage): ActionType[] {
  return stage === 'request' ? REQUEST_ACTIONS : RESPONSE_ACTIONS
}

// 获取条件需要的字段
export function getConditionFields(type: ConditionType): ('value' | 'values' | 'pattern' | 'name' | 'path')[] {
  if (type === 'method' || type === 'resourceType') {
    return ['values']
  }
  if (type.endsWith('Regex')) {
    if (type.startsWith('url') || type.startsWith('body')) {
      return ['pattern']
    }
    return ['name', 'pattern']
  }
  if (type.endsWith('Exists') || type.endsWith('NotExists')) {
    return ['name']
  }
  if (type.startsWith('header') || type.startsWith('query') || type.startsWith('cookie')) {
    return ['name', 'value']
  }
  if (type === 'bodyJsonPath') {
    return ['path', 'value']
  }
  return ['value']
}
