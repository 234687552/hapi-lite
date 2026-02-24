import { unwrapRoleWrappedRecordEnvelope } from '@hapi/protocol/messages'
import { safeStringify, isObject } from '@hapi/protocol'
import type { DecryptedMessage } from '@/types/api'
import type { NormalizedMessage } from '@/chat/types'
import { isCodexContent, isSkippableAgentContent, normalizeAgentRecord } from '@/chat/normalizeAgent'
import { normalizeUserRecord } from '@/chat/normalizeUser'

function withMessageState(message: DecryptedMessage, normalized: NormalizedMessage): NormalizedMessage {
    return { ...normalized, status: message.status, originalText: message.originalText }
}

function normalizeSdkAssistantRecord(message: DecryptedMessage, content: unknown, meta?: unknown): NormalizedMessage | null {
    return normalizeAgentRecord(
        message.id,
        message.localId,
        message.createdAt,
        {
            type: 'output',
            data: {
                type: 'assistant',
                message: {
                    content
                }
            }
        },
        meta
    )
}

function normalizeSdkUserAgentRecord(message: DecryptedMessage, content: unknown, meta?: unknown): NormalizedMessage | null {
    return normalizeAgentRecord(
        message.id,
        message.localId,
        message.createdAt,
        {
            type: 'output',
            data: {
                type: 'user',
                message: {
                    content
                }
            }
        },
        meta
    )
}

export function normalizeDecryptedMessage(message: DecryptedMessage): NormalizedMessage | null {
    const record = unwrapRoleWrappedRecordEnvelope(message.content)
    if (!record) {
        return {
            id: message.id,
            localId: message.localId,
            createdAt: message.createdAt,
            role: 'agent',
            isSidechain: false,
            content: [{ type: 'text', text: safeStringify(message.content), uuid: message.id, parentUUID: null }],
            status: message.status,
            originalText: message.originalText
        }
    }

    if (record.role === 'user') {
        const normalized = normalizeUserRecord(message.id, message.localId, message.createdAt, record.content, record.meta)
        if (normalized) {
            return withMessageState(message, normalized)
        }

        // Claude/SDK raw stream-json may emit `role: user` for tool_result payloads.
        // Treat non-text user records as agent-side tool output when possible.
        if (Array.isArray(record.content) || isObject(record.content)) {
            const sdkUserAsAgent = normalizeSdkUserAgentRecord(message, record.content, record.meta)
            if (sdkUserAsAgent) {
                return withMessageState(message, sdkUserAsAgent)
            }
        }

        return {
            id: message.id,
            localId: message.localId,
            createdAt: message.createdAt,
            role: 'user',
            isSidechain: false,
            content: { type: 'text', text: safeStringify(record.content) },
            meta: record.meta,
            status: message.status,
            originalText: message.originalText
        }
    }
    if (record.role === 'assistant') {
        const normalized = normalizeSdkAssistantRecord(message, record.content, record.meta)
        return normalized
            ? withMessageState(message, normalized)
            : {
                id: message.id,
                localId: message.localId,
                createdAt: message.createdAt,
                role: 'agent',
                isSidechain: false,
                content: [{ type: 'text', text: safeStringify(record.content), uuid: message.id, parentUUID: null }],
                meta: record.meta,
                status: message.status,
                originalText: message.originalText
            }
    }
    if (record.role === 'agent') {
        if (isSkippableAgentContent(record.content)) {
            return null
        }
        const normalized = normalizeAgentRecord(message.id, message.localId, message.createdAt, record.content, record.meta)
        if (!normalized && isCodexContent(record.content)) {
            return null
        }
        return normalized
            ? withMessageState(message, normalized)
            : {
                id: message.id,
                localId: message.localId,
                createdAt: message.createdAt,
                role: 'agent',
                isSidechain: false,
                content: [{ type: 'text', text: safeStringify(record.content), uuid: message.id, parentUUID: null }],
                meta: record.meta,
                status: message.status,
                originalText: message.originalText
            }
    }

    return {
        id: message.id,
        localId: message.localId,
        createdAt: message.createdAt,
        role: 'agent',
        isSidechain: false,
        content: [{ type: 'text', text: safeStringify(record.content), uuid: message.id, parentUUID: null }],
        meta: record.meta,
        status: message.status,
        originalText: message.originalText
    }
}
