'use client';
import { useRouter, useSearchParams } from 'next/navigation';
import { useEffect } from 'react';
import React, { Suspense } from 'react';
import { setCookie } from '@/utils/CookieUtils';

function RedirectComponent() {
  const searchParams = useSearchParams();
  const router = useRouter();
  useEffect(() => {
    if (typeof window !== 'undefined') {
      setCookie('accessToken', searchParams.get('token') || '');
      localStorage.setItem('userName', searchParams.get('name') || '');
      localStorage.setItem('userEmail', searchParams.get('email') || '');
      if (window.clarity) {
        window.clarity('set', 'User Email', searchParams.get('email') || '');
      }
    }

    router.push('/projects');
  }, [searchParams, router]);

  return null;
}

export default function Redirect() {
  return (
    <Suspense fallback={<div>Loading...</div>}>
      <RedirectComponent />
    </Suspense>
  );
}
