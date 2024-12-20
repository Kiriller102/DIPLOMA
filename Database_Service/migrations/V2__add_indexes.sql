-- Индекс на username в таблице users
CREATE INDEX idx_users_username ON users (username);

-- Индекс на sender_id в таблице messages
CREATE INDEX idx_messages_sender_id ON messages (sender_id);

-- Индекс на receiver_id в таблице messages
CREATE INDEX idx_messages_receiver_id ON messages (receiver_id);
