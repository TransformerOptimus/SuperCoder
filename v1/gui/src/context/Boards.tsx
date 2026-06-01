'use client';
import React, { createContext, useState, ReactNode } from 'react';
import { OverviewContent, StoryDetailsAPIData } from '../../types/storyTypes';

interface BoardContextProps {
  storyDetails: StoryDetailsAPIData | null;
  setStoryDetails: React.Dispatch<
    React.SetStateAction<StoryDetailsAPIData | null>
  >;
  storyOverview: OverviewContent | null;
  setStoryOverview: React.Dispatch<
    React.SetStateAction<OverviewContent | null>
  >;
  storyTestCases: string[] | null;
  setStoryTestCases: React.Dispatch<React.SetStateAction<string[] | null>>;
  storyInstructions: string[] | null;
  setStoryInstructions: React.Dispatch<React.SetStateAction<string[] | null>>;
  editTrue: boolean;
  setEditTrue: React.Dispatch<React.SetStateAction<boolean | null>>;
}

const defaultState: BoardContextProps = {
  storyDetails: null,
  setStoryDetails: () => {},
  storyOverview: null,
  setStoryOverview: () => {},
  storyTestCases: null,
  setStoryTestCases: () => {},
  storyInstructions: null,
  setStoryInstructions: () => {},
  editTrue: false,
  setEditTrue: () => {},
};

const BoardContext = createContext<BoardContextProps | undefined>(defaultState);

export const BoardProvider: React.FC<{ children: ReactNode }> = ({
  children,
}) => {
  const [storyDetails, setStoryDetails] = useState<StoryDetailsAPIData | null>(
    null,
  );
  const [storyOverview, setStoryOverview] = useState<OverviewContent | null>(
    null,
  );
  const [storyTestCases, setStoryTestCases] = useState<string[] | null>(null);
  const [storyInstructions, setStoryInstructions] = useState<string[] | null>(
    null,
  );
  const [editTrue, setEditTrue] = useState<boolean | null>(false);

  return (
    <BoardContext.Provider
      value={{
        storyDetails,
        setStoryDetails,
        storyOverview,
        setStoryOverview,
        storyTestCases,
        setStoryTestCases,
        storyInstructions,
        setStoryInstructions,
        editTrue,
        setEditTrue,
      }}
    >
      {children}
    </BoardContext.Provider>
  );
};

export const useBoardContext = () => {
  const context = React.useContext(BoardContext);
  if (!context) {
    throw new Error('useBoardContext must be used within a BoardProvider');
  }
  return context;
};
