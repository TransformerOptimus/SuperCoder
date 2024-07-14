'use client';
import { ActivityItem, StoryList } from '../../types/workbenchTypes';
import React, { createContext, ReactNode, useState } from 'react';

interface WorkbenchContextProps {
  storiesList: StoryList;
  setStoriesList: React.Dispatch<React.SetStateAction<StoryList>>;
  activityLogs: ActivityItem[];
  setActivityLogs: React.Dispatch<React.SetStateAction<ActivityItem[]>>;
  selectedStoryId: string;
  setSelectedStoryId: React.Dispatch<React.SetStateAction<string>>;
  executionInProcess: boolean;
  setExecutionInProcess: React.Dispatch<React.SetStateAction<boolean>>;
}

const defaultState: WorkbenchContextProps = {
  storiesList: null,
  setStoriesList: () => {},
  activityLogs: null,
  setActivityLogs: () => {},
  selectedStoryId: null,
  setSelectedStoryId: () => {},
  executionInProcess: null,
  setExecutionInProcess: () => {},
};

const WorkbenchContext = createContext<WorkbenchContextProps | null>(
  defaultState,
);

export const WorkbenchProvider: React.FC<{ children: ReactNode }> = ({
  children,
}) => {
  const [storiesList, setStoriesList] = useState<StoryList | null>(null);
  const [activityLogs, setActivityLogs] = useState<ActivityItem[]>(null);
  const [selectedStoryId, setSelectedStoryId] = useState<string>(null);
  const [executionInProcess, setExecutionInProcess] = useState<boolean | null>(
    null,
  );

  return (
    <WorkbenchContext.Provider
      value={{
        storiesList,
        setStoriesList,
        activityLogs,
        setActivityLogs,
        selectedStoryId,
        setSelectedStoryId,
        executionInProcess,
        setExecutionInProcess,
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
