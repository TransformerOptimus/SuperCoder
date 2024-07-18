'use client';
import { useRouter, useSearchParams } from 'next/navigation';
import { useEffect } from 'react';
import React, { Suspense } from 'react';
import { setUserData } from '@/app/utils';
import { userData } from '../../../types/authTypes';

function RedirectComponent() {
  const searchParams = useSearchParams();
  const router = useRouter();
  useEffect(() => {
    if (typeof window !== 'undefined') {
      const data: userData = {
        userEmail: searchParams.get('email') || '',
        userName: searchParams.get('name') || '',
        accessToken: searchParams.get('accessToken') || '',
      };
      setUserData(data);
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
