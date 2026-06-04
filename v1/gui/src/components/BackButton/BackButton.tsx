import CustomImage from '@/components/ImageComponents/CustomImage';
import imagePath from '@/app/imagePath';
import { useRouter } from 'next/navigation';

interface BackButtonProps {
  id: string;
  url: string;
}

export default function BackButton({ id, url }: BackButtonProps) {
  const router = useRouter();

  return (
    <div
      id={`${id}_page_back_button`}
      className={
        'flex w-fit cursor-pointer flex-row items-center justify-center gap-1'
      }
      onClick={() => router.push(url)}
    >
      <CustomImage
        className={'size-4'}
        src={imagePath.backArrow}
        alt={'back_button'}
      />
      <span className={'secondary_color proxima_nova text-sm font-normal'}>
        Back
      </span>
    </div>
  );
}
