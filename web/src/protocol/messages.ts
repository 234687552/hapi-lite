import { isObject } from './utils'

type RoleWrappedRecord = {
    role: string
    content: unknown
    meta?: unknown
    usage?: unknown
}

export function isRoleWrappedRecord(value: unknown): value is RoleWrappedRecord {
    if (!isObject(value)) return false
    return typeof value.role === 'string' && 'content' in value
}

export function unwrapRoleWrappedRecordEnvelope(value: unknown): RoleWrappedRecord | null {
    if (isRoleWrappedRecord(value)) return value
    if (!isObject(value)) return null

    const direct = value.message
    if (isRoleWrappedRecord(direct)) {
        return { ...direct, usage: (direct as Record<string, unknown>).usage }
    }

    const data = value.data
    if (isObject(data) && isRoleWrappedRecord(data.message)) {
        const msg = data.message as Record<string, unknown>
        return { ...(msg as RoleWrappedRecord), usage: msg.usage }
    }

    const payload = value.payload
    if (isObject(payload) && isRoleWrappedRecord(payload.message)) {
        const msg = payload.message as Record<string, unknown>
        return { ...(msg as RoleWrappedRecord), usage: msg.usage }
    }

    return null
}

export type { RoleWrappedRecord }
