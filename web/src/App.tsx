import { BrowserRouter, Navigate, Route, Routes } from "react-router-dom";
import { AuthProvider } from "./context/AuthContext";
import { StreamingProvider } from "./context/StreamingContext";
import AuthComplete from "./pages/AuthComplete";
import LoginApiKey from "./pages/LoginApiKey";
import LoginConfirm from "./pages/LoginConfirm";
import LoginMethodSelect from "./pages/LoginMethodSelect";
import LoginSetup from "./pages/LoginSetup";

export default function App() {
  return (
    <AuthProvider>
      <StreamingProvider>
        <BrowserRouter>
          <Routes>
            <Route path="/auth/setup" element={<LoginSetup />} />
            <Route path="/auth/method" element={<LoginMethodSelect />} />
            <Route path="/auth/start" element={<LoginConfirm />} />
            <Route path="/auth/apikey" element={<LoginApiKey />} />
            <Route path="/auth/complete" element={<AuthComplete />} />
            <Route path="*" element={<Navigate to="/auth/method" replace />} />
          </Routes>
        </BrowserRouter>
      </StreamingProvider>
    </AuthProvider>
  );
}
