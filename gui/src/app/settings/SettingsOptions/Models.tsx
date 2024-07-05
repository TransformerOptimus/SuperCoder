'use client';
import CustomSelect from '@/components/CustomSelect/CustomSelect';
import React, { useEffect, useState } from 'react';
import { Button } from '@nextui-org/react';
import { createOrUpdateLLMAPIKey, getLLMAPIKeys } from '@/api/DashboardService';
import toast from 'react-hot-toast';

interface ModelsList {
  model_name: string;
  api_key: string;
}

export default function Models() {
  const [selectedValue, setSelectedValue] = useState<string>('');
  const [apiKey, setApiKey] = useState<string | null>('');
  const [modelsList, setModelsList] = useState<ModelsList[] | null>(null);

  const handleButtonClick = () => {
    toCreateOrUpdateLLMAPIKey().then().catch();
  };

  useEffect(() => {
    toGetLLMAPIKeys().then().catch();
  }, []);

  useEffect(() => {
    if (modelsList && selectedValue.length > 0) {
      const selectedModel = modelsList.find(
        (model) => model.model_name === selectedValue,
      );
      if (selectedModel) {
        setApiKey(selectedModel.api_key);
      }
    }
  }, [selectedValue]);

  useEffect(() => {
    if (modelsList) setSelectedValue(modelsList[0].model_name);
  }, [modelsList]);

  async function toGetLLMAPIKeys() {
    try {
      const organisation_id = localStorage.getItem('organisationId');
      const response = await getLLMAPIKeys(organisation_id);
      if (response) {
        const data = response.data;
        console.log(data);
        setModelsList(data);
      }
    } catch (error) {
      console.error('Error while fetching LLM API Keys: ', error);
    }
  }

  async function toCreateOrUpdateLLMAPIKey() {
    try {
      let id = null;
      if (typeof window !== 'undefined') {
        id = localStorage.getItem('organisationId');
      }
      const payload = {
        organisation_id: Number(id),
        llm_model: selectedValue,
        llm_api_key: apiKey,
      };
      const response = await createOrUpdateLLMAPIKey(payload);
      if (response) {
        const data = response.data;
        console.log(data);
        toast.success('Model is setup successfully');
        toGetLLMAPIKeys().then().catch();
      }
    } catch (error) {
      toast.error(error.message, {
        style: { maxWidth: 'none', whiteSpace: 'nowrap' },
      });
      console.error('Error while creating or updating LLM API Key: ', error);
    }
  }

  return (
    <div id={'models_section'} className={'proxima_nova flex flex-col gap-6'}>
      <span id={'title'} className={'text-xl font-semibold text-white'}>
        {' '}
        Setup Model{' '}
      </span>
      <div id={'model_selection_section'} className={'flex flex-col gap-2'}>
        <span className={'secondary_color text-[13px] font-medium'}>Model</span>
        <CustomSelect
          selectedValues={selectedValue}
          setSelectedValues={setSelectedValue}
        >
          {modelsList &&
            modelsList.length > 0 &&
            modelsList.map((model, index) => (
              <CustomSelect.Item key={index} value={model.model_name}>
                {model.model_name}
              </CustomSelect.Item>
            ))}
        </CustomSelect>
      </div>
      <div id={'api_key_section'} className={'flex flex-col gap-2'}>
        <span className={'secondary_color text-[13px] font-medium'}>
          {' '}
          Open AI API Key{' '}
        </span>
        <input
          value={apiKey}
          type={'password'}
          className={'input_medium'}
          placeholder={'Enter API Key here'}
          onChange={(event) => setApiKey(event.target.value)}
        />
      </div>
      <Button className={'primary_medium w-fit'} onClick={handleButtonClick}>
        {' '}
        Update Changes{' '}
      </Button>
    </div>
  );
}
