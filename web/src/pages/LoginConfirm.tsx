import {useEffect, useRef, useState} from 'react'
import {Navigate, useNavigate} from 'react-router-dom'
import Button from '../components/Button'
import Container from '../components/Container'
import InfoBox from '../components/InfoBox'
import ResultView, {ResultType} from '../components/ResultView'
import StatusIndicator from '../components/StatusIndicator'
import {useAuthContext} from '../context/AuthContext'
import {useWebSocket} from '../hooks/useWebSocket'

export default function LoginConfirm() {
  const navigate = useNavigate()
  const {loading, error, data} = useAuthContext()
  const {status, error: wsError, disconnect} = useWebSocket()
  const [isLoggingIn, setIsLoggingIn] = useState(false)
  const [popupMessage, setPopupMessage] = useState<string | null>(null)
  const [forcedResult, setForcedResult] = useState<ResultType | null>(null)
  const popupCheckRef = useRef<number | null>(null)

  useEffect(() => {
    if (status === 'success' || status === 'error' || status === 'closed' || forcedResult) {
      if (popupCheckRef.current) {
        window.clearInterval(popupCheckRef.current)
        popupCheckRef.current = null
      }
    }
  }, [status, forcedResult])

  if (!loading && data && !data.configured) {
    return <Navigate to="/auth/setup" replace />
  }

  if (forcedResult) {
    return (
      <main className="flex min-h-screen items-center justify-center px-4 py-10">
        <Container>
          <ResultView type={forcedResult} message={popupMessage || undefined} />
        </Container>
      </main>
    )
  }

  if (status === 'success') {
    return (
      <main className="flex min-h-screen items-center justify-center px-4 py-10">
        <Container>
          <ResultView type="success" />
        </Container>
      </main>
    )
  }

  if (status === 'error') {
    return (
      <main className="flex min-h-screen items-center justify-center px-4 py-10">
        <Container>
          <ResultView type="error" message={wsError || undefined} />
        </Container>
      </main>
    )
  }

  if (status === 'closed') {
    return (
      <main className="flex min-h-screen items-center justify-center px-4 py-10">
        <Container>
          <ResultView type="closed" />
        </Container>
      </main>
    )
  }

  const handleLogin = () => {
    setPopupMessage(null)
    setIsLoggingIn(true)

    const width = 600
    const height = 700
    const left = window.screenX + (window.outerWidth - width) / 2
    const top = window.screenY + (window.outerHeight - height) / 2
    const popup = window.open(
      '/auth/popup',
      'backlog_auth',
      `width=${width},height=${height},left=${left},top=${top}`
    )

    if (!popup || popup.closed || typeof popup.closed === 'undefined') {
      setPopupMessage('ポップアップがブロックされました。ポップアップを許可してください。')
      setIsLoggingIn(false)
      return
    }

    setPopupMessage('ポップアップで認証を進めてください...')

    popupCheckRef.current = window.setInterval(() => {
      if (status !== 'connecting' && status !== 'connected') {
        if (popupCheckRef.current) {
          window.clearInterval(popupCheckRef.current)
          popupCheckRef.current = null
        }
        return
      }

      if (popup.closed) {
        if (popupCheckRef.current) {
          window.clearInterval(popupCheckRef.current)
          popupCheckRef.current = null
        }
        disconnect()
        setForcedResult('error')
        setPopupMessage('認証がキャンセルされました。ポップアップが閉じられました。')
      }
    }, 1000)
  }

  return (
    <main className="flex min-h-screen items-center justify-center px-4 py-10">
      <Container>
        <div className="flex flex-col gap-6">
          <header className="space-y-3 text-center">
            <p className="text-xs font-semibold uppercase tracking-[0.3em] text-ink/60">
              Backlog CLI
            </p>
            <h1 className="text-3xl font-semibold text-ink">ログイン</h1>
            <p className="text-sm text-ink/70">
              Backlog CLI がターミナルからの操作で Backlog API にアクセスするための認証を行います。
            </p>
          </header>

          {error ? (
            <div className="rounded-2xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">
              {error}
            </div>
          ) : null}

          <div className="space-y-3">
            <InfoBox
              label="スペース"
              value={data ? `${data.space}.${data.domain}` : '読み込み中...'}
            />
            <InfoBox
              label="リレーサーバー"
              value={data ? data.relayServer : '読み込み中...'}
            />
          </div>

          <div className="flex flex-wrap items-center justify-center gap-3">
            <Button
              variant="secondary"
              type="button"
              onClick={() => {
                disconnect()
                navigate('/auth/setup')
              }}
            >
              設定を変更
            </Button>
            <Button type="button" onClick={handleLogin} disabled={isLoggingIn || loading}>
              ログインする
            </Button>
          </div>

          {isLoggingIn ? (
            <StatusIndicator message={popupMessage || 'ログイン画面を開いています...'} />
          ) : null}
        </div>
      </Container>
    </main>
  )
}
