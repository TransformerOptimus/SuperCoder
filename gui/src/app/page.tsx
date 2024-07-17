import LandingPage from '@/components/HomeComponents/LandingPage';
import React, { Suspense } from 'react';

export default function Home() {
  return (
    <div id={'home'}>
      <Suspense fallback={<div>Loading...</div>}>
        <LandingPage />
      </Suspense>
    </div>
  );
}
