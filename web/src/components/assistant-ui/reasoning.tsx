import { useState, useEffect, type FC, type PropsWithChildren } from 'react'
import { useMessage } from '@assistant-ui/react'
import { MarkdownTextPrimitive } from '@assistant-ui/react-markdown'
import { cn } from '@/lib/utils'
import { defaultComponents, MARKDOWN_PLUGINS } from '@/components/assistant-ui/markdown-text'

function ChevronIcon(props: { className?: string; open?: boolean }) {
    return (
        <svg
            xmlns="http://www.w3.org/2000/svg"
            width="12"
            height="12"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="2"
            strokeLinecap="round"
            strokeLinejoin="round"
            className={cn(
                'transition-transform duration-200',
                props.open ? 'rotate-90' : '',
                props.className
            )}
        >
            <polyline points="9 18 15 12 9 6" />
        </svg>
    )
}

function ShimmerDot() {
    return (
        <span className="inline-block w-1.5 h-1.5 bg-current rounded-full animate-pulse" />
    )
}

/**
 * Renders individual reasoning message part content with markdown support.
 */
export const Reasoning: FC = () => {
    return (
        <MarkdownTextPrimitive
            remarkPlugins={MARKDOWN_PLUGINS}
            components={defaultComponents}
            className={cn('aui-reasoning-content min-w-0 max-w-full break-words text-sm text-[var(--app-hint)]')}
        />
    )
}

/**
 * Wraps consecutive reasoning parts in a collapsible container.
 * Shows shimmer effect while reasoning is streaming.
 * Auto-collapses when final answer starts outputting.
 */
export const ReasoningGroup: FC<PropsWithChildren> = ({ children }) => {
    const [isOpen, setIsOpen] = useState(false)
    const [hasAutoCollapsed, setHasAutoCollapsed] = useState(false)

    // Check if reasoning is still streaming
    const message = useMessage()
    const isStreaming = message.status?.type === 'running'
    const lastPart = message.content.length > 0 ? message.content[message.content.length - 1] : null
    const isReasoningStreaming = isStreaming && lastPart?.type === 'reasoning'

    // Check if final answer has started (text content after reasoning)
    const hasTextAfterReasoning = message.content.some((part, idx) => {
        if (part.type !== 'text') return false
        // Check if there's reasoning before this text
        return message.content.slice(0, idx).some(p => p.type === 'reasoning')
    })

    // Auto-expand while reasoning is streaming
    useEffect(() => {
        if (isReasoningStreaming) {
            setIsOpen(true)
            setHasAutoCollapsed(false)
        }
    }, [isReasoningStreaming])

    // Auto-collapse when final answer starts
    useEffect(() => {
        if (hasTextAfterReasoning && !hasAutoCollapsed && isOpen) {
            setIsOpen(false)
            setHasAutoCollapsed(true)
        }
    }, [hasTextAfterReasoning, hasAutoCollapsed, isOpen])

    return (
        <div className="aui-reasoning-group my-2 rounded-lg bg-[var(--app-subtle-bg)] border border-[var(--app-border)]">
            <button
                type="button"
                onClick={() => setIsOpen(!isOpen)}
                className={cn(
                    'flex items-center gap-1.5 w-full px-3 py-2 text-xs font-medium',
                    'text-[var(--app-hint)] hover:text-[var(--app-fg)]',
                    'transition-colors cursor-pointer select-none'
                )}
            >
                <ChevronIcon open={isOpen} />
                <span>Thinking</span>
                {isReasoningStreaming && (
                    <span className="flex items-center gap-1 ml-1 text-[var(--app-hint)]">
                        <ShimmerDot />
                    </span>
                )}
            </button>

            <div
                className={cn(
                    'overflow-hidden transition-all duration-200 ease-in-out',
                    isOpen ? 'max-h-[5000px] opacity-100' : 'max-h-0 opacity-0'
                )}
            >
                <div className="px-3 pb-3 pt-1 text-sm text-[var(--app-hint)]">
                    {children}
                </div>
            </div>
        </div>
    )
}
