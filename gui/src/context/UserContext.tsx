'use client';

import React, { createContext, useEffect, useState } from 'react';
import { UserData } from '../../types/authTypes';
import { getUserDetails } from '@/api/DashboardService';
import Cookies from 'js-cookie';

interface UserContextType {
  id: number;
  email: string;
  name: string;
  organisation_id: number;
  fetchUserDetails: () => Promise<void>;
}

export const UserContext = createContext<UserContextType | undefined>(
  undefined,
);

export const UserContextProvider: React.FC<{ children: React.ReactNode }> = ({
  children,
}) => {
  const [userDetails, setUserDetails] = useState<UserData | undefined>(
    undefined,
  );

  const fetchUserDetails = async () => {
    try {
      const token = Cookies.get('token');
      if (token) {
        const details = await getUserDetails();
        setUserDetails(details);
      }
    } catch (e) {
      console.error('Error fetching user details: ', e);
    }
  };

  useEffect(() => {
    fetchUserDetails();
  }, []);

  return (
    <UserContext.Provider value={{ ...userDetails, fetchUserDetails }}>
      {children}
    </UserContext.Provider>
  );
};
