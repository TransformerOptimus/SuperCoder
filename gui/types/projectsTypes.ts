export interface CreateProjectPayload {
  name: string;
  backendFramework: string;
  frontendFramework: string;
  description: string;
}

export interface UpdateProjectPayload {
  project_id: number;
  name: string;
  description: string;
}

export interface ProjectTypes {
  project_id: number;
  project_name: string;
  project_description: string;
  project_hash_id: string;
  project_url: string;
  project_backend_url: string;
  project_frontend_url: string;
  pull_request_count: number;
  project_framework: string;
}
