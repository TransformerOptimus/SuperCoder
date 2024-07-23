'use client';
import React from 'react';
import Models from '@/app/settings/SettingsOptions/Models';
import CustomSidebar from '@/components/CustomSidebar/CustomSidebar';
import imagePath from '@/app/imagePath';
import BackButton from '@/components/BackButton/BackButton';
import Integrations from "@/app/settings/SettingsOptions/Integrations";

export default function Settings() {
  const options = [
    {
      key: 'models',
      text: 'Models',
      selected: imagePath.modelsIconSelected,
      unselected: imagePath.modelsIconUnselected,
      icon_css: 'size-4',
      component: <Models/>,
    }, {
      key: 'integrations',
      text: 'Integrations',
      selected: imagePath.modelsIconSelected,
      unselected: imagePath.modelsIconUnselected,
      icon_css: 'size-4',
      component: <Integrations/>,
    },
  ];

  const handleOptionSelect = (key: string) => {
    console.log('Selected option:', key);
  };

  return (
    <div className={'mx-[20vw] my-8 flex flex-col gap-3'}>
      <BackButton id={'settings'} url={'/board'}/>

      <CustomSidebar
        id={'settings'}
        title={'Settings'}
        options={options}
        onOptionSelect={handleOptionSelect}
      />
    </div>
  );
}
