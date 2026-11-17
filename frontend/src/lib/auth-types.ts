export interface PublicUser {
  id: number;
  username: string;
  display_name: string;
  avatar_url: string;
}

export interface Profile extends PublicUser {
  email: string;
  email_verified_at: string | null;
  email_verification_required: boolean;
}
export interface RegistrationResult extends PublicUser {
  email_verification_required: true;
  email_delivery_status: 'sent' | 'unavailable';
  resend_after_seconds: number;
}

export interface AuthResult {
  access_token: string;
  csrf_token: string;
}

export interface RegisterInput {
  email: string;
  username: string;
  display_name: string;
  password: string;
}

export interface ProfilePatch {
  display_name?: string;
}

export interface AuthState {
  status: 'checking' | 'authenticated' | 'anonymous' | 'unavailable';
  user: Profile | null;
  error: string | null;
  errorStatus: number | null;
}
