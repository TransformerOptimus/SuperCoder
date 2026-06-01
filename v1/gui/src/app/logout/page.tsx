'use client';
import React, { useEffect } from 'react';
import { logout } from '@/app/utils';

const Logout: React.FC = () => {
  useEffect(() => {
    logout();
  }, []);

  return (
    <div id={'logout'}>
      <span>....Logout</span>
    </div>
  );
};

export default Logout;
