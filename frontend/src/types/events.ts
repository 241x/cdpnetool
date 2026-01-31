// 拦截事件相关类型

// 请求信息
export interface RequestInfo {
  url: string
  method: string
  headers: Record<string, string>
  body: string
  resourceType?: string  // document/xhr/script/image等
}

// 响应信息
export interface ResponseInfo {
  statusCode: number
  headers: Record<string, string>
  body: string
  timing?: {
    startTime: number  // 开始时间
    endTime: number    // 结束时间
  }
}

// 规则匹配信息
export interface RuleMatch {
  ruleId: string
  ruleName: string
  actions: string[]  // 执行的 action 类型列表
}

// 网络事件（通用结构）
export interface NetworkEvent {
  session: string
  target: string
  timestamp: number
  isMatched: boolean
  request: RequestInfo
  response?: ResponseInfo
  finalResult?: 'blocked' | 'modified' | 'passed'
  matchedRules?: RuleMatch[]
}

// 匹配的事件（会存入数据库）
export interface MatchedEvent {
  networkEvent: NetworkEvent
}

// 前端扩展类型（添加本地 ID 用于 React key）
export interface MatchedEventWithId extends MatchedEvent {
  id: string
}

// 结果类型标签和颜色
export type FinalResultType = 'blocked' | 'modified' | 'passed'

// 结果类型标签
export const FINAL_RESULT_LABELS: Record<FinalResultType, string> = {
  blocked: '阻断',
  modified: '修改',
  passed: '放行',
}

// 结果类型颜色
export const FINAL_RESULT_COLORS: Record<FinalResultType, { bg: string; text: string }> = {
  blocked: { bg: 'bg-red-500/20', text: 'text-red-500' },
  modified: { bg: 'bg-yellow-500/20', text: 'text-yellow-500' },
  passed: { bg: 'bg-green-500/20', text: 'text-green-500' },
}
