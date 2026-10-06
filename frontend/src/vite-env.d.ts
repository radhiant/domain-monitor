/// <reference types="vite/client" />

interface ImportMetaEnv {
  /** Base URL of the agent. Empty means same origin, proxied by Vite or Nginx. */
  readonly VITE_API_BASE?: string
  /** Bearer token, matching API_TOKEN on the agent. */
  readonly VITE_API_TOKEN?: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}

declare module '*.vue' {
  import type { DefineComponent } from 'vue'
  const component: DefineComponent<Record<string, unknown>, Record<string, unknown>, unknown>
  export default component
}
