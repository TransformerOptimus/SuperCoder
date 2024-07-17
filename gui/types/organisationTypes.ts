export interface InviteUserPayload {
    organisationId: string;
    email: string;
    current_user_id: number;
}

export interface RemoveUserPayload {
    organisationId: string;
    user_id: number;
}

export interface UserTeamDetails{
    id: number;
    name: string;
    email: string;
    organisation_id: number;
}