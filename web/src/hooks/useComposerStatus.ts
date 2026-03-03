import { useCallback, useEffect, useRef, useState } from 'react'

export type StatusPhase =
    | 'offline'
    | 'sending'
    | 'online'

function derivePhase(
    active: boolean,
    thinking: boolean,
    isSending: boolean
): StatusPhase {
    if (thinking || isSending) return 'sending'
    if (!active) return 'offline'
    return 'online'
}

export function useComposerStatus(input: {
    active: boolean
    thinking: boolean
    isSending: boolean
    thinkingAt?: number
}) {
    const [isAborting, setIsAborting] = useState(false)

    const phase = derivePhase(input.active, input.thinking, input.isSending)

    const [thinkingStartedAt, setThinkingStartedAt] = useState<number | null>(() => {
        if ((input.thinking || input.isSending) && input.thinkingAt && input.thinkingAt > 0) {
            return input.thinkingAt
        }
        return null
    })

    // abort 结束后恢复默认状态
    useEffect(() => {
        if (isAborting && !input.thinking) setIsAborting(false)
    }, [isAborting, input.thinking])

    const prevPhaseRef = useRef<StatusPhase>(phase)
    useEffect(() => {
        const prev = prevPhaseRef.current
        prevPhaseRef.current = phase
        if (phase === 'sending' && prev !== 'sending') {
            if (input.thinkingAt && input.thinkingAt > 0) {
                setThinkingStartedAt(input.thinkingAt)
            } else {
                setThinkingStartedAt(Date.now())
            }
        } else if (phase !== 'sending') {
            setThinkingStartedAt(null)
        }
    }, [phase, input.thinkingAt])

    const startAbort = useCallback(() => setIsAborting(true), [])

    return { phase, thinkingStartedAt, isAborting, startAbort }
}
