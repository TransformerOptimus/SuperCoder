'use client';
import React, { createContext, ReactNode, useState } from 'react';
import { DesignStoryItem } from '../../types/designStoryTypes';

interface DesignContextProps {
  openCreateStoryModal: boolean;
  setOpenCreateStoryModal: React.Dispatch<React.SetStateAction<boolean>>;
  openStoryDetailsModal: boolean;
  setOpenStoryDetailsModal: React.Dispatch<React.SetStateAction<boolean>>;
  storyList: DesignStoryItem[] | null;
  setStoryList: React.Dispatch<React.SetStateAction<DesignStoryItem[]>>;
  selectedStory: DesignStoryItem;
  setSelectedStory: React.Dispatch<
    React.SetStateAction<DesignStoryItem | null>
  >;
  editTrue: boolean;
  setEditTrue: React.Dispatch<React.SetStateAction<boolean>>;
}

const DesignContext = createContext<DesignContextProps | undefined>(undefined);

export const DesignProvider: React.FC<{ children: ReactNode }> = ({
  children,
}) => {
  const [openCreateStoryModal, setOpenCreateStoryModal] = useState<
    boolean | null
  >(false);
  const [openStoryDetailsModal, setOpenStoryDetailsModal] = useState<
    boolean | null
  >(false);
  const [editTrue, setEditTrue] = useState<boolean | null>(false);
  const [storyList, setStoryList] = useState<DesignStoryItem[]>(null);
  const [selectedStory, setSelectedStory] = useState<DesignStoryItem | null>(
    null,
  );
  return (
    <DesignContext.Provider
      value={{
        openCreateStoryModal,
        setOpenCreateStoryModal,
        openStoryDetailsModal,
        setOpenStoryDetailsModal,
        storyList,
        setStoryList,
        selectedStory,
        setSelectedStory,
        editTrue,
        setEditTrue,
      }}
    >
      {children}
    </DesignContext.Provider>
  );
};

export const useDesignContext = () => {
  const context = React.useContext(DesignContext);
  if (!context) {
    throw new Error('useDesignContext must be used within a DesignProvider');
  }
  return context;
};
