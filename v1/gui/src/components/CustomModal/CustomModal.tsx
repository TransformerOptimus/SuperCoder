import { createContext, useContext, useEffect, useRef, ReactNode } from 'react';
import { CSSTransition } from 'react-transition-group';
import styles from './modal.module.css';
import Image from 'next/image';
import imagePath from '@/app/imagePath';

interface ModalProps {
  isOpen: boolean;
  onClose: () => void;
  width?: string;
  children: ReactNode;
}

interface CustomModalHeaderProps {
  title?: string;
  children?: ReactNode;
  padding?: string;
}

interface CustomModalBodyProps {
  children: ReactNode;
  padding?: string;
}

interface CustomModalFooterProps {
  children: ReactNode;
  padding?: string;
}

const OnCloseContext = createContext<() => void>(() => {});

const CustomModal = ({
  isOpen,
  onClose,
  width = '500px',
  children,
}: ModalProps) => {
  const modalRef = useRef<HTMLDivElement>(null);

  const handleClickOutside = (event: MouseEvent) => {
    if (modalRef.current && !modalRef.current.contains(event.target as Node)) {
      onClose();
    }
  };

  useEffect(() => {
    if (isOpen) {
      document.addEventListener('mousedown', handleClickOutside);
      document.body.style.overflow = 'hidden'; // Prevent background scrolling
    } else {
      document.removeEventListener('mousedown', handleClickOutside);
      document.body.style.overflow = 'unset'; // Enable background scrolling
    }
    return () => {
      document.removeEventListener('mousedown', handleClickOutside);
      document.body.style.overflow = 'unset'; // Cleanup
    };
  }, [isOpen]);

  return (
    <OnCloseContext.Provider value={onClose}>
      <CSSTransition
        in={isOpen}
        timeout={300}
        classNames={{
          enter: styles.modal_enter,
          enterActive: styles.modal_enter_active,
          exit: styles.modal_exit,
          exitActive: styles.modal_exit_active,
        }}
        unmountOnExit
      >
        <div className={styles.overlay}>
          <div className={styles.modal} ref={modalRef} style={{ width }}>
            {children}
          </div>
        </div>
      </CSSTransition>
    </OnCloseContext.Provider>
  );
};

const CustomModalHeader = ({
  title = 'Default Title',
  children,
  padding = '16px',
}: CustomModalHeaderProps) => {
  const onClose = useContext(OnCloseContext);

  return (
    <div
      className={`${styles.modal_header} proxima_nova`}
      style={{ padding: padding }}
    >
      {children ? (
        <div>{children}</div>
      ) : (
        <div className={'flex flex-row items-center justify-between'}>
          <span className={'text-base font-semibold'}>{title}</span>
          <Image
            width={24}
            height={24}
            className={'size-4 cursor-pointer'}
            src={imagePath.closeIcon}
            alt={'cross_icon'}
            onClick={onClose}
          />
        </div>
      )}
    </div>
  );
};

const CustomModalBody = ({
  children,
  padding = '16px',
}: CustomModalBodyProps) => {
  return (
    <div className={`${styles.modal_body}`} style={{ padding: padding }}>
      {children}
    </div>
  );
};

const CustomModalFooter = ({
  children,
  padding = '16px',
}: CustomModalFooterProps) => {
  return (
    <div className={styles.modal_footer} style={{ padding: padding }}>
      {children}
    </div>
  );
};

CustomModal.Header = CustomModalHeader;
CustomModal.Body = CustomModalBody;
CustomModal.Footer = CustomModalFooter;

export default CustomModal;
