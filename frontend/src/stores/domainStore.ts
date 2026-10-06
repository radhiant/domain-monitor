import { defineStore } from 'pinia'
import { api } from '@/services/api'
import { DomainStream } from '@/services/websocket'
import { DEFAULTS } from '@/config/thresholds'
import type {
  AgentConfig,
  ConnectionState,
  DomainGroup,
  ExpiryItem,
  FullSnapshot,
  Incident,
  Overview,
  StatusEvent,
  TargetState,
  WsMessage,
} from '@/types/domain'

export type ViewName = 'wall' | 'detail' | 'list'
export type FitMode = 'tv' | 'scroll'

interface State {
  config: AgentConfig
  overview: Overview | null
  groups: DomainGroup[]
  connection: ConnectionState
  view: ViewName
  selectedId: string | null
  fitMode: FitMode
  loaded: boolean
  loadError: string | null
  lastUpdate: number
}

let stream: DomainStream | null = null

export const useDomainStore = defineStore('domain', {
  state: (): State => ({
    config: { ...DEFAULTS },
    overview: null,
    groups: [],
    connection: 'connecting',
    view: 'wall',
    selectedId: null,
    fitMode: typeof window !== 'undefined' && window.innerWidth < 1024 ? 'scroll' : 'tv',
    loaded: false,
    loadError: null,
    lastUpdate: 0,
  }),

  getters: {
    /** Every endpoint, flattened, in the order list-domain.txt declares them. */
    targets(state): TargetState[] {
      return state.groups.flatMap((g) => g.endpoints)
    },

    selected(): TargetState | null {
      const id = this.selectedId
      if (!id) return null
      return this.targets.find((t: TargetState) => t.id === id) ?? null
    },

    expiry(state): ExpiryItem[] {
      return state.overview?.expiry ?? []
    },

    incidents(state): Incident[] {
      return state.overview?.incidents ?? []
    },

    events(state): StatusEvent[] {
      return state.overview?.events ?? []
    },

    /** True once the wall has something real to show. */
    hasData(state): boolean {
      return state.groups.length > 0
    },
  },

  actions: {
    /** Loads the initial payload, then hands over to the live stream. */
    async init() {
      this.readLocation()

      // Thresholds come from the agent so the frontend never keeps a second,
      // divergent copy of what "degraded" means.
      try {
        this.config = await api.config()
      } catch {
        this.config = { ...DEFAULTS }
      }

      try {
        const [overview, groups] = await Promise.all([api.overview(), api.domains()])
        this.overview = overview
        this.groups = groups
        this.loaded = true
        this.loadError = null
        this.lastUpdate = Date.now()
      } catch (err) {
        this.loadError = err instanceof Error ? err.message : 'Agent tidak dapat dihubungi'
      }

      this.startStream()
      window.addEventListener('popstate', () => this.readLocation())
    },

    startStream() {
      stream?.close()
      stream = new DomainStream({
        onState: (state) => {
          this.connection = state
        },
        onMessage: (msg) => this.applyMessage(msg),
      })
      stream.connect()
    },

    stopStream() {
      stream?.close()
      stream = null
    },

    applyMessage(msg: WsMessage) {
      switch (msg.type) {
        case 'snapshot': {
          const full = msg.data as FullSnapshot
          this.overview = full.overview
          this.groups = full.groups
          this.loaded = true
          this.loadError = null
          break
        }

        case 'overview':
          this.overview = msg.data as Overview
          break

        case 'update':
          this.mergeTarget(msg.data as TargetState)
          break

        case 'event':
          // The overview carries the authoritative event list; this only keeps
          // the ticker current between the five-second overview pushes.
          if (this.overview) {
            const event = msg.data as StatusEvent
            this.overview.events = [event, ...this.overview.events].slice(0, 40)
          }
          break

        case 'pong':
          return
      }

      this.lastUpdate = Date.now()
    },

    /**
     * Replaces one endpoint in place.
     *
     * Swapping a single array element keeps Vue re-rendering just that tile:
     * with 30 tiles on a wall that never sleeps, rebuilding the whole grid on
     * every probe result would be visible as a flicker.
     */
    mergeTarget(next: TargetState) {
      for (const group of this.groups) {
        const index = group.endpoints.findIndex((t) => t.id === next.id)
        if (index === -1) continue

        group.endpoints[index] = next
        group.up_count = group.endpoints.filter((t) => t.status === 'UP').length
        group.status = group.endpoints.reduce<TargetState['status']>(
          (worst, t) => (rank(t.status) > rank(worst) ? t.status : worst),
          'UNKNOWN',
        )
        return
      }
    },

    // --- navigation ---------------------------------------------------------

    /**
     * The view lives in the query string rather than a router.
     *
     * It keeps the bundle smaller, and it means each display can be pinned to
     * its own URL: the TV opens ?view=wall, a desk browser opens ?view=list.
     */
    readLocation() {
      const params = new URLSearchParams(window.location.search)
      const view = params.get('view')
      const target = params.get('target')

      this.view = view === 'detail' || view === 'list' ? view : 'wall'
      this.selectedId = target
      if (this.view === 'detail' && !target) this.view = 'wall'

      const fit = params.get('fit')
      if (fit === 'scroll' || fit === 'tv') {
        this.fitMode = fit
      } else if (typeof window !== 'undefined' && window.innerWidth < 1024) {
        this.fitMode = 'scroll'
      } else {
        this.fitMode = 'tv'
      }
    },

    writeLocation() {
      const params = new URLSearchParams()
      params.set('view', this.view)
      if (this.view === 'detail' && this.selectedId) params.set('target', this.selectedId)
      if (this.fitMode !== 'tv') params.set('fit', this.fitMode)
      window.history.pushState({}, '', `?${params.toString()}`)
    },

    setView(view: ViewName) {
      this.view = view
      if (view !== 'detail') this.selectedId = null
      this.writeLocation()
    },

    openTarget(id: string) {
      this.selectedId = id
      this.view = 'detail'
      this.writeLocation()
    },

    toggleFitMode() {
      this.fitMode = this.fitMode === 'tv' ? 'scroll' : 'tv'
      this.writeLocation()
    },
  },
})

const RANK: Record<string, number> = { UNKNOWN: 0, UP: 1, DEGRADED: 2, DOWN: 3 }
function rank(status: string): number {
  return RANK[status] ?? 0
}
