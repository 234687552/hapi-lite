import { getPermissionModeLabel, getPermissionModeTone, isPermissionModeAllowedForFlavor } from '@hapi/protocol'
import type { PermissionModeTone } from '@hapi/protocol'
import { useEffect, useMemo, useState } from 'react'
import type { ModelMode, PermissionMode } from '@/types/api'
import type { StatusPhase } from '@/hooks/useComposerStatus'
import { isClaudeFlavor } from '@/lib/agentFlavorUtils'
import { getContextBudgetTokens } from '@/chat/modelConfig'
import { useTranslation } from '@/lib/use-translation'

const VIBING_MESSAGES = [
    "Accomplishing", "Actioning", "Actualizing", "Baking", "Booping", "Brewing",
    "Calculating", "Cerebrating", "Channelling", "Churning", "Clauding", "Coalescing",
    "Cogitating", "Computing", "Combobulating", "Concocting", "Conjuring", "Considering",
    "Contemplating", "Cooking", "Crafting", "Creating", "Crunching", "Deciphering",
    "Deliberating", "Determining", "Discombobulating", "Divining", "Doing", "Effecting",
    "Elucidating", "Enchanting", "Envisioning", "Finagling", "Flibbertigibbeting",
    "Forging", "Forming", "Frolicking", "Generating", "Germinating", "Hatching",
    "Herding", "Honking", "Ideating", "Imagining", "Incubating", "Inferring",
    "Manifesting", "Marinating", "Meandering", "Moseying", "Mulling", "Mustering",
    "Musing", "Noodling", "Percolating", "Perusing", "Philosophising", "Pontificating",
    "Pondering", "Processing", "Puttering", "Puzzling", "Razzmatazzing", "Reticulating", "Ruminating",
    "Scheming", "Schlepping", "Shimmying", "Simmering", "Smooshing", "Spelunking",
    "Spinning", "Stewing", "Sussing", "Synthesizing", "Thinking", "Tinkering",
    "Transmuting", "Unfurling", "Unravelling", "Vibing", "Wandering", "Whirring",
    "Wibbling", "Wizarding", "Working", "Wrangling"
]

const THINKING_REFRESH_MS = 1000
const VIBING_CHANGE_MS = 4000

const PERMISSION_TONE_CLASSES: Record<PermissionModeTone, string> = {
    neutral: 'text-[var(--app-hint)]',
    info: 'text-blue-500',
    warning: 'text-amber-500',
    danger: 'text-red-500'
}

function formatDuration(durationMs: number): string {
    const totalSeconds = Math.max(0, Math.floor(durationMs / 1000))
    if (totalSeconds < 60) {
        return `${totalSeconds}s`
    }
    const minutes = Math.floor(totalSeconds / 60)
    const seconds = totalSeconds % 60
    return seconds === 0 ? `${minutes}m` : `${minutes}m ${seconds}s`
}

function formatTokens(tokens: number): string {
    if (tokens >= 1000000) return `${(tokens / 1000000).toFixed(1)}M`
    if (tokens >= 1000) return `${(tokens / 1000).toFixed(1)}k`
    return String(tokens)
}

function getContextWarning(
    contextSize: number,
    maxContextSize: number,
    t: (key: string, params?: Record<string, string | number>) => string
): { text: string; color: string } | null {
    const percentageUsed = (contextSize / maxContextSize) * 100
    const percentageRemaining = Math.max(0, 100 - percentageUsed)
    const percent = Math.round(percentageRemaining)
    if (percentageRemaining <= 5) {
        return { text: t('misc.percentLeft', { percent }), color: 'text-red-500' }
    } else if (percentageRemaining <= 10) {
        return { text: t('misc.percentLeft', { percent }), color: 'text-amber-500' }
    } else {
        return { text: t('misc.percentLeft', { percent }), color: 'text-[var(--app-hint)]' }
    }
}

type StatusDisplay = { text: string; color: string; dotColor: string; isPulsing: boolean }

function getStatusDisplay(
    phase: StatusPhase,
    thinkingLabel: string | null,
    t: (key: string) => string
): StatusDisplay {
    switch (phase) {
        case 'thinking':
            return { text: thinkingLabel ?? '', color: 'text-[#34C759]', dotColor: 'bg-[#34C759]', isPulsing: true }
        case 'permission':
            return { text: t('misc.permissionRequired'), color: 'text-[#FF9500]', dotColor: 'bg-[#FF9500]', isPulsing: true }
        case 'aborting':
            return { text: t('misc.stopping'), color: 'text-[#FF9500]', dotColor: 'bg-[#FF9500]', isPulsing: true }
        case 'sending':
            return { text: t('misc.sending'), color: 'text-[#34C759]', dotColor: 'bg-[#34C759]', isPulsing: true }
        case 'offline':
            return { text: t('misc.offline'), color: 'text-[#999]', dotColor: 'bg-[#999]', isPulsing: false }
        case 'idle':
        default:
            return { text: t('misc.online'), color: 'text-[#34C759]', dotColor: 'bg-[#34C759]', isPulsing: false }
    }
}

export function StatusBar(props: {
    phase: StatusPhase
    thinkingStartedAt: number | null
    contextSize?: number
    inputTokens?: number
    outputTokens?: number
    modelMode?: ModelMode
    permissionMode?: PermissionMode
    agentFlavor?: string | null
}) {
    const { t } = useTranslation()
    const [now, setNow] = useState(() => Date.now())
    const [vibingIndex, setVibingIndex] = useState(() => Math.floor(Math.random() * VIBING_MESSAGES.length))

    const isThinking = props.phase === 'thinking'

    useEffect(() => {
        if (!isThinking) return
        setNow(Date.now())
        const timer = globalThis.setInterval(() => setNow(Date.now()), THINKING_REFRESH_MS)
        const vibingTimer = globalThis.setInterval(
            () => setVibingIndex(Math.floor(Math.random() * VIBING_MESSAGES.length)),
            VIBING_CHANGE_MS
        )
        return () => {
            globalThis.clearInterval(timer)
            globalThis.clearInterval(vibingTimer)
        }
    }, [isThinking])

    const thinkingLabel = useMemo(() => {
        if (!isThinking) return null
        const msg = `${VIBING_MESSAGES[vibingIndex]}…`
        const segments: string[] = []
        if (props.thinkingStartedAt !== null) {
            segments.push(formatDuration(now - props.thinkingStartedAt))
        }
        // 仅 Claude 有 token 数据，其他 agent（Codex 等）不展示
        if (isClaudeFlavor(props.agentFlavor)) {
            const total = (props.inputTokens ?? 0) + (props.outputTokens ?? 0)
            if (total > 0) {
                const arrow = (props.outputTokens ?? 0) > 0 ? '↓' : '↑'
                segments.push(`${arrow} ${formatTokens(total)} tokens`)
            }
        }
        return segments.length > 0 ? `${msg} (${segments.join(' ')})` : msg
    }, [isThinking, props.thinkingStartedAt, props.agentFlavor, props.inputTokens, props.outputTokens, vibingIndex, now])

    const statusDisplay = useMemo(
        () => getStatusDisplay(props.phase, thinkingLabel, t),
        [props.phase, thinkingLabel, t]
    )

    const contextWarning = useMemo(() => {
        if (props.contextSize === undefined) return null
        const maxContextSize = getContextBudgetTokens(props.modelMode)
        if (!maxContextSize) return null
        return getContextWarning(props.contextSize, maxContextSize, t)
    }, [props.contextSize, props.modelMode, t])

    const permissionMode = props.permissionMode
    const displayPermissionMode = permissionMode
        && permissionMode !== 'default'
        && isPermissionModeAllowedForFlavor(permissionMode, props.agentFlavor)
        ? permissionMode
        : null

    const permissionModeLabel = displayPermissionMode ? getPermissionModeLabel(displayPermissionMode) : null
    const permissionModeTone = displayPermissionMode ? getPermissionModeTone(displayPermissionMode) : null
    const permissionModeColor = permissionModeTone ? PERMISSION_TONE_CLASSES[permissionModeTone] : 'text-[var(--app-hint)]'

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
                    {contextWarning ? (
                        <span className={`text-[10px] ${contextWarning.color}`}>
                            {contextWarning.text}
                        </span>
                    ) : null}
                </div>
            </div>

            {displayPermissionMode ? (
                <span className={`text-xs ${permissionModeColor}`}>
                    {permissionModeLabel}
                </span>
            ) : null}
        </div>
    )
}
