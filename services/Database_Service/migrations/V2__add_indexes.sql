-- Индекс на username в таблице users
CREATE UNIQUE INDEX idx_users_username ON users (username);

-- Индексы на Email в таблице users
CREATE UNIQUE INDEX idx_users_email ON users (email);

-- Индекс на sender_id в таблице messages
CREATE INDEX idx_messages_sender_id ON messages (sender_id);


-- Индекс на chat_id в таблице messages
CREATE INDEX idx_messages_chat_id ON messages (chat_id);