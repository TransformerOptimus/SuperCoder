export const storyStatus = {
  TODO: 'TODO',
  IN_PROGRESS: 'IN_PROGRESS',
  DONE: 'DONE',
  IN_REVIEW: 'IN_REVIEW',
  MAX_LOOP_ITERATIONS: 'MAX_LOOP_ITERATION_REACHED',
  LLM_KEY_NOT_FOUND: 'IN_REVIEW_LLM_KEY_NOT_FOUND',
};

export const showStoryDetailsDropdown = [
  storyStatus.TODO,
  storyStatus.IN_REVIEW,
];

export const storyActions = {
  REBUILD: 'Re-Build',
  GET_HELP: 'GET_HELP',
  GO_TO_SETTINGS: 'GOTO_SETTINGS',
}