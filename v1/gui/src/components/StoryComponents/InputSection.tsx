import React, { forwardRef } from 'react';
import { InputSectionProps } from '../../../types/storyTypes';

const InputSection = forwardRef<
  HTMLTextAreaElement | HTMLInputElement,
  InputSectionProps
>(({ id, label, type = 'text', isTextArea = false, placeholder }, ref) => (
  <div id={id} className={'flex flex-col'}>
    <span className={'text-[13px] font-normal opacity-40'}>{label}</span>

    {isTextArea ? (
      <textarea
        className={'textarea_medium w-full'}
        placeholder={placeholder}
        ref={ref as React.Ref<HTMLTextAreaElement>}
      />
    ) : (
      <input
        type={type}
        className={'input_medium w-full'}
        placeholder={placeholder}
        ref={ref as React.Ref<HTMLInputElement>}
      />
    )}
  </div>
));
InputSection.displayName = 'InputSection';

export default InputSection;
