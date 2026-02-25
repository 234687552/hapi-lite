import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import { VitePWA } from 'vite-plugin-pwa'
import { resolve } from 'node:path'
import { createRequire } from 'node:module'

const require = createRequire(import.meta.url)
const base = process.env.VITE_BASE_URL || '/'

export default defineConfig({
    define: {
        __APP_VERSION__: JSON.stringify(require('./package.json').version),
    },
    server: {
        port: 5703,
        host: true,
        allowedHosts: true,
        cors: true,
        proxy: {
            '/api': {
                target: 'http://127.0.0.1:3009',
                changeOrigin: true
            },
            '/socket.io': {
                target: 'http://127.0.0.1:3009',
                ws: true
            }
        }
    },
    plugins: [
        react(),
        VitePWA({
            registerType: 'autoUpdate',
            includeAssets: ['favicon.ico', 'apple-touch-icon-180x180.png', 'mask-icon.svg'],
            strategies: 'injectManifest',
            srcDir: 'src',
            filename: 'sw.ts',
            manifest: {
                name: 'HAPI-LITE',
                short_name: 'HAPI-LITE',
                description: 'AI-powered development assistant',
                theme_color: '#ffffff',
                background_color: '#ffffff',
                display: 'standalone',
                orientation: 'portrait',
                scope: base,
                start_url: base,
                icons: [
                    {
                        src: 'pwa-64x64.png',
                        sizes: '64x64',
                        type: 'image/png'
                    },
                    {
                        src: 'pwa-192x192.png',
                        sizes: '192x192',
                        type: 'image/png'
                    },
                    {
                        src: 'pwa-512x512.png',
                        sizes: '512x512',
                        type: 'image/png'
                    },
                    {
                        src: 'maskable-icon-512x512.png',
                        sizes: '512x512',
                        type: 'image/png',
                        purpose: 'maskable'
                    }
                ]
            },
            injectManifest: {
                globPatterns: ['**/*.{js,css,html,ico,png,svg,woff,woff2}'],
                maximumFileSizeToCacheInBytes: 3 * 1024 * 1024
            },
            devOptions: {
                enabled: true,
                type: 'module'
            }
        })
    ],
    base,
    resolve: {
        alias: [
            { find: '@', replacement: resolve(__dirname, 'src') },
            { find: /^@hapi\/protocol$/, replacement: resolve(__dirname, 'src/protocol') },
            { find: /^@hapi\/protocol\/(.*)$/, replacement: resolve(__dirname, 'src/protocol/$1') }
        ]
    },
    build: {
        outDir: 'dist',
        emptyOutDir: true
    }
})
