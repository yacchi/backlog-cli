import {BrowserRouter, Navigate, Route, Routes} from 'react-router-dom'
import {AuthProvider} from './context/AuthContext'
import {WebSocketProvider} from './context/WebSocketContext'
import LoginConfirm from './pages/LoginConfirm'
import LoginSetup from './pages/LoginSetup'

export default function App() {
  return (
    <AuthProvider>
      <WebSocketProvider>
        <BrowserRouter>
          <Routes>
            <Route path="/auth/setup" element={<LoginSetup />} />
            <Route path="/auth/start" element={<LoginConfirm />} />
            <Route path="*" element={<Navigate to="/auth/start" replace />} />
          </Routes>
        </BrowserRouter>
      </WebSocketProvider>
    </AuthProvider>
  )
}
