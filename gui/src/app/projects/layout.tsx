import React, { ReactNode } from 'react';
import NavBar from '@/components/LayoutComponents/NavBar';
import styles from './projects.module.css';
import { SocketProvider } from '@/context/SocketContext';

export default function ProjectsLayout({
  children,
}: Readonly<{ children: ReactNode }>) {
  return (
    <div className={'layout fixed'}>
      <NavBar />
      <div className={'main_content'}>
        <section className={styles.project_content}>{children}</section>
      </div>
    </div>
  );
}
