CREATE TYPE role_type AS ENUM ('admin', 'member');
CREATE TYPE encryption_algorithm_type AS ENUM ('AES', 'Kuznechik');
CREATE TYPE chat_type AS ENUM ('group', 'private');
CREATE TYPE notification_type AS ENUM ('message', 'friend_request', 'system');

CREATE TABLE users
(
    id            SERIAL PRIMARY KEY,
    username      VARCHAR(50)  NOT NULL UNIQUE,
    email         VARCHAR(100) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    created_at    TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at    TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    is_active     BOOLEAN   DEFAULT TRUE
);

CREATE TABLE chats
(
    id         SERIAL PRIMARY KEY,
    name       VARCHAR(100),
    type chat_type NOT NULL,
    created_by INT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (created_by) REFERENCES users (id) ON DELETE CASCADE
);

CREATE TABLE messages
(
    id                   SERIAL PRIMARY KEY,
    chat_id              INT NOT NULL,
    sender_id            INT  NOT NULL,
    content              TEXT NOT NULL,
    encryption_algorithm encryption_algorithm_type,
    sent_at              TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (sender_id) REFERENCES users (id) ON DELETE CASCADE,
    FOREIGN KEY (chat_id) REFERENCES chats (id) ON DELETE CASCADE

);

CREATE TABLE chat_participants
(
    chat_id     INT NOT NULL,
    user_id     INT NOT NULL,
    role        role_type NOT NULL,
    joined_at   TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
    FOREIGN KEY (chat_id) REFERENCES  chats (id) ON DELETE CASCADE
);

CREATE TABLE notification
(
    id              SERIAL PRIMARY KEY,
    user_id         INT NOT NULL,
    type            notification_type NOT NULL,
    context         VARCHAR(255),
    read            boolean,
    created_at      TIMESTAMP NOT NULL
)