'use client';

import React, { createContext, useContext, useEffect, useState } from 'react';
import { UserData } from '../../types/authTypes';
import { getUserDetails } from '@/api/DashboardService';
import Cookies from 'js-cookie';
import toast from 'react-hot-toast';

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
    const token = Cookies.get('token');
    if (token) {
      try {
        const details = await getUserDetails();
        setUserDetails(details);
      } catch (e) {
        toast.error('Error fetching user details');
      }
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

export const useUserContext = () => {
  const context = useContext(UserContext);
  if (!context) {
    throw new Error('useUserContext must be used within a UserContextProvider');
  }
  return context;
};
