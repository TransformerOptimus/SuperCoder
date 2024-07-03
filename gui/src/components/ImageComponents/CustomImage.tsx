import Image from 'next/image';
import { CustomImageProps } from '../../../types/imageComponentsTypes';

export default function CustomImage({
  className,
  src,
  alt,
  priority = false,
  onClick,
}: CustomImageProps) {
  return (
    <Image
      width={0}
      height={0}
      sizes={'100vw'}
      className={className}
      src={src}
      alt={alt}
      priority={priority}
      onClick={onClick}
    />
  );
}
