'use client';
import { StoryList } from '../../types/workbenchTypes';
import React, { createContext, ReactNode, useState } from 'react';

interface WorkbenchContextProps {
  storiesList: StoryList;
  setStoriesList: React.Dispatch<React.SetStateAction<StoryList>>;
}

const defaultState: WorkbenchContextProps = {
  storiesList: null,
  setStoriesList: () => {},
};

const WorkbenchContext = createContext<WorkbenchContextProps | null>(
  defaultState,
);

export const WorkbenchProvider: React.FC<{ children: ReactNode }> = ({
  children,
}) => {
  const [storiesList, setStoriesList] = useState<StoryList | null>(null);

  return (
    <WorkbenchContext.Provider
      value={{
        storiesList,
        setStoriesList,
      }}
    >
      {children}
    </WorkbenchContext.Provider>
  );
};

export const useWorkbenchContext = () => {
  const context = React.useContext(WorkbenchContext);
  if (!context) {
    throw new Error(
      'useWorkbenchContext must be used within a WorkbenchProvider',
    );
  }
  return context;
};
