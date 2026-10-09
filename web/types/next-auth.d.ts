import { DefaultSession } from 'next-auth';
import { JWT } from 'next-auth/jwt';

declare module 'next-auth' {
  interface Session {
    user: {
      id: string;
      role?: string;
      // Go tokenVersion captured at login; embedded in every backend
      // assertion so a revocation bump invalidates this session (SEC-02).
      ver?: number;
      // Add custom fields here
    } & DefaultSession['user']; // includes name, email, image
  }

  interface User {
    id: string;
    role?: string;
    tokenVersion?: number;
    // Mirror any fields added to Session['user'] above
  }
}

declare module 'next-auth/jwt' {
  interface JWT {
    id: string;
    role?: string;
    ver?: number;
  }
}
