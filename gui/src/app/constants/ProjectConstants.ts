import imagePath from '@/app/imagePath';

export const frameworkOptions = [
  { id: 'flask', text: 'Flask', src: imagePath.flaskImage, available: true },
  {
    id: 'django',
    text: 'Django',
    src: imagePath.djangoImage,
    available: true,
  },
  {
    id: 'fast_api',
    text: 'Fast API',
    src: imagePath.fastAPIImage,
    available: false,
  },
];
