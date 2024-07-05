import React, { useState, useRef, useEffect } from 'react';
import { Button } from '@nextui-org/react';
import imagePath from '@/app/imagePath';
import {
  DynamicInputSectionProps,
  CreateEditStoryProps,
} from '../../../types/storyTypes';
import CustomImage from '@/components/ImageComponents/CustomImage';
import styles from './story.module.css';
import { FormStoryPayload } from '../../../types/storyTypes';
import { createStory, editStory } from '@/api/DashboardService';
import { useBoardContext } from '@/context/Boards';
import InputSection from '@/components/StoryComponents/InputSection';

const DynamicInputSection: React.FC<DynamicInputSectionProps> = ({
  id,
  label,
  inputs,
  placeholder,
  setInputs,
  buttonText,
}) => {
  const addInput = () => setInputs([...inputs, '']);

  const removeInput = (index: number) =>
    setInputs(inputs.filter((_, i) => i !== index));

  const handleChange = (index: number, value: string) => {
    const newInputs = [...inputs];
    newInputs[index] = value;
    setInputs(newInputs);
  };

  return (
    <div id={id} className={'flex flex-col'}>
      <span className={'text-[13px] font-normal opacity-40'}>{label}</span>

      <div className={'mb-2 flex flex-col gap-2'}>
        {inputs.map((input, index) => (
          <div
            key={index}
            className={'flex flex-row items-start justify-center gap-2'}
          >
            <textarea
              className={'textarea_medium w-full'}
              value={input}
              onChange={(e) => handleChange(index, e.target.value)}
              placeholder={placeholder}
            />

            <Button
              className={'secondary_medium w-fit px-0'}
              onClick={() => removeInput(index)}
              isIconOnly
            >
              <CustomImage
                className={'size-5'}
                src={imagePath.closeIcon}
                alt={'close_icon'}
              />
            </Button>
          </div>
        ))}
      </div>

      <Button className={'secondary_medium w-fit'} onClick={addInput}>
        <CustomImage
          className={'size-5'}
          src={imagePath.addIcon}
          alt={'add_icon'}
        />

        {buttonText}
      </Button>
    </div>
  );
};

const CreateEditStory: React.FC<CreateEditStoryProps> = ({
  id,
  close,
  top,
  story_id,
  toGetAllStoriesOfProject,
}) => {
  const [testCases, setTestCases] = useState<string[]>(['']);
  const summaryRef = useRef<HTMLTextAreaElement>(null);
  const descriptionRef = useRef<HTMLTextAreaElement>(null);
  const devInstructionsRef = useRef<HTMLTextAreaElement>(null);
  const { editTrue, setEditTrue, storyDetails } = useBoardContext();

  const handleSubmit = () => {
    const data = {
      story_id: story_id,
      project_id: Number(localStorage.getItem('projectId')),
      summary: summaryRef.current?.value || '',
      description: descriptionRef.current?.value || '',
      test_cases: testCases,
      instructions: devInstructionsRef.current?.value || '',
    };

    if (editTrue) toEditStory(data).then().catch();
    else toCreateStory(data).then().catch();
  };

  const handleReset = () => {
    summaryRef.current.value = '';
    descriptionRef.current.value = '';
    devInstructionsRef.current.value = '';
    setTestCases([]);
  };

  useEffect(() => {
    if (editTrue && storyDetails) {
      if (summaryRef.current)
        summaryRef.current.value = storyDetails.overview.name;
      if (descriptionRef.current)
        descriptionRef.current.value = storyDetails.overview.description;
      if (devInstructionsRef.current)
        devInstructionsRef.current.value = storyDetails.instructions;
      setTestCases(storyDetails.test_cases);
    } else {
      handleReset();
    }
  }, [editTrue, storyDetails]);

  async function toCreateStory(payload: FormStoryPayload) {
    try {
      const response = await createStory(payload);
      if (response) {
        const data = response.data;
        toGetAllStoriesOfProject();
        console.log(data);
        setTimeout(() => {
          handleReset();
        }, 500);
        close();
      }
    } catch (error) {
      console.error('Error while creating story:: ', error);
      close();
    }
  }

  async function toEditStory(payload: FormStoryPayload) {
    try {
      const response = await editStory(payload);
      if (response) {
        const data = response.data;
        toGetAllStoriesOfProject();
        console.log(data);
      }
    } catch (error) {
      console.error('Error while editing story:: ', error);
    } finally {
      setTimeout(() => {
        setEditTrue(false);
      }, 500);
      close();
    }
  }

  return (
    <div
      id={`${id}_new_story`}
      className={`${styles.new_story_container} proxima_nova text-white`}
      style={{ height: `calc(100% - ${top})` }}
    >
      <div id={'new_story_header'} className={styles.new_story_header}>
        <span className={'text-xl font-medium'}>
          {editTrue ? 'Edit Story' : 'New Story'}
        </span>

        <CustomImage
          className={'size-5 cursor-pointer'}
          src={imagePath.closeIcon}
          alt={'close_icon'}
          onClick={close}
        />
      </div>

      <div className={styles.new_story_body}>
        <InputSection
          id={'summary'}
          label={'Summary'}
          placeholder={'Enter Summary'}
          ref={summaryRef}
        />

        <InputSection
          id={'description'}
          label={'Description'}
          placeholder={'Enter Description'}
          isTextArea
          ref={descriptionRef}
        />

        <DynamicInputSection
          id={'test_cases'}
          label={'Test Cases'}
          inputs={testCases}
          setInputs={setTestCases}
          placeholder={'Enter test case'}
          buttonText={'Add test case'}
        />

        <InputSection
          id={'dev_instructions'}
          label={'Dev Instructions'}
          placeholder={'Enter Dev Instructions'}
          isTextArea
          ref={devInstructionsRef}
        />
      </div>

      <div className={styles.new_story_footer}>
        <Button onClick={handleSubmit} className={'primary_medium w-fit'}>
          {editTrue ? 'Save' : 'Create'}
        </Button>
      </div>
    </div>
  );
};

export default CreateEditStory;
