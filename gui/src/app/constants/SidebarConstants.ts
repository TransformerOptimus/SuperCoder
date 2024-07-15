import { SidebarOption } from '../../../types/sidebarTypes';
import imagePath from '@/app/imagePath';

export const sidebarOptions: SidebarOption[] = [
  {
    id: 'board',
    text: 'Board',
    selected: imagePath.boardIconSelected,
    unselected: imagePath.boardIconUnselected,
    route: `/board`,
  },
  {
    id: 'design',
    text: 'Design',
    selected: imagePath.designIconSelected,
    unselected: imagePath.designIconUnselected,
    route: '/design',
  },
  {
    id: 'design_workbench',
    text: 'Front-End Workbench',
    selected: imagePath.frontendWorkbenchIconSelected,
    unselected: imagePath.frontendWorkbenchIconUnselected,
    route: `/design_workbench`,
  },
  {
    id: 'workbench',
    text: 'Back-End Workbench',
    selected: imagePath.backendWorkbenchIconSelected,
    unselected: imagePath.backendWorkbenchIconUnselected,
    route: `/workbench`,
  },
  {
    id: 'code',
    text: 'Code',
    selected: imagePath.codeIconSelected,
    unselected: imagePath.codeIconUnselected,
    route: `/code`,
  },
  {
    id: 'pull_request',
    text: 'Pull Request',
    selected: imagePath.pullRequestIconSelected,
    unselected: imagePath.pullRequestIconUnselected,
    route: `/pull_request`,
  },
];
