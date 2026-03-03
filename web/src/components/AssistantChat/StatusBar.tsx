import { useEffect, useMemo, useState } from 'react'
import type { StatusPhase } from '@/hooks/useComposerStatus'
import { useTranslation } from '@/lib/use-translation'
import { buildSendingStatusText, getStatusAgentLabel } from '@/components/AssistantChat/statusByAgent'

const THINKING_REFRESH_MS = 1000

type StatusDisplay = { text: string; color: string; dotColor: string; isPulsing: boolean }

function getStatusDisplay(
    phase: StatusPhase,
    sendingLabel: string | null,
    t: (key: string) => string
): StatusDisplay {
    switch (phase) {
        case 'sending':
            return { text: sendingLabel ?? t('misc.sending'), color: 'text-[#34C759]', dotColor: 'bg-[#34C759]', isPulsing: true }
        case 'offline':
            return { text: t('misc.offline'), color: 'text-[#999]', dotColor: 'bg-[#999]', isPulsing: false }
        case 'online':
        default:
            return { text: t('misc.online'), color: 'text-[#34C759]', dotColor: 'bg-[#34C759]', isPulsing: false }
    }
}

export function StatusBar(props: {
    phase: StatusPhase
    thinkingStartedAt: number | null
    inputTokens?: number
    outputTokens?: number
    agentFlavor?: string | null
}) {
    const { t } = useTranslation()
    const [now, setNow] = useState(() => Date.now())
    const isSending = props.phase === 'sending'

    useEffect(() => {
        if (!isSending) return
        setNow(Date.now())
        const timer = globalThis.setInterval(() => setNow(Date.now()), THINKING_REFRESH_MS)
        return () => {
            globalThis.clearInterval(timer)
        }
    }, [isSending])

    const sendingLabel = useMemo(() => {
        if (!isSending) return null
        const elapsedMs = props.thinkingStartedAt !== null ? Math.max(0, now - props.thinkingStartedAt) : null
        return buildSendingStatusText(props.agentFlavor, {
            elapsedMs,
            inputTokens: props.inputTokens,
            outputTokens: props.outputTokens
        })
    }, [isSending, now, props.agentFlavor, props.inputTokens, props.outputTokens, props.thinkingStartedAt])

    const statusDisplay = useMemo(
        () => getStatusDisplay(props.phase, sendingLabel, t),
        [props.phase, sendingLabel, t]
    )

    const agentLabel = useMemo(
        () => getStatusAgentLabel(props.agentFlavor),
        [props.agentFlavor]
    )

    return (
        <div className="flex items-center justify-between px-2 pb-1">
            <div className="min-w-0 flex-1">
                <div className="flex items-baseline gap-3">
                    <div className="flex items-center gap-1.5">
                        <span
                            className={`h-2 w-2 rounded-full ${statusDisplay.dotColor} ${statusDisplay.isPulsing ? 'animate-pulse' : ''}`}
                        />
                        <span className={`text-xs ${statusDisplay.color}`}>
                            {statusDisplay.text}
                        </span>
                    </div>
                </div>
            </div>
            <span className="text-xs text-[var(--app-hint)]">
                {agentLabel}
            </span>
        </div>
    )
}
