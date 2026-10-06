import { streamUrl } from '@/services/api'
import type { ConnectionState, WsMessage } from '@/types/domain'

/** Backoff steps, in milliseconds. */
const BACKOFF = [1000, 2000, 4000, 8000, 15000, 30000]

/** How long to wait for a pong before assuming the socket is dead. */
const HEARTBEAT_MS = 25000

export interface StreamHandlers {
  onMessage: (msg: WsMessage) => void
  onState: (state: ConnectionState) => void
}

/**
 * Live connection to the agent.
 *
 * The reconnect logic matters more here than in most dashboards: this page is
 * left open on a TV for weeks, and it has to survive an agent restart, a
 * network blip and a display waking from sleep without anyone touching it.
 */
export class DomainStream {
  private socket: WebSocket | null = null
  private attempt = 0
  private heartbeat: ReturnType<typeof setInterval> | null = null
  private retry: ReturnType<typeof setTimeout> | null = null
  private closed = false

  constructor(private handlers: StreamHandlers) {}

  connect(): void {
    this.closed = false
    this.cleanupSocket()
    this.handlers.onState(this.attempt === 0 ? 'connecting' : 'reconnecting')

    try {
      this.socket = new WebSocket(streamUrl())
    } catch {
      this.scheduleRetry()
      return
    }

    this.socket.onopen = () => {
      this.attempt = 0
      this.handlers.onState('live')
      this.startHeartbeat()
    }

    this.socket.onmessage = (event) => {
      try {
        this.handlers.onMessage(JSON.parse(event.data) as WsMessage)
      } catch {
        // A frame we cannot parse is not worth tearing the connection down for.
      }
    }

    this.socket.onerror = () => {
      // onclose always follows, and that is where the retry lives.
    }

    this.socket.onclose = () => {
      this.stopHeartbeat()
      if (!this.closed) {
        this.handlers.onState('reconnecting')
        this.scheduleRetry()
      }
    }
  }

  /** Closes the connection and stops reconnecting. */
  close(): void {
    this.closed = true
    this.stopHeartbeat()
    if (this.retry) {
      clearTimeout(this.retry)
      this.retry = null
    }
    this.cleanupSocket()
    this.handlers.onState('offline')
  }

  private scheduleRetry(): void {
    if (this.retry) clearTimeout(this.retry)
    const delay = BACKOFF[Math.min(this.attempt, BACKOFF.length - 1)]
    this.attempt++
    this.retry = setTimeout(() => this.connect(), delay)
  }

  /**
   * An application-level ping. A dead TCP connection can sit open for minutes
   * before the browser notices, which on a wall display means minutes of
   * confidently showing stale numbers.
   */
  private startHeartbeat(): void {
    this.stopHeartbeat()
    this.heartbeat = setInterval(() => {
      if (this.socket?.readyState === WebSocket.OPEN) {
        this.socket.send(JSON.stringify({ type: 'ping' }))
      }
    }, HEARTBEAT_MS)
  }

  private stopHeartbeat(): void {
    if (this.heartbeat) {
      clearInterval(this.heartbeat)
      this.heartbeat = null
    }
  }

  private cleanupSocket(): void {
    if (!this.socket) return
    this.socket.onopen = null
    this.socket.onmessage = null
    this.socket.onerror = null
    this.socket.onclose = null
    if (this.socket.readyState === WebSocket.OPEN || this.socket.readyState === WebSocket.CONNECTING) {
      this.socket.close()
    }
    this.socket = null
  }
}
