import CustomImage from '@/components/ImageComponents/CustomImage';
import imagePath from '@/app/imagePath';
import React from 'react';

export default function GithubIntegrationSymbol({
  className,
  ...props
}: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={`flex items-center justify-center bg-white ${className}`}
      {...props}
    >
      <CustomImage
        className={'h-full w-full'}
        src={imagePath.githubLogo}
        alt={'github_logo'}
      />
    </div>
  );
}
