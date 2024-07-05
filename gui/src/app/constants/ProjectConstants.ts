import imagePath from '@/app/imagePath';

export const projectTypes = {
  BACKEND: 'BACKEND',
  DESIGN: 'DESIGN',
};

export const frameworkOptions = [
  {
    id: 'flask',
    text: 'Flask',
    src: imagePath.flaskImage,
    available: true,
    type: projectTypes.BACKEND,
  },
  {
    id: 'nextjs',
    text: 'Next Js',
    src: imagePath.nextJsImage,
    available: true,
    type: projectTypes.DESIGN,
  },
  {
    id: 'django',
    text: 'Django',
    src: imagePath.djangoImage,
    available: true,
    type: projectTypes.BACKEND,
  },
  {
    id: 'fast_api',
    text: 'Fast API',
    src: imagePath.fastAPIImage,
    available: false,
    type: projectTypes.BACKEND,
  },
];
