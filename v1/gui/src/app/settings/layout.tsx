import NavBar from '@/components/LayoutComponents/NavBar';
import React from 'react';
import styles from '@/app/projects/projects.module.css';
import { Toaster } from 'react-hot-toast';

export default function SettingsLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return (
    <div className={'layout fixed'}>
      <NavBar />
      <div className={'main_content'}>
        <section className={styles.project_content}>{children}</section>
        <Toaster />
      </div>
    </div>
  );
}
