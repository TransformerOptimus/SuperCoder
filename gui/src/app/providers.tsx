'use client';

import { NextUIProvider } from '@nextui-org/react';
import { UserContextProvider } from '@/context/UserContext';

export function Providers({ children }: { children: React.ReactNode }) {
  return (
    <NextUIProvider>
      <UserContextProvider>{children}</UserContextProvider>
    </NextUIProvider>
  );
}
