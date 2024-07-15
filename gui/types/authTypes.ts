export interface authPayload {
  email: string;
  password: string;
  organizationId?: string;
}

export interface userData {
  userEmail: string;
  userName: string;
  organisationId: string;
  accessToken: string;
}
