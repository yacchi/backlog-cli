import { BrowserRouter, Navigate, Route, Routes } from "react-router-dom";
import { AuthProvider } from "./context/AuthContext";
import { StreamingProvider } from "./context/StreamingContext";
import AuthComplete from "./pages/AuthComplete";
import LoginApiKey from "./pages/LoginApiKey";
import LoginConfirm from "./pages/LoginConfirm";
import LoginMethodSelect from "./pages/LoginMethodSelect";
import LoginSetup from "./pages/LoginSetup";
import Portal from "./pages/Portal";

export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        {/* Portal route (no auth context needed) */}
        <Route path="/portal/:domain" element={<Portal />} />

        {/* Auth routes */}
        <Route
          path="/auth/*"
          element={
            <AuthProvider>
              <StreamingProvider>
                <Routes>
                  <Route path="/setup" element={<LoginSetup />} />
                  <Route path="/method" element={<LoginMethodSelect />} />
                  <Route path="/start" element={<LoginConfirm />} />
                  <Route path="/apikey" element={<LoginApiKey />} />
                  <Route path="/complete" element={<AuthComplete />} />
                  <Route
                    path="*"
                    element={<Navigate to="/auth/method" replace />}
                  />
                </Routes>
              </StreamingProvider>
            </AuthProvider>
          }
        />

        <Route path="*" element={<Navigate to="/auth/method" replace />} />
      </Routes>
    </BrowserRouter>
  );
}
