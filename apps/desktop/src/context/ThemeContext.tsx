import { createContext, useContext, useState, useEffect, useCallback, type ReactNode } from 'react';

export type ThemeMode = 'light' | 'dark' | 'system';

interface ThemeContextType {
  /** User preference (persisted). */
  mode: ThemeMode;
  /** Resolved theme — what's actually applied. Kept for existing consumers. */
  dark: boolean;
  setMode: (mode: ThemeMode) => void;
  /** Flip the resolved theme to an explicit light/dark preference. */
  toggle: () => void;
}

const ThemeContext = createContext<ThemeContextType>({
  mode: 'system',
  dark: false,
  setMode: () => {},
  toggle: () => {},
});

const STORAGE_KEY = 'theme';

function prefersDark(): boolean {
  return typeof window !== 'undefined' && window.matchMedia('(prefers-color-scheme: dark)').matches;
}

function readStoredMode(): ThemeMode {
  const saved = localStorage.getItem(STORAGE_KEY);
  if (saved === 'dark' || saved === 'light' || saved === 'system') return saved;
  return 'system'; // default: follow OS
}

function resolveDark(mode: ThemeMode): boolean {
  return mode === 'system' ? prefersDark() : mode === 'dark';
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [mode, setModeState] = useState<ThemeMode>(() => readStoredMode());
  const [dark, setDark] = useState<boolean>(() => resolveDark(readStoredMode()));

  // Apply the resolved theme to the DOM + persist the preference.
  useEffect(() => {
    const root = document.documentElement;
    if (dark) {
      root.classList.add('dark');
      root.setAttribute('data-theme', 'DARK');
    } else {
      root.classList.remove('dark');
      root.setAttribute('data-theme', 'LIGHT');
    }
  }, [dark]);

  useEffect(() => {
    localStorage.setItem(STORAGE_KEY, mode);
    setDark(resolveDark(mode));
  }, [mode]);

  // While following the OS, react live to system appearance changes.
  useEffect(() => {
    if (mode !== 'system') return;
    const mq = window.matchMedia('(prefers-color-scheme: dark)');
    const onChange = () => setDark(mq.matches);
    mq.addEventListener('change', onChange);
    return () => mq.removeEventListener('change', onChange);
  }, [mode]);

  const setMode = useCallback((m: ThemeMode) => setModeState(m), []);

  const toggle = useCallback(() => {
    setModeState((prev) => (resolveDark(prev) ? 'light' : 'dark'));
  }, []);

  return (
    <ThemeContext.Provider value={{ mode, dark, setMode, toggle }}>
      {children}
    </ThemeContext.Provider>
  );
}

export function useTheme() {
  return useContext(ThemeContext);
}
