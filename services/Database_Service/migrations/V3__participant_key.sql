ALTER TABLE chat_participants ADD CONSTRAINT chat_participants_pkey PRIMARY KEY (chat_id, user_id);
CREATE INDEX idx_chat_participants_user_id ON chat_participants (user_id);
