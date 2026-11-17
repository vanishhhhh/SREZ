import type {
  AuthResult,
  AuthState,
  Profile,
  ProfilePatch,
  PublicUser,
  RegisterInput,
  RegistrationResult,
} from './auth-types';
import { ApiError, jsonRequest } from './http';
import type { RequestOptions } from './http';
export { ApiError } from './http';

const emptyState: AuthState = { status: 'checking', user: null, error: null, errorStatus: null };
const asApiError = (error: unknown) =>
  error instanceof ApiError ? error : new ApiError(0, 'network unavailable');
function authResult(value: AuthResult): AuthResult {
  if (
    !value ||
    typeof value.access_token !== 'string' ||
    !value.access_token ||
    typeof value.csrf_token !== 'string' ||
    !value.csrf_token
  )
    throw new ApiError(502, 'invalid response');
  return value;
}
function publicUser(value: PublicUser): PublicUser {
  if (
    !value ||
    !Number.isSafeInteger(value.id) ||
    value.id <= 0 ||
    typeof value.username !== 'string' ||
    typeof value.display_name !== 'string' ||
    typeof value.avatar_url !== 'string'
  )
    throw new ApiError(502, 'invalid response');
  const { id, username, display_name, avatar_url } = value;
  return { id, username, display_name, avatar_url };
}
function profile(value: Profile): Profile {
  const user = publicUser(value);
  if (
    typeof value.email !== 'string' ||
    typeof value.email_verification_required !== 'boolean' ||
    !(
      value.email_verified_at === null ||
      (typeof value.email_verified_at === 'string' &&
        Number.isFinite(Date.parse(value.email_verified_at)))
    )
  )
    throw new ApiError(502, 'invalid response');
  return {
    ...user,
    email: value.email,
    email_verified_at: value.email_verified_at,
    email_verification_required: value.email_verification_required,
  };
}

// Only this instance holds the access JWT. React and cross-tab messages expose
// identity/status only. HttpOnly cookies remain entirely browser-managed.
export class SessionClient {
  private access = '';
  private generation = 0;
  private initialized = false;
  private loggingOut = false;
  private refreshFlight: Promise<string> | null = null;
  private restoreFlight: Promise<void> | null = null;
  private loginFlight: Promise<void> | null = null;
  private logoutFlight: Promise<void> | null = null;
  private state: AuthState = emptyState;
  private failure: ApiError | null = null;
  private listeners = new Set<() => void>();
  private channel: BroadcastChannel | null = null;
  private cookieQueue: Promise<void> = Promise.resolve();
  constructor() {
    if (typeof window !== 'undefined' && 'BroadcastChannel' in window) {
      this.channel = new BroadcastChannel('srez-session-events');
      this.channel.onmessage = (event) => {
        if (event.data === 'logout') this.clear();
        else if (event.data === 'session-changed') void this.synchronize();
      };
    }
  }
  private async synchronize(): Promise<void> {
    const generation = ++this.generation;
    this.access = '';
    this.failure = null;
    this.initialized = false;
    this.emit({ status: 'checking', user: null, error: null, errorStatus: null });
    await Promise.allSettled([
      this.loginFlight,
      this.logoutFlight,
      this.restoreFlight,
      this.refreshFlight,
    ]);
    if (generation === this.generation) await this.restore();
  }
  snapshot = () => this.state;
  subscribe = (fn: () => void) => {
    this.listeners.add(fn);
    return () => {
      this.listeners.delete(fn);
    };
  };
  private emit(state: AuthState) {
    this.state = state;
    for (const fn of this.listeners) fn();
  }
  private assertCurrent(generation: number) {
    if (generation !== this.generation) throw new ApiError(401, 'session changed');
  }
  private clear(error: string | null = null) {
    this.generation++;
    this.access = '';
    this.failure = null;
    this.initialized = true;
    this.emit({ status: 'anonymous', user: null, error, errorStatus: error ? 401 : null });
  }
  private unavailable(error: unknown) {
    this.failure = asApiError(error);
    this.emit({
      status: 'unavailable',
      user: this.state.user,
      error: this.failure.message,
      errorStatus: this.failure.status,
    });
  }
  private authenticated(user: Profile) {
    this.failure = null;
    this.initialized = true;
    this.emit({ status: 'authenticated', user, error: null, errorStatus: null });
  }
  private async sessionLock<T>(fn: () => Promise<T>): Promise<T> {
    // Serialize ALL cookie mutations, including login/logout, across tabs.
    // Without Web Locks, per-instance single-flight and one bounded 409 retry
    // remain; no credentials or bearer tokens are shared through storage.
    const run = this.cookieQueue.then(() => {
      if (typeof navigator !== 'undefined' && navigator.locks)
        return navigator.locks.request('srez-cookie-session', fn);
      return fn();
    });
    this.cookieQueue = run.then(
      () => {},
      () => {},
    );
    return run;
  }
  private async rotate(): Promise<AuthResult> {
    const csrf = (await jsonRequest<{ csrf_token: string }>('/auth/csrf')).data;
    if (!csrf || typeof csrf.csrf_token !== 'string' || !csrf.csrf_token)
      throw new ApiError(502, 'invalid response');
    return authResult(
      (
        await jsonRequest<AuthResult>('/auth/refresh', {
          method: 'POST',
          body: {},
          headers: { 'X-CSRF-Token': csrf.csrf_token },
        })
      ).data,
    );
  }
  private refresh(): Promise<string> {
    if (this.loggingOut) return Promise.reject(new ApiError(401, 'session changed'));
    if (this.refreshFlight) return this.refreshFlight;
    const generation = this.generation;
    const priorUser = this.state.user;
    const hadUser = !!priorUser;
    const flight = this.sessionLock(async () => {
      this.assertCurrent(generation);
      let result: AuthResult;
      try {
        result = await this.rotate();
      } catch (error) {
        if (!(error instanceof ApiError) || error.status !== 409) throw error;
        // Give the winning response a chance to update the browser cookie.
        // Never retain/replay a refresh value; re-read CSRF for CURRENT cookie.
        await new Promise((resolve) => setTimeout(resolve, 200));
        this.assertCurrent(generation);
        result = await this.rotate();
      }
      this.assertCurrent(generation);
      this.access = result.access_token;
      if (priorUser) {
        // Also revalidate identity when BroadcastChannel is unavailable. A
        // different cookie identity must never receive an old user's write.
        const current = profile(
          (
            await jsonRequest<Profile>('/profile', {
              headers: { Authorization: 'Bearer ' + this.access },
            })
          ).data,
        );
        this.assertCurrent(generation);
        if (current.id !== priorUser.id) {
          ++this.generation;
          this.authenticated(current);
          throw new ApiError(401, 'session changed');
        }
        this.authenticated(current);
      }
      return this.access;
    })
      .catch((error) => {
        if (generation === this.generation) {
          if (error instanceof ApiError && error.status === 401)
            this.clear(hadUser ? new ApiError(401, 'session expired').message : null);
          else this.unavailable(error);
        }
        throw error;
      })
      .finally(() => {
        if (this.refreshFlight === flight) this.refreshFlight = null;
      });
    this.refreshFlight = flight;
    return flight;
  }
  async requestWithHeaders<T>(
    path: string,
    options: RequestOptions = {},
  ): Promise<{ data: T; headers: Headers }> {
    if (!options.auth) return jsonRequest<T>(path, options);
    if (this.loggingOut) throw new ApiError(401, 'session changed');
    if (this.state.status === 'unavailable')
      throw this.failure || new ApiError(0, 'network unavailable');
    const generation = this.generation;
    const token = this.access || (await this.refresh());
    this.assertCurrent(generation);
    const run = async (access: string) => {
      options.signal?.throwIfAborted();
      const result = await jsonRequest<T>(path, {
        ...options,
        headers: { ...options.headers, Authorization: 'Bearer ' + access },
      });
      this.assertCurrent(generation);
      return result;
    };
    try {
      return await run(token);
    } catch (error) {
      // A response from a previous identity/logout must NEVER rotate the new
      // cookie, clear a newer login, or return private data to the old view.
      this.assertCurrent(generation);
      if (!(error instanceof ApiError) || error.status !== 401) throw error;
      if (this.access === token) this.access = '';
      const replacement = this.access || (await this.refresh());
      this.assertCurrent(generation);
      try {
        return await run(replacement);
      } catch (retryError) {
        if (
          generation === this.generation &&
          retryError instanceof ApiError &&
          retryError.status === 401
        )
          this.clear(new ApiError(401, 'session expired').message);
        throw retryError;
      }
    }
  }
  async request<T>(path: string, options: RequestOptions = {}): Promise<T> {
    return (await this.requestWithHeaders<T>(path, options)).data;
  }
  bootstrap(): Promise<void> {
    return this.initialized ? Promise.resolve() : this.restore();
  }
  restore(): Promise<void> {
    if (this.loginFlight) return this.loginFlight.catch(() => {});
    if (this.restoreFlight) return this.restoreFlight;
    const generation = this.generation;
    this.emit({ ...this.state, status: 'checking', error: null, errorStatus: null });
    const flight = (async () => {
      try {
        if (!this.access) await this.refresh();
        this.assertCurrent(generation);
        const user = profile(await this.request<Profile>('/profile', { auth: true }));
        this.assertCurrent(generation);
        this.authenticated(user);
      } catch (error) {
        if (generation === this.generation) {
          if (error instanceof ApiError && error.status === 401)
            this.clear(this.state.user ? new ApiError(401, 'session expired').message : null);
          else this.unavailable(error);
        }
      }
    })().finally(() => {
      this.initialized = true;
      if (this.restoreFlight === flight) this.restoreFlight = null;
    });
    this.restoreFlight = flight;
    return flight;
  }
  async register(input: RegisterInput): Promise<RegistrationResult> {
    const data = await this.request<RegistrationResult>('/auth/register', {
      method: 'POST',
      body: input,
    });
    const user = publicUser(data);
    if (
      data.email_verification_required !== true ||
      !['sent', 'unavailable'].includes(data.email_delivery_status) ||
      !Number.isSafeInteger(data.resend_after_seconds) ||
      data.resend_after_seconds < 0
    )
      throw new ApiError(502, 'invalid response');
    return {
      ...user,
      email_verification_required: true,
      email_delivery_status: data.email_delivery_status,
      resend_after_seconds: data.resend_after_seconds,
    };
  }
  login(identifier: string, password: string): Promise<void> {
    if (this.loginFlight) return this.loginFlight;
    const flight = this.performLogin(identifier, password).finally(() => {
      if (this.loginFlight === flight) this.loginFlight = null;
    });
    this.loginFlight = flight;
    return flight;
  }
  private async performLogin(identifier: string, password: string): Promise<void> {
    await Promise.allSettled([this.restoreFlight, this.refreshFlight, this.logoutFlight]);
    const generation = ++this.generation;
    this.access = '';
    this.failure = null;
    this.emit({ status: 'checking', user: null, error: null, errorStatus: null });
    try {
      await this.sessionLock(async () => {
        this.assertCurrent(generation);
        const result = authResult(
          await this.request<AuthResult>('/auth/login', {
            method: 'POST',
            body: { identifier, password },
          }),
        );
        this.assertCurrent(generation);
        this.access = result.access_token;
      });
      const user = profile(await this.request<Profile>('/profile', { auth: true }));
      this.assertCurrent(generation);
      this.authenticated(user);
      this.channel?.postMessage('session-changed');
    } catch (error) {
      if (generation === this.generation) {
        if (
          error instanceof ApiError &&
          (error.status === 401 || error.code === 'email verification required')
        )
          this.clear(error.message);
        else this.unavailable(error);
      }
      throw error;
    }
  }
  async updateProfile(input: ProfilePatch): Promise<Profile> {
    const generation = this.generation;
    // Explicit whitelist: username/email/id/role are never accepted here.
    const { display_name } = input;
    const user = profile(
      await this.request<Profile>('/profile', {
        method: 'PATCH',
        auth: true,
        body: { display_name },
      }),
    );
    this.assertCurrent(generation);
    this.authenticated(user);
    return user;
  }
  async resetPassword(token: string, password: string): Promise<void> {
    const generation = this.generation;
    await this.request('/auth/reset-password', { method: 'POST', body: { token, password } });
    // A later login/identity change must survive an earlier reset response.
    if (generation === this.generation) {
      this.clear();
      this.channel?.postMessage('logout');
    }
  }
  async reloadProfile(): Promise<void> {
    const generation = this.generation;
    const user = profile(await this.request<Profile>('/profile', { auth: true }));
    this.assertCurrent(generation);
    this.authenticated(user);
  }
  logout(): Promise<void> {
    if (this.logoutFlight) return this.logoutFlight;
    this.loggingOut = true;
    ++this.generation;
    const flight = this.sessionLock(async () => {
      try {
        const csrf = (await jsonRequest<{ csrf_token: string }>('/auth/csrf')).data;
        if (!csrf || typeof csrf.csrf_token !== 'string' || !csrf.csrf_token)
          throw new ApiError(502, 'invalid response');
        await jsonRequest('/auth/logout', {
          method: 'POST',
          body: {},
          headers: { 'X-CSRF-Token': csrf.csrf_token },
        });
      } catch (error) {
        if (!(error instanceof ApiError) || error.status !== 401) throw error;
      }
    })
      .then(() => {
        this.clear();
        this.channel?.postMessage('logout');
      })
      .finally(() => {
        this.loggingOut = false;
        if (this.logoutFlight === flight) this.logoutFlight = null;
      });
    this.logoutFlight = flight;
    return flight;
  }
  async checkServer(): Promise<void> {
    const result = await this.request<{ status: string }>('/health');
    if (!result || result.status !== 'ok') throw new ApiError(502, 'invalid response');
  }
}
export const session = new SessionClient();
