'use client';
import React from 'react';
import Models from '@/app/settings/SettingsOptions/Models';
import CustomSidebar from '@/components/CustomSidebar/CustomSidebar';
import imagePath from '@/app/imagePath';
import Users from '@/app/settings/SettingsOptions/Users';

export default function Settings() {
  const options = [
    {
      key: 'models',
      text: 'Models',
      selected: imagePath.modelsIconSelected,
      unselected: imagePath.modelsIconUnselected,
      icon_css: 'size-4',
      component: <Models />,
    },
    {
      key: 'users',
      text: 'Users',
      selected: imagePath.usersIconSelected,
      unselected: imagePath.usersIconUnselected,
      icon_css: 'size-4',
      component: <Users />,
    },
  ];

  const handleOptionSelect = (key: string) => {
    console.log('Selected option:', key);
  };

  return (
    <div className={'mx-[20vw] my-8'}>
      <CustomSidebar
        id={'settings'}
        title={'Settings'}
        options={options}
        onOptionSelect={handleOptionSelect}
      />
    </div>
  );
}
