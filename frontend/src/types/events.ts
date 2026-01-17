// 拦截事件相关类型

// ========== 请求响应数据 ==========
export interface RequestResponseData {
  headers: Record<string, string>
  body: string
  statusCode?: number  // 仅响应阶段有
}

// ========== 规则匹配信息 ==========
export interface RuleMatch {
  ruleId: string
  ruleName: string
  actions: string[]  // 执行的 action 类型列表
}

// ========== 匹配的事件（会存入数据库）==========
export interface MatchedEvent {
  session: string
  target: string
  url: string
  method: string
  stage: 'request' | 'response'
  statusCode?: number  // 仅响应阶段有
  timestamp: number
  finalResult: 'blocked' | 'modified' | 'passed'
  matchedRules: RuleMatch[]
  original: RequestResponseData
  modified: RequestResponseData
}

// ========== 未匹配的事件（仅内存，不存数据库）==========
export interface UnmatchedEvent {
  target: string
  url: string
  method: string
  stage: 'request' | 'response'
  statusCode?: number
  timestamp: number
}

// ========== 统一事件接口（用于通道传输）==========
export interface InterceptEvent {
  isMatched: boolean
  matched?: MatchedEvent
  unmatched?: UnmatchedEvent
}

// ========== 前端扩展类型（添加本地 ID 用于 React key）==========
export interface MatchedEventWithId extends MatchedEvent {
  id: string
}

export interface UnmatchedEventWithId extends UnmatchedEvent {
  id: string
}

// ========== 结果类型标签和颜色 ==========
export type FinalResultType = 'blocked' | 'modified' | 'passed'

export const FINAL_RESULT_LABELS: Record<FinalResultType, string> = {
  blocked: '阻断',
  modified: '修改',
  passed: '放行',
}

export const FINAL_RESULT_COLORS: Record<FinalResultType, { bg: string; text: string }> = {
  blocked: { bg: 'bg-red-500/20', text: 'text-red-500' },
  modified: { bg: 'bg-yellow-500/20', text: 'text-yellow-500' },
  passed: { bg: 'bg-green-500/20', text: 'text-green-500' },
}

// 未匹配事件的默认样式
export const UNMATCHED_COLORS = { bg: 'bg-slate-500/20', text: 'text-slate-500' }
