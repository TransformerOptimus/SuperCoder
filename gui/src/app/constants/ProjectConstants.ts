import imagePath from '@/app/imagePath';

export const storyTypes = {
  BACKEND: 'BACKEND',
  DESIGN: 'DESIGN',
};

export const backendFrameworkOptions = [
  {
    id: 'flask',
    text: 'Flask',
    src: imagePath.flaskImage,
    available: true,
  },
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

export const frontendFrameworkOptions = [
  {
    id: 'nextjs',
    text: 'Next Js',
    src: imagePath.nextJsImage,
    available: true,
  },
];
