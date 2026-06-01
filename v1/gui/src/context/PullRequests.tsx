'use client';
import React, { createContext, ReactNode, useState } from 'react';
import { PRListItems } from '../../types/pullRequestsTypes';

interface PullRequestsContextProps {
  prList: PRListItems[];
  setPRList: React.Dispatch<React.SetStateAction<PRListItems[]>>;
}

const PullRequestsContext = createContext<PullRequestsContextProps | undefined>(
  undefined,
);

export const PullRequestsProvider: React.FC<{ children: ReactNode }> = ({
  children,
}) => {
  const [prList, setPRList] = useState<PRListItems[] | null>(null);
  return (
    <PullRequestsContext.Provider value={{ prList, setPRList }}>
      {children}
    </PullRequestsContext.Provider>
  );
};

export const usePullRequestsContext = () => {
  const context = React.useContext(PullRequestsContext);

  if (!context) {
    throw new Error(
      'usePullRequestsContext must be used within a BoardProvider',
    );
  }

  return context;
};
