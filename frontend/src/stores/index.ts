import { create } from 'zustand'
import type { 
  InterceptEvent, 
  MatchedEventWithId, 
  UnmatchedEventWithId 
} from '@/types/events'

// 类型定义
export interface TargetInfo {
  id: string
  type: string
  url: string
  title: string
  isCurrent: boolean
  isUser: boolean
}

// Session 状态
interface SessionState {
  currentSessionId: string | null
  devToolsURL: string
  isConnected: boolean
  isIntercepting: boolean
  targets: TargetInfo[]
  attachedTargets: Set<string>
  
  // 事件分开存储
  matchedEvents: MatchedEventWithId[]    // 匹配的事件（会存入数据库）
  unmatchedEvents: UnmatchedEventWithId[] // 未匹配的事件（仅内存）
  
  // Actions
  setDevToolsURL: (url: string) => void
  setCurrentSession: (id: string | null) => void
  setConnected: (connected: boolean) => void
  setIntercepting: (intercepting: boolean) => void
  setTargets: (targets: TargetInfo[]) => void
  toggleAttachedTarget: (targetId: string) => void
  
  // 事件操作
  addInterceptEvent: (event: InterceptEvent) => void
  clearMatchedEvents: () => void
  clearUnmatchedEvents: () => void
  clearAllEvents: () => void
}

// 生成事件 ID
function generateEventId(timestamp: number): string {
  return `${timestamp}_${Math.random().toString(36).slice(2, 10)}`
}

export const useSessionStore = create<SessionState>((set) => ({
  currentSessionId: null,
  devToolsURL: 'http://localhost:9222',
  isConnected: false,
  isIntercepting: false,
  targets: [],
  attachedTargets: new Set(),
  matchedEvents: [],
  unmatchedEvents: [],
  
  setDevToolsURL: (url) => set({ devToolsURL: url }),
  setCurrentSession: (id) => set({ currentSessionId: id }),
  setConnected: (connected) => set({ isConnected: connected }),
  setIntercepting: (intercepting) => set({ isIntercepting: intercepting }),
  setTargets: (targets) => set({ targets }),
  toggleAttachedTarget: (targetId) => set((state) => {
    const newSet = new Set(state.attachedTargets)
    if (newSet.has(targetId)) {
      newSet.delete(targetId)
    } else {
      newSet.add(targetId)
    }
    return { attachedTargets: newSet }
  }),
  
  // 添加事件（根据 isMatched 分开存储）
  addInterceptEvent: (event) => set((state) => {
    if (event.isMatched && event.matched) {
      const eventWithId: MatchedEventWithId = {
        ...event.matched,
        id: generateEventId(event.matched.timestamp),
      }
      return {
        matchedEvents: [eventWithId, ...state.matchedEvents].slice(0, 200) // 保留最新 200 条
      }
    } else if (!event.isMatched && event.unmatched) {
      const eventWithId: UnmatchedEventWithId = {
        ...event.unmatched,
        id: generateEventId(event.unmatched.timestamp),
      }
      return {
        unmatchedEvents: [eventWithId, ...state.unmatchedEvents].slice(0, 100) // 保留最新 100 条
      }
    }
    return {}
  }),
  
  clearMatchedEvents: () => set({ matchedEvents: [] }),
  clearUnmatchedEvents: () => set({ unmatchedEvents: [] }),
  clearAllEvents: () => set({ matchedEvents: [], unmatchedEvents: [] }),
}))

// 主题状态
interface ThemeState {
  isDark: boolean
  toggle: () => void
}

export const useThemeStore = create<ThemeState>((set) => ({
  isDark: true,
  toggle: () => set((state) => {
    const newIsDark = !state.isDark
    if (newIsDark) {
      document.documentElement.classList.add('dark')
    } else {
      document.documentElement.classList.remove('dark')
    }
    return { isDark: newIsDark }
  }),
}))

// 初始化主题
if (typeof window !== 'undefined') {
  document.documentElement.classList.add('dark')
}
