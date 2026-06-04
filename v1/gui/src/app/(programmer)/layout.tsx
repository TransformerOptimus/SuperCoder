'use client';
import '../_app.css';
import React, { useEffect } from 'react';
import SideBar from '@/components/LayoutComponents/SideBar';
import NavBar from '@/components/LayoutComponents/NavBar';
import Code from '@/app/(programmer)/code/Code';
import { Toaster } from 'react-hot-toast';
import { SocketProvider } from '@/context/SocketContext';
import { useUserContext } from '@/context/UserContext';

export default function ProgrammerLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const userContext = useUserContext();
  if (!userContext) {
    throw new Error('useUser must be used within a UserProvider');
  }

  useEffect(() => {
    if (typeof window !== 'undefined' && window.clarity && userContext) {
      window.clarity('set', 'User Email', userContext.name);
    }
  }, [userContext]);

  return (
    <div className={'layout'}>
      <NavBar />
      <div className={'main_content'}>
        <SideBar />
        <SocketProvider>
          <section className={'content'}>
            <Code />
            {children}
          </section>
        </SocketProvider>

        <Toaster />
      </div>
    </div>
  );
}
