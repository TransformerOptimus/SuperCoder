import CustomImage from '@/components/ImageComponents/CustomImage';
import { CustomTagProps } from '../../../types/customComponentTypes';
import styles from './tag.module.css';

export default function CustomTag({
  text,
  icon,
  iconClass,
  iconBack,
  iconBackClass,
  color = 'grey',
  children,
  className = 'rounded-3xl',
}: CustomTagProps) {
  const tagCSS = {
    grey: styles.grey_tag,
    purple: styles.purple_tag,
    yellow: styles.yellow_tag,
    green: styles.green_tag,
    red: styles.red_tag,
  };

  return (
    <div
      id={`${text.toLowerCase()}_tag`}
      className={`${tagCSS[color]} ${className} flex flex-row items-center gap-2`}
    >
      {icon && (
        <CustomImage
          className={iconClass}
          src={icon}
          alt={`${text.toLowerCase()}_tag_icon`}
        />
      )}
      <span className={'text-nowrap'}>{text}</span>
      {children}
      {iconBack && (
        <CustomImage
          className={iconBackClass}
          src={iconBack}
          alt={'back_icon'}
        />
      )}
    </div>
  );
}
