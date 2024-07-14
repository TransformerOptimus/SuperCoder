import React, { useState, useRef, useEffect } from 'react';
import { Button } from '@nextui-org/react';
import imagePath from '@/app/imagePath';
import {
  CreateDesignStoryPayload,
  CreateEditDesignStoryProps,
  EditDesignStoryPayload,
} from '../../../types/designStoryTypes';
import CustomImage from '@/components/ImageComponents/CustomImage';
import styles from './desingStory.module.css';
import { createDesignStory, editDesignStory } from '@/api/DashboardService';
import { useDesignContext } from '@/context/Design';
import toast from 'react-hot-toast';
import InputSection from '@/components/StoryComponents/InputSection';

const CreateEditDesignStory: React.FC<CreateEditDesignStoryProps> = ({
  id,
  close,
  top,
  toGetAllDesignStoriesOfProject,
}) => {
  const titleRef = useRef<HTMLTextAreaElement>(null);
  const [uploadedImage, setUploadedImage] = useState<Blob | null>(null);
  const [imageName, setImageName] = useState<string>('');
  const [imagePreview, setImagePreview] = useState<string | ArrayBuffer | null>(
    null,
  );
  const [isLoading, setIsLoading] = useState<boolean | null>(false);
  const { editTrue, setEditTrue, selectedStory, openCreateStoryModal } =
    useDesignContext();

  useEffect(() => {
    if (openCreateStoryModal) {
      if (editTrue && selectedStory) {
        titleRef.current.value = selectedStory.title;
        setImagePreview(selectedStory.input_file_url);
        setImageName(selectedStory.input_file_url.split('/').at(-1));
      } else {
        titleRef.current.value = '';
        removeImage();
      }
    } else {
      setEditTrue(false);
    }
  }, [openCreateStoryModal]);

  const handleSubmit = () => {
    setIsLoading(true);
    if (editTrue) {
      const data = {
        story_id: selectedStory.id,
        title: titleRef.current?.value || '',
        file: uploadedImage,
        imageName: imageName,
      };
      toEditStory(data)
        .then(() => close())
        .catch();
    } else {
      const data = {
        project_id: localStorage.getItem('projectId'),
        title: titleRef.current?.value || '',
        file: uploadedImage,
        imageName: imageName,
      };
      toCreateStory(data)
        .then(() => close())
        .catch();
    }
  };

  async function toCreateStory(payload: CreateDesignStoryPayload) {
    try {
      const response = await createDesignStory(payload);
      if (response) {
        const data = response.data;
        toGetAllDesignStoriesOfProject();
        setIsLoading(false);
      }
    } catch (error) {
      console.error('Error while creating story:: ', error);
      setIsLoading(false);
    }
  }

  async function toEditStory(payload: EditDesignStoryPayload) {
    try {
      const response = await editDesignStory(payload);
      if (response) {
        const data = response.data;
        toGetAllDesignStoriesOfProject();
        setEditTrue(false);
      }
    } catch (error) {
      console.error('Error while editing story:: ', error);
      setEditTrue(false);
      setIsLoading(false);
    }
  }

  const handleImageUpload = (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    if (file) {
      const fileSizeInMB = file.size / (1024 * 1024);
      if (fileSizeInMB > 5) {
        toast.error('File size should be less than 5MB.');
        event.target.value = '';
        return;
      }
      const fileType = file.type.toLowerCase();
      const fileName = file.name.toLowerCase();
      if (
        fileType === 'image/png' ||
        (fileType === 'image/jpeg' && fileName.endsWith('.jpg'))
      ) {
        setUploadedImage(file);
        const reader = new FileReader();
        reader.onloadend = () => {
          setImagePreview(reader.result);
          setImageName(file.name);
        };
        reader.readAsDataURL(file);
      } else {
        toast.error('Please upload only JPEG or PNG images.');
        event.target.value = '';
      }
    }
  };
  const removeImage = () => {
    setUploadedImage(null);
    setImagePreview(null);
    setImageName('');
    const fileInput = document.getElementById(
      'image-upload-input',
    ) as HTMLInputElement;
    if (fileInput) {
      fileInput.value = '';
    }
  };
  const triggerFileInput = () => {
    const fileInput = document.getElementById('image-upload-input');
    if (fileInput) {
      fileInput.click();
    }
  };

  return (
    <div
      id={`${id}_new_story`}
      className={`${styles.new_story_container} proxima_nova flex flex-col text-white`}
      style={{ height: `calc(100% - ${top})` }}
    >
      <div
        id={'new_story_header'}
        className={`${styles.new_story_header} flex flex-shrink-0 flex-row items-center justify-between px-4 py-6`}
      >
        <span className={'text-xl font-medium'}>New Story</span>

        <CustomImage
          className={'size-5'}
          src={imagePath.closeIcon}
          alt={'close_icon'}
          onClick={close}
        />
      </div>

      <div
        className={`flex flex-grow flex-col gap-6 overflow-y-auto px-4 py-6`}
      >
        <InputSection
          id={'title'}
          label={'Title'}
          placeholder={'Enter title'}
          ref={titleRef}
        />
        <div className={'flex flex-col gap-2'}>
          <span className={'text-[13px] font-normal opacity-40'}>
            Add Design
          </span>
          <div>
            {imagePreview ? (
              <div className={'flex flex-col gap-4'}>
                <div
                  className={`relative flex max-h-[50vh] justify-center overflow-hidden rounded-lg ${styles.story_image_container}`}
                >
                  <img
                    className={'max-h-full max-w-full object-contain'}
                    src={imagePreview as string}
                    alt={'Uploaded_image'}
                  />
                </div>
                <div className="flex flex-row justify-between">
                  <span className="text-[13px] font-normal opacity-40">
                    {imageName && imageName}
                  </span>
                  <Button
                    className={`flex h-fit flex-row items-center gap-1 bg-transparent p-0 text-[13px] font-normal ${styles.red_color}`}
                    onClick={removeImage}
                  >
                    <CustomImage
                      className={'my-auto size-3'}
                      src={imagePath.redCrossDeleteIcon}
                      alt={'remove'}
                    />
                    Remove
                  </Button>
                </div>
              </div>
            ) : (
              <div
                className={`${styles.upload_button} flex h-32 cursor-pointer flex-row items-center justify-center gap-2 rounded-lg border-2 border-dashed`}
                onClick={triggerFileInput}
              >
                <CustomImage
                  className={'size-5'}
                  src={imagePath.uploadIcon}
                  alt={'upload_icon'}
                />
                <span>Upload Image</span>
              </div>
            )}
            <input
              type="file"
              id="image-upload-input"
              className="hidden"
              accept="image/*"
              onChange={handleImageUpload}
            />
          </div>
        </div>
      </div>
      <div
        className={`${styles.new_story_footer} flex flex-shrink-0 items-start justify-end gap-2 self-stretch p-4`}
      >
        <Button
          onClick={handleSubmit}
          className={'primary_medium w-fit'}
          isLoading={isLoading}
        >
          {editTrue ? 'Update' : 'Create'}
        </Button>
      </div>
    </div>
  );
};

export default CreateEditDesignStory;
