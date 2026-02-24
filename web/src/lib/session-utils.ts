import type { Machine } from '@/types/api'

export function getSessionTitle(session: { id: string; metadata?: { name?: string; path?: string } | null }): string {
    if (session.metadata?.name) {
        return session.metadata.name
    }
    if (session.metadata?.path) {
        const parts = session.metadata.path.split('/').filter(Boolean)
        return parts.length > 0 ? parts[parts.length - 1] : session.id.slice(0, 8)
    }
    return session.id.slice(0, 8)
}

export function getMachineTitle(machine: Machine): string {
    if (machine.metadata?.displayName) return machine.metadata.displayName
    if (machine.metadata?.host) return machine.metadata.host
    return machine.id.slice(0, 8)
}
