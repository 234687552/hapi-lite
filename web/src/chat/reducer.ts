import type { AgentState } from '@/types/api'
import type { ChatBlock, NormalizedMessage, UsageData } from '@/chat/types'
import { traceMessages, type TracedMessage } from '@/chat/tracer'
import { dedupeAgentEvents, foldApiErrorEvents } from '@/chat/reducerEvents'
import { collectToolIdsFromMessages, ensureToolBlock, getPermissions } from '@/chat/reducerTools'
import { reduceTimeline } from '@/chat/reducerTimeline'

// Calculate context size from usage data
function calculateContextSize(usage: UsageData): number {
    return (usage.cache_creation_input_tokens || 0) + (usage.cache_read_input_tokens || 0) + usage.input_tokens
}

export type LatestUsage = {
    inputTokens: number
    outputTokens: number
    cacheCreation: number
    cacheRead: number
    contextSize: number
    timestamp: number
}

export function reduceChatBlocks(
    normalized: NormalizedMessage[],
    agentState: AgentState | null | undefined
): { blocks: ChatBlock[]; hasReadyEvent: boolean; latestUsage: LatestUsage | null } {
    const permissionsById = getPermissions(agentState)
    const toolIdsInMessages = collectToolIdsFromMessages(normalized)

    const traced = traceMessages(normalized)
    const groups = new Map<string, TracedMessage[]>()
    const root: TracedMessage[] = []

    for (const msg of traced) {
        if (msg.sidechainId) {
            const existing = groups.get(msg.sidechainId) ?? []
            existing.push(msg)
            groups.set(msg.sidechainId, existing)
        } else {
            root.push(msg)
        }
    }

    const consumedGroupIds = new Set<string>()
    const reducerContext = { permissionsById, groups, consumedGroupIds }
    const rootResult = reduceTimeline(root, reducerContext)
    let hasReadyEvent = rootResult.hasReadyEvent

    // Only create permission-only tool cards when there is no tool call/result in the transcript.
    // Also skip if the permission is older than the oldest message in the current view,
    // to avoid mixing old tool cards with newer messages when paginating.
    const oldestMessageTime = normalized.length > 0
        ? Math.min(...normalized.map(m => m.createdAt))
        : null

    for (const [id, entry] of permissionsById) {
        if (toolIdsInMessages.has(id)) continue
        if (rootResult.toolBlocksById.has(id)) continue

        const createdAt = entry.permission.createdAt ?? Date.now()

        // Skip permissions that are older than the oldest message in the current view.
        // These will be shown when the user loads older messages.
        if (oldestMessageTime !== null && createdAt < oldestMessageTime) {
            continue
        }

        const block = ensureToolBlock(rootResult.blocks, rootResult.toolBlocksById, id, {
            createdAt,
            localId: null,
            name: entry.toolName,
            input: entry.input,
            description: null,
            permission: entry.permission
        })

        if (entry.permission.status === 'approved') {
            block.tool.state = 'completed'
            block.tool.completedAt = entry.permission.completedAt ?? createdAt
            if (block.tool.result === undefined) {
                block.tool.result = 'Approved'
            }
        } else if (entry.permission.status === 'denied' || entry.permission.status === 'canceled') {
            block.tool.state = 'error'
            block.tool.completedAt = entry.permission.completedAt ?? createdAt
            if (block.tool.result === undefined && entry.permission.reason) {
                block.tool.result = { error: entry.permission.reason }
            }
        }
    }

    // Calculate cumulative usage for current turn
    // Find the last user message to determine the start of current turn
    let lastUserMsgIndex = -1
    for (let i = normalized.length - 1; i >= 0; i--) {
        if (normalized[i].role === 'user') {
            lastUserMsgIndex = i
            break
        }
    }

    // Accumulate tokens from all messages after the last user message
    let latestUsage: LatestUsage | null = null
    let totalInput = 0
    let totalOutput = 0
    let totalCacheCreation = 0
    let totalCacheRead = 0
    let lastContextSize = 0
    let lastTimestamp = 0

    // Start from the message after the last user message, or from the beginning if no user message
    const startIndex = lastUserMsgIndex >= 0 ? lastUserMsgIndex + 1 : 0

    for (let i = startIndex; i < normalized.length; i++) {
        const msg = normalized[i]
        if (msg.usage) {
            totalInput += msg.usage.input_tokens
            totalOutput += msg.usage.output_tokens
            totalCacheCreation += msg.usage.cache_creation_input_tokens ?? 0
            totalCacheRead += msg.usage.cache_read_input_tokens ?? 0
            lastContextSize = calculateContextSize(msg.usage)
            lastTimestamp = msg.createdAt
        }
    }

    if (totalInput > 0 || totalOutput > 0) {
        latestUsage = {
            inputTokens: totalInput,
            outputTokens: totalOutput,
            cacheCreation: totalCacheCreation,
            cacheRead: totalCacheRead,
            contextSize: lastContextSize,
            timestamp: lastTimestamp
        }
    }

    return { blocks: dedupeAgentEvents(foldApiErrorEvents(rootResult.blocks)), hasReadyEvent, latestUsage }
}
