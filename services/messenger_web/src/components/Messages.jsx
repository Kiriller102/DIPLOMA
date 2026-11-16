import React, { useEffect, useState, useRef } from "react";
import styled from "styled-components";
import { api } from "../api";
import { ALGORITHMS } from '../crypto/protocol';

const Messages = ({ chatID, userID, users, cryptoSession, algorithm }) => {
    const [messages, setMessages] = useState([]);
    const [error, setError] = useState('');
    const [keys, setKeys] = useState([]);
    const [migrationPending, setMigrationPending] = useState(false);
    const [migrationStatus, setMigrationStatus] = useState('');
    const endRef = useRef(null);
    const migrated = useRef(new Map());
    useEffect(() => {
        if (!chatID) return;
        const controller = new AbortController();
        let timer;
        let lastID = 0;
        const poll = async () => {
            try {
                const data = await api(`/messages/longpoll/${chatID}?last_id=${lastID}`, { signal: controller.signal });
                if (controller.signal.aborted) return;
                if (data?.length) {
                    const decoded = await Promise.all(data.map(async message => {
                        try { return await cryptoSession.decrypt(message); }
                        catch (err) { return { ...message, content: `Не удалось расшифровать: ${err.message}`, failed: true }; }
                    }));
                    if (controller.signal.aborted) return;
                    lastID = data[data.length - 1].id;
                    setMessages(current => {
                        const ids = new Set(current.map(message => message.id));
                        return [...current, ...decoded.filter(message => !ids.has(message.id)).map(message => migrated.current.get(message.id) || message)];
                    });
                }
                setError('');
                timer = setTimeout(poll, 50);
            } catch (err) {
                if (err.name !== 'AbortError') {
                    if ([401, 403, 404].includes(err.status)) setError('Доступ к чату закрыт. Выберите другой чат или войдите снова.');
                    else { setError(`Не удалось получить сообщения: ${err.message}. Повторяем подключение…`); timer = setTimeout(poll, 3000); }
                }
            }
        };
        cryptoSession.chatKeys(chatID).then(data => { if (!controller.signal.aborted) setKeys(data); }).catch(err => { if (!controller.signal.aborted) setError(err.message); });
        poll();
        return () => { controller.abort(); clearTimeout(timer); };
    }, [chatID, cryptoSession]);
    useEffect(() => { endRef.current?.scrollIntoView({ behavior: 'smooth' }); }, [messages]);
    const migrate = async () => {
        if (migrationPending) return;
        setMigrationPending(true); setMigrationStatus('');
        try {
            const history = await api(`/messages/${chatID}`);
            const legacy = history.messages.filter(message => message.crypto_version === 0);
            for (const [index, message] of legacy.entries()) {
                setMigrationStatus(`Шифруем старую историю: ${index + 1} из ${legacy.length}…`);
                const encrypted = await cryptoSession.migrate(message, algorithm);
                migrated.current.set(message.id, encrypted);
                setMessages(current => current.map(item => item.id === message.id ? encrypted : item));
            }
            setMigrationStatus('Старая история зашифрована.');
        } catch (err) { setMigrationStatus(`Перенос остановлен: ${err.message}. Уже перенесённые сообщения сохранены.`); }
        finally { setMigrationPending(false); }
    };
    return <StyledMessages aria-live="polite">
        {chatID && <details className="keys"><summary>Ключи участников · сверить отпечатки</summary>
            <p>Сверьте эти отпечатки с участниками по другому каналу связи, особенно при первом общении.</p>
            {keys.map(key => <p key={key.user_id}>{users.find(user => user.id === key.user_id)?.username || `Пользователь ${key.user_id}`}<br /><code>{key.key_id?.match(/.{1,8}/g)?.join(' ') || 'Ещё не настроил шифрование'}</code></p>)}
        </details>}
        {error && <p role="alert">{error}</p>}
        {messages.some(message => message.legacy) && <div className="legacy" role="status">
            <p>В чате есть старая история, которая хранится открытым текстом. Для переноса все участники должны настроить шифрование.</p>
            <button disabled={migrationPending} onClick={migrate}>Зашифровать старую историю</button>
            <small>Алгоритм переноса: {ALGORITHMS[algorithm]}</small>
        </div>}
        {migrationStatus && <p role="status">{migrationStatus}</p>}
        {!messages.length && <div className="welcome"><h3>{chatID ? 'Начните беседу' : 'Выберите чат'}</h3><p>Создайте чат и пригласите другого пользователя.</p></div>}
        {messages.map(message => <div key={message.id} className={`message ${message.sender_id === userID ? 'own' : ''}`}>
            <small>{users.find(user => user.id === message.sender_id)?.username || `Пользователь ${message.sender_id}`} · {new Date(message.sent_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })} · {message.legacy ? 'Без шифрования: старая история' : message.failed ? 'Ошибка проверки' : ALGORITHMS[message.algorithm]}{message.migrated ? ' · перенесено участником из старой истории' : ''}</small>
            <div>{message.content}</div>
        </div>)}
        <div ref={endRef} />
    </StyledMessages>;
};

const StyledMessages = styled.div`
    flex: auto;
    overflow-y: auto;
    padding-top: 10px;
    background: rgba(0, 0, 0, 0);
    border-radius: 10px;
    display: flex;
    flex-direction: column;
    width: 100%;
    .keys, .legacy { padding: 10px; border: 1px solid #666879; border-radius: 8px; margin-bottom: 10px; color: #eee; }
    .keys code { overflow-wrap: anywhere; font-size: 12px; }
    .legacy small { display: block; margin-top: 6px; }

    .own { align-self: flex-end; }
    small { color: #b9bdec; }
    .message {
        background: rgba(247, 197, 159, .1);
        color: rgb(255, 255, 255);
        padding: 8px 12px;
        border-radius: 6px;
        margin-bottom: 5px;
        margin-right: 10px;
        width: fit-content;
        max-width: 100%;
        white-space: pre-wrap;
        word-wrap: break-word;
        word-break: break-word;
    }

    .welcome {
        text-align: center;
        color: #bbb;
        padding: 20px;
    }

    .welcome h3 {
        align-self: center;
        margin-top: auto;
        font-size: 18px;
        font-weight: bold;
    }

    .welcome p {
        margin-top: 15px;
        font-size: 14px;
    }
`;

export default Messages;
