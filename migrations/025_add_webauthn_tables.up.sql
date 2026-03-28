-- Create webauthn_credentials table for storing registered passkeys
CREATE TABLE IF NOT EXISTS webauthn_credentials (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    name VARCHAR(100) NOT NULL,
    credential_id VARCHAR(255) NOT NULL,
    public_key TEXT NOT NULL,
    aaguid VARCHAR(255),
    sign_count INTEGER DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_webauthn_credentials_credential_id ON webauthn_credentials(credential_id);
CREATE INDEX IF NOT EXISTS idx_webauthn_credentials_user_id ON webauthn_credentials(user_id);

-- Create webauthn_sessions table for temporary challenge data during registration/authentication
CREATE TABLE IF NOT EXISTS webauthn_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    challenge VARCHAR(255) NOT NULL,
    session_data TEXT,
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_webauthn_sessions_user_id ON webauthn_sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_webauthn_sessions_expires_at ON webauthn_sessions(expires_at);
