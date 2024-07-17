export interface authPayload {
  email: string;
  password: string;
  invite_token?: string;
}

export interface userData {
  userEmail: string;
  userName: string;
  organisationId: string;
  accessToken: string;
}
