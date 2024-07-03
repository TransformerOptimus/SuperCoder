import { CustomTextImageProps } from '../../../types/imageComponentsTypes';
import CustomImage from '@/components/ImageComponents/CustomImage';

export default function CustomTextImage({
  gap,
  textCSS,
  text,
  imageCSS,
  src,
  alt,
  priority,
}: CustomTextImageProps) {
  return (
    <div id={'react_text_image'} className={`flex flex-row ${gap}`}>
      <CustomImage
        className={imageCSS}
        src={src}
        alt={alt}
        priority={priority}
      />
      <span className={textCSS}>{text}</span>
    </div>
  );
}
