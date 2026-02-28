import { useCallback, useEffect, useRef, useState } from 'react'
import type { AgentState } from '@/types/api'

export type StatusPhase =
    | 'offline'     // session 未激活
    | 'sending'     // HTTP 发送中
    | 'thinking'    // agent 运行中
    | 'aborting'    // 已发中止信号，等后端确认
    | 'permission'  // agent 等待用户授权
    | 'idle'        // 空闲

function derivePhase(
    active: boolean,
    thinking: boolean,
    hasPermissions: boolean,
    isAborting: boolean,
    isSending: boolean
): StatusPhase {
    // thinking 优先于 offline（SSE 竞态时 thinking 更真实）
    if (thinking) return hasPermissions ? 'permission' : 'thinking'
    if (!active) return 'offline'
    if (isAborting) return 'aborting'
    if (isSending) return 'sending'
    return 'idle'
}

export function useComposerStatus(input: {
    active: boolean
    thinking: boolean   // session.thinking || threadIsRunning
    isSending: boolean  // HTTP 发送中
    agentState?: AgentState | null
    thinkingAt?: number // 后端记录的开始时间，用于页面刷新后恢复计时
}) {
    const [isAborting, setIsAborting] = useState(false)

    const hasPermissions = Boolean(
        input.agentState?.requests &&
        Object.keys(input.agentState.requests).length > 0
    )

    const phase = derivePhase(
        input.active,
        input.thinking,
        hasPermissions,
        isAborting,
        input.isSending
    )

    // 页面刷新时 phase 已是 thinking，用后端时间戳做初始值恢复计时
    const [thinkingStartedAt, setThinkingStartedAt] = useState<number | null>(() => {
        if (input.thinking && input.thinkingAt && input.thinkingAt > 0) {
            return input.thinkingAt
        }
        return null
    })

    // thinking 停止后自动清除 aborting 标记
    useEffect(() => {
        if (isAborting && !input.thinking) setIsAborting(false)
    }, [isAborting, input.thinking])

    // 每次重新进入 thinking 阶段时重置计时起点
    // prevPhaseRef 初始化为当前 phase，确保刷新时不覆盖上面的懒初始值
    const prevPhaseRef = useRef<StatusPhase>(phase)
    useEffect(() => {
        const prev = prevPhaseRef.current
        prevPhaseRef.current = phase
        if (phase === 'thinking' && prev !== 'thinking') {
            setThinkingStartedAt(Date.now())
        } else if (phase !== 'thinking') {
            setThinkingStartedAt(null)
        }
    }, [phase])

    const startAbort = useCallback(() => setIsAborting(true), [])

    return { phase, thinkingStartedAt, isAborting, startAbort }
}
