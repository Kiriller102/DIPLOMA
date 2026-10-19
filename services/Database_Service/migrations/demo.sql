BEGIN;
SELECT pg_advisory_xact_lock(20261019);
INSERT INTO users (username, email, first_name, last_name, password_hash)
VALUES ('alice', 'alice@example.com', 'Алиса', '', '!demo-no-login'),
       ('bob', 'bob@example.com', 'Боб', '', '!demo-no-login')
ON CONFLICT DO NOTHING;

INSERT INTO chats (name, type, created_by)
SELECT 'Общий чат', 'group', id FROM users u WHERE username = 'alice'
AND NOT EXISTS (SELECT 1 FROM chats WHERE name = 'Общий чат' AND created_by = u.id);

INSERT INTO chat_participants (chat_id, user_id, role)
SELECT c.id, u.id, CASE WHEN c.created_by = u.id THEN 'admin'::role_type ELSE 'member'::role_type END
FROM chats c CROSS JOIN users u
WHERE c.name = 'Общий чат' AND u.username IN ('alice', 'bob')
ON CONFLICT DO NOTHING;
COMMIT;
