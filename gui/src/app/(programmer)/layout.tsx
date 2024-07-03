'use client';

import '../_app.css';
import React, { useState, useEffect } from 'react';
import SideBar from '@/components/LayoutComponents/SideBar';
import NavBar from '@/components/LayoutComponents/NavBar';
import { usePathname } from 'next/navigation';
import Code from '@/app/(programmer)/code/Code';
import { Toaster } from 'react-hot-toast';
import { SocketProvider } from '@/context/SocketContext';

export default function ProgrammerLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const pathname = usePathname();
  const [isCodeRendered, setIsCodeRendered] = useState(false);

  useEffect(() => {
    if (pathname === '/code') {
      setIsCodeRendered(true);
    }
  }, [pathname]);

  useEffect(() => {
    if (typeof window !== 'undefined' && window.clarity) {
      console.log('Email Tracked');
      window.clarity('set', 'User Email', localStorage.getItem('userEmail'));
    }
  }, []);

  return (
    <div className="layout">
      <NavBar />
      <div className="main_content">
        <SideBar />
        <SocketProvider>
          <section className="content">
            {isCodeRendered && <Code />}
            {children}
          </section>
        </SocketProvider>
        <Toaster />
      </div>
    </div>
  );
}
