import { type FC, type PropsWithChildren } from 'react'
import { MarkdownTextPrimitive } from '@assistant-ui/react-markdown'
import { cn } from '@/lib/utils'
import { defaultComponents, MARKDOWN_PLUGINS } from '@/components/assistant-ui/markdown-text'

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
    return (
        <div className="aui-reasoning-group my-2 flex items-baseline gap-1.5 text-sm text-[var(--app-hint)]">
            <span className="shrink-0 text-xs opacity-60">∴</span>
            <span className="shrink-0 text-xs font-medium">Thinking</span>
            <span className="min-w-0">{children}</span>
        </div>
    )
}

