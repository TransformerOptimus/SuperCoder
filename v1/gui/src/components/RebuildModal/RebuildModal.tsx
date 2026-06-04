import { Button } from '@nextui-org/react';
import CustomModal from '@/components/CustomModal/CustomModal';
import { ReBuildModalProps } from '../../../types/customComponentTypes';

const ReBuildModal: React.FC<ReBuildModalProps> = ({
  openRebuildModal,
  setOpenRebuildModal,
  rebuildComment,
  setRebuildComment,
  handleRebuildStory,
}) => {
  return (
    <CustomModal
      isOpen={openRebuildModal}
      onClose={() => setOpenRebuildModal(false)}
      width={'40vw'}
    >
      <CustomModal.Header title={'Add comment to Re-Build'} />
      <CustomModal.Body padding={'24px 16px'}>
        <div className={'flex flex-col gap-2'}>
          <span className={'secondary_color text-[13px] font-normal'}>
            Comment
          </span>
          <textarea
            value={rebuildComment}
            className={'textarea_large'}
            placeholder={'Write down the comment here..'}
            onChange={(event) => setRebuildComment(event.target.value)}
          />
        </div>
      </CustomModal.Body>
      <CustomModal.Footer>
        <Button
          className={'primary_medium'}
          onClick={() => handleRebuildStory()}
        >
          Re-Build with comment
        </Button>
      </CustomModal.Footer>
    </CustomModal>
  );
};

export default ReBuildModal;
