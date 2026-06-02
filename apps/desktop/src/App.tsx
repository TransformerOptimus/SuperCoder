import { useEffect, useRef } from 'react';
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import { ThemeProvider } from './context/ThemeContext';
import { AntDThemeProvider } from './providers/AntDThemeProvider';
import TopBar from './components/TopBar';
import Workspace from './pages/Workspace';
import Settings from './pages/Settings';
import { useAppStore } from './store';
import { useAgentEvents } from './hooks/useAgentEvents';

function App() {
  const loadProviders = useAppStore((s) => s.loadProviders);
  const initialized = useRef(false);

  // Listen for local agent events (Tauri → store), once.
  useAgentEvents();

  useEffect(() => {
    if (initialized.current) return;
    initialized.current = true;
    loadProviders();
  }, [loadProviders]);

  return (
    <ThemeProvider>
      <AntDThemeProvider>
        <BrowserRouter>
          <div className="h-screen flex flex-col bg-white dark:bg-dark-bg overflow-hidden">
            <TopBar />
            <Routes>
              <Route path="/" element={<Workspace />} />
              <Route path="/settings" element={<Settings />} />
              <Route path="*" element={<Navigate to="/" replace />} />
            </Routes>
          </div>
        </BrowserRouter>
      </AntDThemeProvider>
    </ThemeProvider>
  );
}

export default App;
