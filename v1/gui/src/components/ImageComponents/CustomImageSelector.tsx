import {
  CustomImageSelectorProps,
  ImageOptions,
} from '../../../types/imageComponentsTypes';
import Image from 'next/image';
import styles from './image.module.css';
import { memo } from 'react';

function CustomImageSelector({
  size,
  gap,
  imageOptions,
  selectedOption,
  onSelectOption,
}: CustomImageSelectorProps) {
  const handleImageOptionClick = (option: ImageOptions) => {
    if (option.available) onSelectOption(option.id);
  };

  return (
    <div
      id={'custom_image_selector'}
      className={`proxima_nova flex flex-row items-start justify-start text-white`}
      style={{ gap: gap }}
    >
      {imageOptions &&
        imageOptions.map((image, index) => (
          <div
            className={'size flex flex-col items-center gap-3'}
            key={index}
            onClick={() => handleImageOptionClick(image)}
            id={image.id + '_image_option'}
          >
            <Image
              width={0}
              height={0}
              sizes={'100vw'}
              className={`${
                selectedOption === image.id
                  ? styles.image_option_selected
                  : styles.image_option
              } ${
                image.available ? 'cursor-pointer' : 'cursor-not-allowed'
              } rounded-xl`}
              src={image.src}
              alt={image.text.toLowerCase().replace(' ', '_') + '_image'}
              style={{ width: size, height: size }}
            />

            <div className={'flex flex-col items-center'}>
              <span className={'text-[13px] font-normal'}>{image.text}</span>

              {!image.available && (
                <span className={'text-[9px] font-normal opacity-40'}>
                  {' '}
                  Coming soon..{' '}
                </span>
              )}
            </div>
          </div>
        ))}
    </div>
  );
}

export default memo(CustomImageSelector);
