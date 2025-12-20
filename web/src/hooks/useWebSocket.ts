import {useCallback, useEffect, useRef, useState} from 'react'

type Status = 'connecting' | 'connected' | 'success' | 'error' | 'closed'

interface UseWebSocketResult {
  status: Status
  error: string | null
  disconnect: () => void
}

export function useWebSocket(): UseWebSocketResult {
  const [status, setStatus] = useState<Status>('connecting')
  const [error, setError] = useState<string | null>(null)
  const wsRef = useRef<WebSocket | null>(null)
  const activeRef = useRef(true)

  useEffect(() => {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const wsUrl = `${protocol}//${window.location.host}/auth/ws`

    try {
      wsRef.current = new WebSocket(wsUrl)
    } catch (err) {
      setStatus('closed')
      return
    }

    wsRef.current.onopen = () => {
      setStatus('connected')
    }

    wsRef.current.onmessage = (event) => {
      if (!activeRef.current) return

      try {
        const data = JSON.parse(event.data) as {status?: string; error?: string}
        if (data.status === 'success') {
          activeRef.current = false
          setStatus('success')
        } else if (data.status === 'error') {
          activeRef.current = false
          setStatus('error')
          setError(data.error || '認証に失敗しました')
        }
      } catch (err) {
        console.error('WebSocket message parse error:', err)
      }
    }

    wsRef.current.onclose = () => {
      if (activeRef.current) {
        activeRef.current = false
        setStatus('closed')
      }
    }

    return () => {
      activeRef.current = false
      wsRef.current?.close()
    }
  }, [])

  const disconnect = useCallback(() => {
    activeRef.current = false
    wsRef.current?.close()
  }, [])

  return {
    status,
    error,
    disconnect,
  }
}
