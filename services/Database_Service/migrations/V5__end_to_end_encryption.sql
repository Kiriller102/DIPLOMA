-- Private keys are encrypted by a separate client-only secret phrase.
CREATE TABLE crypto_identities (
    user_id INT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    key_id TEXT NOT NULL UNIQUE,
    exchange_key TEXT NOT NULL,
    signing_key TEXT NOT NULL,
    salt TEXT NOT NULL,
    nonce TEXT NOT NULL,
    encrypted_private_keys TEXT NOT NULL,
    iterations INT NOT NULL CHECK (iterations = 600000)
);

-- Existing rows remain readable until a participant migrates them in a browser.
ALTER TABLE messages ADD COLUMN crypto_version INT NOT NULL DEFAULT 0;
ALTER TABLE messages ADD COLUMN client_id TEXT;
ALTER TABLE messages ADD CONSTRAINT messages_crypto_format CHECK (
    (crypto_version = 0 AND client_id IS NULL) OR
    (crypto_version = 1 AND client_id IS NOT NULL AND encryption_algorithm IS NOT NULL
        AND left(content, 8) = 'e2ee:v1:')
);
CREATE UNIQUE INDEX messages_client_id ON messages(chat_id, client_id) WHERE client_id IS NOT NULL;
