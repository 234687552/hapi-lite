type KnownAgentFlavor = 'claude' | 'codex' | 'gemini' | 'opencode'

type SendingLabelInput = {
    elapsedMs: number | null
    inputTokens?: number
    outputTokens?: number
}

type StatusAgentSpec = {
    label: string
    buildSendingLabel: (input: SendingLabelInput) => string
}

function formatDuration(ms: number): string {
    const totalSeconds = Math.max(0, Math.floor(ms / 1000))
    if (totalSeconds < 60) return `${totalSeconds}s`
    const minutes = Math.floor(totalSeconds / 60)
    const seconds = totalSeconds % 60
    return seconds === 0 ? `${minutes}m` : `${minutes}m ${seconds}s`
}

function formatTokens(tokens: number): string {
    if (tokens >= 1000000) return `${(tokens / 1000000).toFixed(1)}M`
    if (tokens >= 1000) return `${(tokens / 1000).toFixed(1)}k`
    return String(tokens)
}

function buildClaudeSendingLabel(input: SendingLabelInput): string {
    const segments: string[] = []
    if (input.elapsedMs !== null) {
        segments.push(formatDuration(input.elapsedMs))
    }
    const totalTokens = (input.inputTokens ?? 0) + (input.outputTokens ?? 0)
    if (totalTokens > 0) {
        const arrow = (input.outputTokens ?? 0) > 0 ? '↓' : '↑'
        segments.push(`${arrow} ${formatTokens(totalTokens)} tokens`)
    }
    return segments.length > 0 ? `thinking... (${segments.join(' ')})` : 'thinking...'
}

function buildCodexSendingLabel(input: SendingLabelInput): string {
    if (input.elapsedMs === null) return 'running...'
    return `running... (${formatDuration(input.elapsedMs)})`
}

function buildGeminiSendingLabel(input: SendingLabelInput): string {
    if (input.elapsedMs === null) return 'generating...'
    return `generating... (${formatDuration(input.elapsedMs)})`
}

function buildOpencodeSendingLabel(input: SendingLabelInput): string {
    if (input.elapsedMs === null) return 'processing...'
    return `processing... (${formatDuration(input.elapsedMs)})`
}

const STATUS_AGENT_SPECS: Record<KnownAgentFlavor, StatusAgentSpec> = {
    claude: {
        label: 'Claude',
        buildSendingLabel: buildClaudeSendingLabel
    },
    codex: {
        label: 'Codex',
        buildSendingLabel: buildCodexSendingLabel
    },
    gemini: {
        label: 'Gemini',
        buildSendingLabel: buildGeminiSendingLabel
    },
    opencode: {
        label: 'Opencode',
        buildSendingLabel: buildOpencodeSendingLabel
    }
}

function toKnownAgentFlavor(flavor?: string | null): KnownAgentFlavor {
    if (flavor === 'codex') return 'codex'
    if (flavor === 'gemini') return 'gemini'
    if (flavor === 'opencode') return 'opencode'
    return 'claude'
}

export function getStatusAgentLabel(flavor?: string | null): string {
    return STATUS_AGENT_SPECS[toKnownAgentFlavor(flavor)].label
}

export function buildSendingStatusText(flavor: string | null | undefined, input: SendingLabelInput): string {
    return STATUS_AGENT_SPECS[toKnownAgentFlavor(flavor)].buildSendingLabel(input)
}
