import { DesignProvider } from '@/context/Design';
import React from 'react';

export default function DesignLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return (
    <section>
      <DesignProvider>{children}</DesignProvider>
    </section>
  );
}
