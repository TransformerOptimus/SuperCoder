export interface CustomImageProps {
  className: string;
  src: string;
  alt: string;
  priority?: boolean;
  onClick?: () => void;
}

export interface CustomTextImageProps {
  gap: string;
  textCSS: string;
  text: string;
  imageCSS: string;
  src: string;
  alt: string;
  priority?: boolean;
}

export interface ImageOptions {
  id: string;
  src: string;
  text: string;
  available: boolean;
}

export interface CustomImageSelectorProps {
  size: string;
  gap: string;
  imageOptions: ImageOptions[];
  selectedOption: string;
  onSelectOption: (id: string) => void;
}
