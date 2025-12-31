'use client';
import React, { useEffect, useState } from 'react';
import { Button } from '@nextui-org/react';
import { createOrUpdateLLMAPIKey, getLLMAPIKeys } from '@/api/DashboardService';
import toast from 'react-hot-toast';
import CustomInput from '@/components/CustomInput/CustomInput';
import imagePath from '@/app/imagePath';
import { ModelsList } from '../../../../types/settingTypes';
import { validateOpenAIKey } from '@/app/utils';

export default function Models() {
  const [modelsList, setModelsList] = useState<ModelsList[] | null>(null);
  const [validationErrors, setValidationErrors] = useState<
    Record<string, string>
  >({});
  const modelDetails = {
    'gpt-4o': { text: 'Open AI API Key (gpt-4o)', icon: imagePath.openAIIcon },
    'claude-3': {
      text: 'Anthropic API Key (claude-3.4-sonnet)',
      icon: imagePath.claudeIcon,
    },
  };

  const validateApiKeys = (): boolean => {
    const errors: Record<string, string> = {};

    modelsList?.forEach((model) => {
      if (model.model_name === 'gpt-4o' && !validateOpenAIKey(model.api_key)) {
        errors[model.model_name] =
          'Invalid OpenAI API key. Key must start with "sk-" and be at least 20 characters.';
      }
    });

    setValidationErrors(errors);
    return Object.keys(errors).length === 0;
  };

  const handleButtonClick = () => {
    if (!validateApiKeys()) {
      return;
    }
    toCreateOrUpdateLLMAPIKey().then().catch();
  };

  const handleApiKeyChange = (model_name: string, value: string) => {
    setModelsList(
      (prev) =>
        prev?.map((model) =>
          model.model_name === model_name
            ? { ...model, api_key: value }
            : model,
        ) || null,
    );
    if (validationErrors[model_name]) {
      setValidationErrors((prev) => {
        const newErrors = { ...prev };
        delete newErrors[model_name];
        return newErrors;
      });
    }
  };

  useEffect(() => {
    toGetLLMAPIKeys().then().catch();
  }, []);

  async function toGetLLMAPIKeys() {
    try {
      const response = await getLLMAPIKeys();
      if (response) {
        const data = response.data.map((model: ModelsList) => ({
          ...model,
          ...modelDetails[model.model_name],
        }));
        setModelsList(data);
      }
    } catch (error) {
      console.error('Error while fetching LLM API Keys: ', error);
    }
  }

  async function toCreateOrUpdateLLMAPIKey() {
    try {
      const apiKeys = modelsList.map((model) => ({
        llm_model: model.model_name,
        llm_api_key: model.api_key,
      }));

      const payload = {
        api_keys: apiKeys,
      };

      const response = await createOrUpdateLLMAPIKey(payload);
      if (response) {
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
        Setup Models
      </span>
      <div id={'api_key_section'} className={'flex flex-col gap-6'}>
        {modelsList?.map((model) => (
          <div key={model.model_name} className={'flex flex-col gap-2'}>
            <span className={'secondary_color text-[13px] font-medium'}>
              {model.text}
            </span>
            <CustomInput
              format={'password'}
              value={model.api_key || ''}
              setter={(value) => handleApiKeyChange(model.model_name, value)}
              placeholder={'Enter API Key here'}
              icon={model.icon}
              iconCSS={'size-4'}
              alt={`${model.model_name}_icon`}
              isError={!!validationErrors[model.model_name]}
              errorMessage={validationErrors[model.model_name]}
            />
          </div>
        ))}
      </div>
      <Button className={'primary_medium w-fit'} onClick={handleButtonClick}>
        Update Changes
      </Button>
    </div>
  );
}
